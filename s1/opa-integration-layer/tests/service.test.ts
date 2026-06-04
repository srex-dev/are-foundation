import { createHash, generateKeyPairSync, sign } from "node:crypto";
import { evaluateWithBundles } from "../src/evaluator";
import { OpaIntegrationService } from "../src/service";
import {
  AdmissionEnvelope,
  AdmissionEnvelopeClient,
  BundlePayload,
  EvaluateRequest,
  EvaluateResponse,
  LoadedBundle,
  PolicyRegistryClient
} from "../src/types";

class RegistryMock implements PolicyRegistryClient {
  public activeBundles: BundlePayload[] = [];
  private readonly byVersion = new Map<string, BundlePayload>();

  public setBundle(bundle: BundlePayload): void {
    const idx = this.activeBundles.findIndex((b) => b.bundleName === bundle.bundleName);
    if (idx >= 0) {
      this.activeBundles[idx] = bundle;
    } else {
      this.activeBundles.push(bundle);
    }
    this.byVersion.set(`${bundle.bundleName}:${bundle.version}`, bundle);
  }

  public async getActiveBundles(): Promise<BundlePayload[]> {
    return this.activeBundles;
  }

  public async getActiveBundle(bundleName: string): Promise<BundlePayload> {
    const found = this.activeBundles.find((b) => b.bundleName === bundleName);
    if (!found) {
      throw new Error("not_found");
    }
    return found;
  }

  public async getBundle(bundleName: string, version: string): Promise<BundlePayload> {
    const found = this.byVersion.get(`${bundleName}:${version}`);
    if (!found) {
      throw new Error("not_found");
    }
    return found;
  }
}

function admissionEnvelope(overrides: Partial<AdmissionEnvelope> = {}): AdmissionEnvelope {
  return {
    envelopeId: "env-1",
    agentId: "agent-1",
    policyId: "p",
    policyVer: "v",
    admittedScopes: ["READ"],
    admittedBehavioralCaps: { max_latency_ms: 50 },
    admittedTsMs: Date.now(),
    issuingAuthority: "test",
    signature: new Uint8Array(),
    ...overrides
  };
}

function req(overrides: Partial<EvaluateRequest> = {}): EvaluateRequest {
  return {
    evaluationId: "eval-1",
    agentId: "agent-1",
    actionClass: "READ",
    resource: "resource/a",
    context: {
      passportId: "passport-1",
      passportType: "STANDARD",
      activeScopes: [],
      environment: {},
      agentMetadata: {},
      actionMetadata: {},
      requestTs: Date.now()
    },
    ...overrides
  };
}

function bundle(name: string, version: string, regoSource: string, integrity?: "auto" | string): BundlePayload {
  const payload: BundlePayload = {
    bundleId: `${name}-${version}`,
    bundleName: name,
    version,
    regoSource
  };
  if (integrity === "auto") {
    payload.integritySha256 = createHash("sha256").update(regoSource, "utf8").digest("hex");
  } else if (integrity) {
    payload.integritySha256 = integrity;
  }
  return payload;
}

