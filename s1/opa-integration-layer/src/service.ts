import { createHash, createPublicKey, verify } from "node:crypto";
import { EvaluationCache } from "./cache";
import { compileRegoLikeSource, evaluateWithBundles } from "./evaluator";
import {
  AdmissionEnvelope,
  AdmissionEnvelopeClient,
  BundlePayload,
  EvaluateRequest,
  EvaluateResponse,
  LoadedBundle,
  PolicyLifecycleEvent,
  PolicyRegistryClient
} from "./types";

type DecisionResult = ReturnType<typeof evaluateWithBundles>;
type Evaluator = (request: EvaluateRequest, bundles: LoadedBundle[]) => DecisionResult;

export type OpaIntegrationServiceOptions = {
  admissionEnvelopeClient?: AdmissionEnvelopeClient;
  admissionEnvelopeCacheTtlSeconds?: number;
};

/** In-memory OPA integration service with deny-by-default behavior. */
export class OpaIntegrationService {
  private readonly registry: PolicyRegistryClient;
  private readonly cache: EvaluationCache;
  private readonly loadedByName = new Map<string, LoadedBundle>();
  private readonly consecutiveErrorsByBundle = new Map<string, number>();
  private readonly evaluator: Evaluator;
  private readonly options: OpaIntegrationServiceOptions;
  private readonly admissionCache = new Map<string, { env: AdmissionEnvelope | null; ts: number }>();
  private readonly admissionInflight = new Map<string, Promise<AdmissionEnvelope | null>>();
  private lastReloadTs = 0;

  public constructor(
    registry: PolicyRegistryClient,
    cacheTtlSeconds = 30,
    cacheMaxEntries = 10000,
    evaluator: Evaluator = evaluateWithBundles,
    options: OpaIntegrationServiceOptions = {}
  ) {
    this.registry = registry;
    this.cache = new EvaluationCache(cacheTtlSeconds, cacheMaxEntries);
    this.evaluator = evaluator;
    this.options = options;
  }

  public async startupLoadBundles(): Promise<void> {
    const active = await this.registry.getActiveBundles();
    if (active.length === 0) {
      throw new Error("no_active_bundles");
    }
    for (const bundle of active) {
      this.loadBundleFromPayload(bundle, true);
    }
  }

  public async evaluate(request: EvaluateRequest): Promise<EvaluateResponse> {
    const now = Date.now();
    if (!request.evaluationId || !request.agentId || !request.actionClass || !request.resource || !request.context?.passportId) {
      throw new Error("INVALID_ARGUMENT");
    }

    let effectiveRequest = request;
    const embedded = request.context.admissionEnvelope;
    if (!embedded && this.options.admissionEnvelopeClient) {
      const fetched = await this.resolveAdmissionEnvelope(request.agentId, now);
      if (fetched) {
        effectiveRequest = {
          ...request,
          context: { ...request.context, admissionEnvelope: fetched }
        };
      }
    }

    const selected = this.selectBundles(effectiveRequest.bundleNames);
    const versions = selected.map((b) => `${b.bundleName}:${b.version}`).sort();
    const envelopeId = effectiveRequest.context.admissionEnvelope?.envelopeId ?? "none";
    const metadataKey = this.flattenMetadata(effectiveRequest.context.actionMetadata);
    const cacheKey = this.buildCacheKey(
      effectiveRequest.agentId,
      effectiveRequest.actionClass,
      effectiveRequest.resource,
      effectiveRequest.context.passportId,
      versions,
      envelopeId,
      metadataKey
    );
    const cached = this.cache.get(cacheKey, now);
    if (cached) {
      return cached;
    }

    try {
      if (selected.length === 0) {
        const deny = this.denyResponse(effectiveRequest, now, [], [], "no_policy_covers_action_class");
        this.cache.set(cacheKey, deny, now);
        return deny;
      }
      const decision = this.evaluator(effectiveRequest, selected);
      const response: EvaluateResponse = {
        evaluationId: effectiveRequest.evaluationId,
        effect: decision.effect,
        decisionReason:
          decision.effect === "DENY" && decision.denyReason.startsWith("admission_")
            ? "admission_envelope"
            : decision.effect === "DENY"
              ? "deny_by_default"
              : "policy_decision",
        matchedRules: decision.matchedRules,
        firedRules: decision.firedRules,
        activeBundleNames: selected.map((b) => b.bundleName),
        activeBundleVersions: selected.map((b) => b.version),
        evaluatedTs: now,
        cacheHit: false,
        denyReason: decision.denyReason
      };
      this.cache.set(cacheKey, response, now);
      for (const bundle of selected) {
        this.consecutiveErrorsByBundle.set(bundle.bundleName, 0);
      }
      return response;
    } catch {
      for (const bundle of selected) {
        const errors = (this.consecutiveErrorsByBundle.get(bundle.bundleName) ?? 0) + 1;
        this.consecutiveErrorsByBundle.set(bundle.bundleName, errors);
        if (errors >= 10) {
          this.loadedByName.delete(bundle.bundleName);
        }
      }
      return this.denyResponse(
        effectiveRequest,
        now,
        selected.map((b) => b.bundleName),
        selected.map((b) => b.version),
        "evaluation_error"
      );
    }
  }

