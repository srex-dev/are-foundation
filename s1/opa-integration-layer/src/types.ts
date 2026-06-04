/** Policy effect ordering uses DENY > ESCALATE > ALLOW. */
export type PolicyEffect = "ALLOW" | "DENY" | "ESCALATE";

/** Request scope context from passport. */
export interface ContextScope {
  actionClass: string;
  resourcePattern: string;
  expiresTs: number;
}

/** Immutable admission contract for an agent (registry / co-processor). */
export interface AdmissionEnvelope {
  envelopeId: string;
  agentId: string;
  policyId: string;
  policyVer: string;
  admittedScopes: string[];
  admittedBehavioralCaps: Record<string, number>;
  admittedTsMs: number;
  issuingAuthority: string;
  /** Opaque signature bytes when present. */
  signature: Uint8Array;
}

/** Full evaluation context from policy co-processor. */
export interface EvaluationContext {
  passportId: string;
  passportType: string;
  activeScopes: ContextScope[];
  environment: Record<string, string>;
  agentMetadata: Record<string, string>;
  actionMetadata: Record<string, string>;
  requestTs: number;
  /**
   * When set, admission checks run before policy rules. Otherwise the service may
   * fetch from {@link AdmissionEnvelopeClient} when configured.
   */
  admissionEnvelope?: AdmissionEnvelope | null;
}

/** Evaluate request contract. */
export interface EvaluateRequest {
  evaluationId: string;
  agentId: string;
  actionClass: string;
  resource: string;
  context: EvaluationContext;
  bundleNames?: string[];
}

/** Rule match evidence returned to caller. */
export interface RuleMatch {
  ruleName: string;
  bundleName: string;
  actionClass: string;
  effect: PolicyEffect;
  fired: boolean;
  matchReason: string;
}

/** Evaluate response contract. */
export interface EvaluateResponse {
  evaluationId: string;
  effect: PolicyEffect;
  decisionReason: string;
  matchedRules: RuleMatch[];
  firedRules: string[];
  activeBundleNames: string[];
  activeBundleVersions: string[];
  evaluatedTs: number;
  cacheHit: boolean;
  denyReason: string;
}

/** Single policy rule representation used by in-process evaluator. */
export interface PolicyRule {
  ruleName: string;
  actionClass: string;
  effect: PolicyEffect;
  resourcePrefix?: string;
}

/** Bundle payload fetched from registry. */
export interface BundlePayload {
  bundleId: string;
  bundleName: string;
  version: string;
  regoSource: string;
  /** When set, must equal lowercase hex SHA-256 of regoSource (UTF-8); mismatch fails load. */
  integritySha256?: string;
  /**
   * Optional Ed25519 signature (standard 64-byte raw, base64-encoded) over
   * `${bundleId}\n${version}\n${integritySha256}` UTF-8 bytes. Requires env
   * `ARE_OPA_BUNDLE_SIGNING_PUBLIC_KEY_PEM` (SPKI PEM).
   */
  bundleSignatureEd25519?: string;
}

/** Compiled bundle loaded into evaluator. */
export interface LoadedBundle {
  bundleName: string;
  bundleId: string;
  version: string;
  loadedTs: number;
  ruleCount: number;
  compileValid: boolean;
  rules: PolicyRule[];
}

/** Registry client contract consumed by integration layer. */
export interface PolicyRegistryClient {
  getActiveBundles(): Promise<BundlePayload[]>;
  getActiveBundle(bundleName: string): Promise<BundlePayload>;
  getBundle(bundleName: string, version: string): Promise<BundlePayload>;
}

/** Fetches admission envelopes from agent-registry (or a test double). */
export interface AdmissionEnvelopeClient {
  getAdmissionEnvelope(agentId: string): Promise<AdmissionEnvelope | null>;
}

/** Parsed lifecycle event consumed from Kafka. */
export interface PolicyLifecycleEvent {
  eventType: "BUNDLE_ACTIVATED" | "BUNDLE_DEPRECATED" | string;
  bundleName: string;
  bundleVersion?: string;
}