describe("OpaIntegrationService", () => {
  test("startup requires at least one active bundle", async () => {
    const registry = new RegistryMock();
    const service = new OpaIntegrationService(registry);
    await expect(service.startupLoadBundles()).rejects.toThrow("no_active_bundles");
  });

  test("startup rejects bundle when integritySha256 does not match rego source", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=READ prefix=resource/", "deadbeef"));
    const service = new OpaIntegrationService(registry);
    await expect(service.startupLoadBundles()).rejects.toThrow("bundle_integrity_mismatch");
  });

  test("startup accepts bundleSignatureEd25519 when public key PEM is configured", async () => {
    const rego = "ALLOW action=READ prefix=resource/";
    const b = bundle("b1", "1.0.0", rego, "auto");
    const dig = b.integritySha256 as string;
    const msg = Buffer.from(`${b.bundleId}\n${b.version}\n${dig}`, "utf8");
    const { privateKey, publicKey } = generateKeyPairSync("ed25519");
    const pubPem = publicKey.export({ type: "spki", format: "pem" });
    if (typeof pubPem !== "string") {
      throw new Error("expected pem string");
    }
    b.bundleSignatureEd25519 = sign(null, msg, privateKey).toString("base64");
    const prev = process.env.ARE_OPA_BUNDLE_SIGNING_PUBLIC_KEY_PEM;
    process.env.ARE_OPA_BUNDLE_SIGNING_PUBLIC_KEY_PEM = pubPem;
    try {
      const registry = new RegistryMock();
      registry.setBundle(b);
      const service = new OpaIntegrationService(registry);
      await service.startupLoadBundles();
    } finally {
      if (prev === undefined) {
        delete process.env.ARE_OPA_BUNDLE_SIGNING_PUBLIC_KEY_PEM;
      } else {
        process.env.ARE_OPA_BUNDLE_SIGNING_PUBLIC_KEY_PEM = prev;
      }
    }
  });

  test("reload of one bundle preserves cache entries that only reference another bundle", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=READ prefix=resource/"));
    registry.setBundle(bundle("b2", "1.0.0", "DENY action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();

    expect((await service.evaluate(req({ evaluationId: "e1", bundleNames: ["b1"] }))).cacheHit).toBe(false);
    expect((await service.evaluate(req({ evaluationId: "e2", bundleNames: ["b1"] }))).cacheHit).toBe(true);

    registry.setBundle(bundle("b2", "1.0.1", "DENY action=READ prefix=resource/"));
    const r = await service.reloadBundle("b2");
    expect(r.reloaded).toBe(true);

    expect((await service.evaluate(req({ evaluationId: "e3", bundleNames: ["b1"] }))).cacheHit).toBe(true);
  });

  test("evaluate with no matching action returns deny by default", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=WRITE prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();
    const res = await service.evaluate(req({ actionClass: "READ" }));
    expect(res.effect).toBe("DENY");
    expect(res.denyReason).toBe("no_policy_covers_action_class");
  });

  test("conflict resolution deny beats allow", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("allow-bundle", "1.0.0", "ALLOW action=READ prefix=resource/"));
    registry.setBundle(bundle("deny-bundle", "1.0.0", "DENY action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();
    const res = await service.evaluate(req());
    expect(res.effect).toBe("DENY");
    expect(res.firedRules.length).toBeGreaterThan(0);
  });

  test("conflict resolution escalate beats allow", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("allow-bundle", "1.0.0", "ALLOW action=READ prefix=resource/"));
    registry.setBundle(bundle("escalate-bundle", "1.0.0", "ESCALATE action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();
    const res = await service.evaluate(req());
    expect(res.effect).toBe("ESCALATE");
  });

  test("evaluation errors return deny and unload after 10 consecutive errors", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("err-bundle", "1.0.0", "ALLOW action=READ prefix=resource/"));
    const throwingEvaluator = (_request: EvaluateRequest, _bundles: LoadedBundle[]) => {
      throw new Error("boom");
    };
    const service = new OpaIntegrationService(registry, 30, 1000, throwingEvaluator);
    await service.startupLoadBundles();
    for (let i = 0; i < 10; i++) {
      const res = await service.evaluate(req({ evaluationId: `eval-${i}` }));
      expect(res.effect).toBe("DENY");
      expect(res.denyReason).toBe("evaluation_error");
    }
    expect(service.getLoadedBundles().bundles.find((b) => b.bundleName === "err-bundle")).toBeUndefined();
  });

  test("cache hit on identical request and invalidates after bundle version change", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();

    const first = await service.evaluate(req({ evaluationId: "eval-first" }));
    const second = await service.evaluate(req({ evaluationId: "eval-second" }));
    expect(first.cacheHit).toBe(false);
    expect(second.cacheHit).toBe(true);

    registry.setBundle(bundle("b1", "1.0.1", "ALLOW action=READ prefix=resource/"));
    await service.handleLifecycleEvent({ eventType: "BUNDLE_ACTIVATED", bundleName: "b1", bundleVersion: "1.0.1" });
    const third = await service.evaluate(req({ evaluationId: "eval-third" }));
    expect(third.cacheHit).toBe(false);
    expect(third.activeBundleVersions).toContain("1.0.1");
  });

  test("invalid rego on activation retains old bundle", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();

    registry.setBundle(bundle("b1", "1.0.1", "invalid_rego"));
    await service.handleLifecycleEvent({ eventType: "BUNDLE_ACTIVATED", bundleName: "b1", bundleVersion: "1.0.1" });
    const loaded = service.getLoadedBundles().bundles.find((b) => b.bundleName === "b1");
    expect(loaded?.version).toBe("1.0.0");
  });

  test("deprecation removes bundle and evaluate denies", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();
    await service.handleLifecycleEvent({ eventType: "BUNDLE_DEPRECATED", bundleName: "b1" });
    const res = await service.evaluate(req());
    expect(res.effect).toBe("DENY");
  });

  test("deprecating one bundle does not invalidate cache entries that only used another bundle", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=READ prefix=resource/"));
    registry.setBundle(bundle("b2", "1.0.0", "DENY action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();

    const first = await service.evaluate(req({ evaluationId: "e1", bundleNames: ["b2"] }));
    const second = await service.evaluate(req({ evaluationId: "e2", bundleNames: ["b2"] }));
    expect(first.cacheHit).toBe(false);
    expect(second.cacheHit).toBe(true);

    await service.handleLifecycleEvent({ eventType: "BUNDLE_DEPRECATED", bundleName: "b1" });

    const third = await service.evaluate(req({ evaluationId: "e3", bundleNames: ["b2"] }));
    expect(third.cacheHit).toBe(true);
    expect(third.effect).toBe("DENY");
  });

  test("deprecating a bundle invalidates cache entries that referenced it", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=READ prefix=resource/"));
    registry.setBundle(bundle("b2", "1.0.0", "DENY action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();

    expect((await service.evaluate(req({ evaluationId: "e1", bundleNames: ["b2"] }))).cacheHit).toBe(false);
    expect((await service.evaluate(req({ evaluationId: "e2", bundleNames: ["b2"] }))).cacheHit).toBe(true);

    await service.handleLifecycleEvent({ eventType: "BUNDLE_DEPRECATED", bundleName: "b2" });

    const third = await service.evaluate(req({ evaluationId: "e3", bundleNames: ["b2"] }));
    expect(third.cacheHit).toBe(false);
    expect(third.effect).toBe("DENY");
    expect(third.denyReason).toBe("no_policy_covers_action_class");
  });

  test("reloadBundle no-op when version unchanged", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();
    const r = await service.reloadBundle("b1");
    expect(r.reloaded).toBe(false);
  });

  test("reloadBundle loads new active version", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();
    registry.setBundle(bundle("b1", "1.0.1", "DENY action=READ prefix=resource/"));
    const r = await service.reloadBundle("b1");
    expect(r.reloaded).toBe(true);
    const res = await service.evaluate(req({ evaluationId: "eval-after-reload" }));
    expect(res.effect).toBe("DENY");
  });

  test("hot reload during concurrent evaluations does not fail requests", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();

    const evaluations: Array<Promise<EvaluateResponse>> = [];
    for (let i = 0; i < 50; i++) {
      evaluations.push(service.evaluate(req({ evaluationId: `eval-${i}` })));
      if (i === 10) {
        registry.setBundle(bundle("b1", "1.0.1", "ESCALATE action=READ prefix=resource/"));
        await service.handleLifecycleEvent({ eventType: "BUNDLE_ACTIVATED", bundleName: "b1", bundleVersion: "1.0.1" });
      }
    }
    const results = await Promise.all(evaluations);
    expect(results.length).toBe(50);
    for (const result of results) {
      expect(["ALLOW", "ESCALATE", "DENY"]).toContain(result.effect);
    }
  });

  test("bundle filter evaluates only selected bundle names", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("allow-bundle", "1.0.0", "ALLOW action=READ prefix=resource/"));
    registry.setBundle(bundle("deny-bundle", "1.0.0", "DENY action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();
    const res = await service.evaluate(req({ bundleNames: ["allow-bundle"] }));
    expect(res.effect).toBe("ALLOW");
  });

  test("admission behavioral cap denies even when policy allows", async () => {
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry);
    await service.startupLoadBundles();
    const base = req({
      context: {
        ...req().context,
        admissionEnvelope: admissionEnvelope(),
        actionMetadata: { max_latency_ms: "500" }
      }
    });
    const res = await service.evaluate(base);
    expect(res.effect).toBe("DENY");
    expect(res.denyReason).toBe("admission_cap_exceeded:max_latency_ms");
    expect(res.decisionReason).toBe("admission_envelope");
  });

  test("admission envelope client fetch is cached per agent", async () => {
    let calls = 0;
    const client: AdmissionEnvelopeClient = {
      async getAdmissionEnvelope(agentId) {
        calls++;
        if (agentId !== "agent-1") {
          return null;
        }
        return admissionEnvelope();
      }
    };
    const registry = new RegistryMock();
    registry.setBundle(bundle("b1", "1.0.0", "ALLOW action=READ prefix=resource/"));
    const service = new OpaIntegrationService(registry, 30, 10000, evaluateWithBundles, {
      admissionEnvelopeClient: client,
      admissionEnvelopeCacheTtlSeconds: 60
    });
    await service.startupLoadBundles();
    const ctx = { ...req().context, actionMetadata: { max_latency_ms: "10" } };
    const r1 = await service.evaluate(req({ evaluationId: "x1", context: ctx }));
    const r2 = await service.evaluate(req({ evaluationId: "x2", context: ctx }));
    expect(calls).toBe(1);
    expect(r1.effect).toBe("ALLOW");
    expect(r2.cacheHit).toBe(true);
  });
});