  private async resolveAdmissionEnvelope(agentId: string, nowMs: number): Promise<AdmissionEnvelope | null> {
    const client = this.options.admissionEnvelopeClient;
    if (!client) {
      return null;
    }
    const ttlMs = (this.options.admissionEnvelopeCacheTtlSeconds ?? 30) * 1000;
    const hit = this.admissionCache.get(agentId);
    if (hit && nowMs - hit.ts < ttlMs) {
      return hit.env;
    }
    let inflight = this.admissionInflight.get(agentId);
    if (!inflight) {
      inflight = (async () => {
        try {
          const env = await client.getAdmissionEnvelope(agentId);
          this.admissionCache.set(agentId, { env, ts: Date.now() });
          return env;
        } finally {
          this.admissionInflight.delete(agentId);
        }
      })();
      this.admissionInflight.set(agentId, inflight);
    }
    return inflight;
  }

  public getLoadedBundles(): { bundles: LoadedBundle[]; lastReloadTs: number } {
    return {
      bundles: Array.from(this.loadedByName.values()).sort((a, b) => a.bundleName.localeCompare(b.bundleName)),
      lastReloadTs: this.lastReloadTs
    };
  }

  public async reloadBundle(bundleName: string): Promise<{ bundle: LoadedBundle | null; reloaded: boolean; message: string }> {
    if (!bundleName) {
      throw new Error("INVALID_ARGUMENT");
    }
    const active = await this.registry.getActiveBundle(bundleName);
    const existing = this.loadedByName.get(bundleName);
    if (existing && existing.version === active.version) {
      return { bundle: existing, reloaded: false, message: "no-op, already current version" };
    }
    const loaded = this.loadBundleFromPayload(active, false);
    return { bundle: loaded, reloaded: true, message: "bundle reloaded" };
  }

  public async handleLifecycleEvent(event: PolicyLifecycleEvent): Promise<void> {
    if (event.eventType === "BUNDLE_ACTIVATED") {
      const payload = event.bundleVersion
        ? await this.registry.getBundle(event.bundleName, event.bundleVersion)
        : await this.registry.getActiveBundle(event.bundleName);
      this.loadBundleFromPayload(payload, false);
      return;
    }
    if (event.eventType === "BUNDLE_DEPRECATED") {
      this.loadedByName.delete(event.bundleName);
      this.cache.invalidateForBundle(event.bundleName);
      this.lastReloadTs = Date.now();
    }
  }

