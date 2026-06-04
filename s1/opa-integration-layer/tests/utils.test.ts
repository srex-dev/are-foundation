import { EvaluationCache } from "../src/cache";
import { assertAdmissionEnvelope, compileRegoLikeSource, evaluateWithBundles } from "../src/evaluator";
import { EvaluateRequest, LoadedBundle } from "../src/types";

function request(
  actionClass = "READ",
  resource = "resource/x",
  ctxOverrides: Partial<EvaluateRequest["context"]> = {}
): EvaluateRequest {
  return {
    evaluationId: "eval-utility",
    agentId: "agent-1",
    actionClass,
    resource,
    context: {
      passportId: "passport-1",
      passportType: "STANDARD",
      activeScopes: [],
      environment: {},
      agentMetadata: {},
      actionMetadata: {},
      requestTs: Date.now(),
      ...ctxOverrides
    }
  };
}

describe("EvaluationCache", () => {
  test("cache miss, set/get, expiry, clear, and LRU eviction", () => {
    const cache = new EvaluationCache(1, 2);
    const now = 1000;

    expect(cache.get("missing", now)).toBeNull();
    cache.set("a", {
      evaluationId: "a",
      effect: "ALLOW",
      decisionReason: "ok",
      matchedRules: [],
      firedRules: [],
      activeBundleNames: ["b"],
      activeBundleVersions: ["1.0.0"],
      evaluatedTs: now,
      cacheHit: false,
      denyReason: ""
    }, now);
    expect(cache.get("a", now + 10)?.cacheHit).toBe(true);

    cache.set("b", {
      evaluationId: "b",
      effect: "DENY",
      decisionReason: "deny",
      matchedRules: [],
      firedRules: [],
      activeBundleNames: ["b"],
      activeBundleVersions: ["1.0.0"],
      evaluatedTs: now,
      cacheHit: false,
      denyReason: "x"
    }, now);
    cache.set("c", {
      evaluationId: "c",
      effect: "DENY",
      decisionReason: "deny",
      matchedRules: [],
      firedRules: [],
      activeBundleNames: ["b"],
      activeBundleVersions: ["1.0.0"],
      evaluatedTs: now,
      cacheHit: false,
      denyReason: "x"
    }, now);

    expect(cache.get("a", now + 20)).toBeNull();
    expect(cache.get("b", now + 2001)).toBeNull();

    cache.clear();
    expect(cache.size()).toBe(0);
  });

  test("invalidateForBundle removes only matching entries", () => {
    const cache = new EvaluationCache(60, 10);
    const now = 1000;
    const base = {
      evaluationId: "x",
      effect: "ALLOW" as const,
      decisionReason: "ok",
      matchedRules: [],
      firedRules: [],
      activeBundleVersions: ["1.0.0"],
      evaluatedTs: now,
      cacheHit: false,
      denyReason: ""
    };
    cache.set("k1", { ...base, activeBundleNames: ["b1", "b2"] }, now);
    cache.set("k2", { ...base, evaluationId: "y", activeBundleNames: ["b3"] }, now);
    expect(cache.size()).toBe(2);

    cache.invalidateForBundle("b1");
    expect(cache.get("k1", now + 1)).toBeNull();
    expect(cache.get("k2", now + 2)?.cacheHit).toBe(true);

    cache.invalidateForBundle("b3");
    expect(cache.size()).toBe(0);
  });
});

describe("assertAdmissionEnvelope", () => {
  test("allows when no envelope on context", () => {
    const r = assertAdmissionEnvelope(request());
    expect(r.ok).toBe(true);
  });

  test("scope wildcard prefix match", () => {
    const r = assertAdmissionEnvelope(
      request("READFOO", "resource/x", {
        admissionEnvelope: {
          envelopeId: "e",
          agentId: "agent-1",
          policyId: "",
          policyVer: "",
          admittedScopes: ["READ*"],
          admittedBehavioralCaps: {},
          admittedTsMs: 0,
          issuingAuthority: "x",
          signature: new Uint8Array()
        }
      })
    );
    expect(r.ok).toBe(true);
  });

  test("scope violation", () => {
    const r = assertAdmissionEnvelope(
      request("WRITE", "resource/x", {
        admissionEnvelope: {
          envelopeId: "e",
          agentId: "agent-1",
          policyId: "",
          policyVer: "",
          admittedScopes: ["READ"],
          admittedBehavioralCaps: {},
          admittedTsMs: 0,
          issuingAuthority: "x",
          signature: new Uint8Array()
        }
      })
    );
    expect(r.ok).toBe(false);
    if (!r.ok) {
      expect(r.denyReason).toBe("admission_scope_violation");
    }
  });

  test("invalid numeric cap", () => {
    const r = assertAdmissionEnvelope(
      request("READ", "resource/x", {
        admissionEnvelope: {
          envelopeId: "e",
          agentId: "agent-1",
          policyId: "",
          policyVer: "",
          admittedScopes: [],
          admittedBehavioralCaps: { max_latency_ms: 10 },
          admittedTsMs: 0,
          issuingAuthority: "x",
          signature: new Uint8Array()
        },
        actionMetadata: { max_latency_ms: "not-a-number" }
      })
    );
    expect(r.ok).toBe(false);
    if (!r.ok) {
      expect(r.denyReason).toBe("admission_cap_invalid_numeric:max_latency_ms");
    }
  });

  test("cap at boundary allows", () => {
    const r = assertAdmissionEnvelope(
      request("READ", "resource/x", {
        admissionEnvelope: {
          envelopeId: "e",
          agentId: "agent-1",
          policyId: "",
          policyVer: "",
          admittedScopes: [],
          admittedBehavioralCaps: { max_latency_ms: 100 },
          admittedTsMs: 0,
          issuingAuthority: "x",
          signature: new Uint8Array()
        },
        actionMetadata: { max_latency_ms: "100" }
      })
    );
    expect(r.ok).toBe(true);
  });
});

describe("Evaluator", () => {
  test("compile rejects malformed source", () => {
    expect(() => compileRegoLikeSource("")).toThrow("rego_compile_error");
    expect(() => compileRegoLikeSource("ALLOW")).toThrow("rego_compile_error");
    expect(() => compileRegoLikeSource("MAYBE action=READ prefix=resource/")).toThrow("rego_compile_error");
    expect(() => compileRegoLikeSource("ALLOW prefix=resource/")).toThrow("rego_compile_error");
    expect(() => compileRegoLikeSource("ALLOW ALLOW action=READ prefix=resource/")).toThrow("rego_compile_error");
  });

  test("evaluate returns no_rule_fired when action covered but resource mismatched", () => {
    const rules = compileRegoLikeSource("ALLOW action=READ prefix=resource/allowed");
    const bundles: LoadedBundle[] = [{
      bundleName: "b1",
      bundleId: "b1-1",
      version: "1.0.0",
      loadedTs: Date.now(),
      ruleCount: rules.length,
      compileValid: true,
      rules
    }];
    const result = evaluateWithBundles(request("READ", "resource/other"), bundles);
    expect(result.effect).toBe("DENY");
    expect(result.denyReason).toBe("no_rule_fired");
  });
});