  private selectBundles(bundleNames?: string[]): LoadedBundle[] {
    const all = Array.from(this.loadedByName.values());
    if (!bundleNames || bundleNames.length === 0) {
      return all;
    }
    const wanted = new Set(bundleNames);
    return all.filter((b) => wanted.has(b.bundleName));
  }

  private verifyBundlePayload(payload: BundlePayload): void {
    const expected = payload.integritySha256?.trim().toLowerCase();
    if (expected) {
      const digest = createHash("sha256").update(payload.regoSource, "utf8").digest("hex");
      if (digest !== expected) {
        throw new Error("bundle_integrity_mismatch");
      }
    }
    const sigB64 = payload.bundleSignatureEd25519?.trim();
    if (!sigB64) {
      return;
    }
    if (!expected) {
      throw new Error("bundle_signature_requires_integritySha256");
    }
    const pem = process.env.ARE_OPA_BUNDLE_SIGNING_PUBLIC_KEY_PEM?.trim();
    if (!pem) {
      throw new Error("bundle_signature_requires_ARE_OPA_BUNDLE_SIGNING_PUBLIC_KEY_PEM");
    }
    const pub = createPublicKey(pem);
    const msg = Buffer.from(`${payload.bundleId}\n${payload.version}\n${expected}`, "utf8");
    const sig = Buffer.from(sigB64, "base64");
    if (sig.length !== 64) {
      throw new Error("bundle_signature_invalid_length");
    }
    if (!verify(null, msg, pub, sig)) {
      throw new Error("bundle_signature_verify_failed");
    }
  }

  private loadBundleFromPayload(payload: BundlePayload, startupStrict: boolean): LoadedBundle {
    try {
      this.verifyBundlePayload(payload);
      const rules = compileRegoLikeSource(payload.regoSource);
      const loaded: LoadedBundle = {
        bundleName: payload.bundleName,
        bundleId: payload.bundleId,
        version: payload.version,
        loadedTs: Date.now(),
        ruleCount: rules.length,
        compileValid: true,
        rules
      };
      this.loadedByName.set(payload.bundleName, loaded);
      this.cache.invalidateForBundle(payload.bundleName);
      this.lastReloadTs = Date.now();
      return loaded;
    } catch (err) {
      if (startupStrict) {
        throw err;
      }
      return this.loadedByName.get(payload.bundleName) ?? {
        bundleName: payload.bundleName,
        bundleId: payload.bundleId,
        version: payload.version,
        loadedTs: Date.now(),
        ruleCount: 0,
        compileValid: false,
        rules: []
      };
    }
  }

  private denyResponse(
    request: EvaluateRequest,
    now: number,
    bundleNames: string[],
    bundleVersions: string[],
    denyReason: string
  ): EvaluateResponse {
    return {
      evaluationId: request.evaluationId,
      effect: "DENY",
      decisionReason: "deny_by_default",
      matchedRules: [],
      firedRules: [],
      activeBundleNames: bundleNames,
      activeBundleVersions: bundleVersions,
      evaluatedTs: now,
      cacheHit: false,
      denyReason
    };
  }

  private flattenMetadata(meta: Record<string, string>): string {
    const keys = Object.keys(meta).sort();
    return keys.map((k) => `${k}=${meta[k]}`).join("&");
  }

  private buildCacheKey(
    agentId: string,
    actionClass: string,
    resource: string,
    passportId: string,
    sortedBundleVersions: string[],
    envelopeId: string,
    actionMetadataKey: string
  ): string {
    const hash = createHash("sha256");
    hash.update(agentId);
    hash.update("|");
    hash.update(actionClass);
    hash.update("|");
    hash.update(resource);
    hash.update("|");
    hash.update(passportId);
    hash.update("|");
    hash.update(sortedBundleVersions.join(","));
    hash.update("|");
    hash.update(envelopeId);
    hash.update("|");
    hash.update(actionMetadataKey);
    return hash.digest("hex");
  }
}
