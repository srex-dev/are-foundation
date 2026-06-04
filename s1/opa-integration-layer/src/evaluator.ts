import { EvaluateRequest, LoadedBundle, PolicyEffect, PolicyRule, RuleMatch } from "./types";

/** Pre-policy deterministic checks against {@link EvaluateRequest.context.admissionEnvelope}. */
export function assertAdmissionEnvelope(
  request: EvaluateRequest
): { ok: true } | { ok: false; denyReason: string } {
  const env = request.context.admissionEnvelope;
  if (!env) {
    return { ok: true };
  }

  const scopes = env.admittedScopes ?? [];
  if (scopes.length > 0) {
    const ac = request.actionClass;
    let scopeOk = false;
    for (const pat of scopes) {
      if (pat.endsWith("*")) {
        const pre = pat.slice(0, -1);
        if (ac === pre || ac.startsWith(pre)) {
          scopeOk = true;
          break;
        }
      } else if (ac === pat) {
        scopeOk = true;
        break;
      }
    }
    if (!scopeOk) {
      return { ok: false, denyReason: "admission_scope_violation" };
    }
  }

  const caps = env.admittedBehavioralCaps ?? {};
  for (const [metric, cap] of Object.entries(caps)) {
    const raw = request.context.actionMetadata[metric];
    if (raw === undefined || raw === "") {
      continue;
    }
    const val = Number(raw);
    if (Number.isNaN(val)) {
      return { ok: false, denyReason: `admission_cap_invalid_numeric:${metric}` };
    }
    if (val > cap) {
      return { ok: false, denyReason: `admission_cap_exceeded:${metric}` };
    }
  }

  return { ok: true };
}

/**
 * Parses simple rule source into executable policy rules.
 * Full Rego semantics belong in OPA proper (see audit M-17 / DEFERRED: optional `opa` CLI or WASM path).
 */
export function compileRegoLikeSource(regoSource: string): PolicyRule[] {
  const trimmed = regoSource.trim();
  if (!trimmed || trimmed.includes("invalid_rego")) {
    throw new Error("rego_compile_error");
  }

  const lines = trimmed
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.length > 0 && !line.startsWith("#"));
  const rules: PolicyRule[] = [];

  for (const [idx, line] of lines.entries()) {
    if (!/^(ALLOW|DENY|ESCALATE)\b/i.test(line)) {
      throw new Error("rego_compile_error");
    }
    const effectTokens = line.match(/\b(ALLOW|DENY|ESCALATE)\b/gi);
    if (!effectTokens || effectTokens.length !== 1) {
      throw new Error("rego_compile_error");
    }
    if (!/\baction=\S+/.test(line)) {
      throw new Error("rego_compile_error");
    }
    const parts = line.split(/\s+/);
    if (parts.length < 3) {
      throw new Error("rego_compile_error");
    }
    const effect = parts[0].toUpperCase() as PolicyEffect;
    if (effect !== "ALLOW" && effect !== "DENY" && effect !== "ESCALATE") {
      throw new Error("rego_compile_error");
    }
    const actionToken = parts.find((p) => p.startsWith("action="));
    if (!actionToken) {
      throw new Error("rego_compile_error");
    }
    const actionClass = actionToken.slice("action=".length);
    const prefixToken = parts.find((p) => p.startsWith("prefix="));
    rules.push({
      ruleName: `${effect.toLowerCase()}_rule_${idx + 1}`,
      actionClass,
      effect,
      resourcePrefix: prefixToken ? prefixToken.slice("prefix=".length) : undefined
    });
  }
  return rules;
}

/** Evaluates one request against loaded rules and returns full evidence. */
export function evaluateWithBundles(
  request: EvaluateRequest,
  bundles: LoadedBundle[]
): { effect: PolicyEffect; denyReason: string; matchedRules: RuleMatch[]; firedRules: string[] } {
  const admission = assertAdmissionEnvelope(request);
  if (!admission.ok) {
    return { effect: "DENY", denyReason: admission.denyReason, matchedRules: [], firedRules: [] };
  }

  const matchedRules: RuleMatch[] = [];
  let seenAllow = false;
  let seenEscalate = false;
  let seenDeny = false;
  let coveredActionClass = false;

  for (const bundle of bundles) {
    for (const rule of bundle.rules) {
      if (rule.actionClass !== request.actionClass) {
        continue;
      }
      coveredActionClass = true;
      const resourceMatch = !rule.resourcePrefix || request.resource.startsWith(rule.resourcePrefix);
      const fired = resourceMatch;
      matchedRules.push({
        ruleName: rule.ruleName,
        bundleName: bundle.bundleName,
        actionClass: rule.actionClass,
        effect: rule.effect,
        fired,
        matchReason: resourceMatch ? "resource_match" : "resource_mismatch"
      });
      if (fired) {
        if (rule.effect === "ALLOW") {
          seenAllow = true;
        } else if (rule.effect === "ESCALATE") {
          seenEscalate = true;
        } else if (rule.effect === "DENY") {
          seenDeny = true;
        }
      }
    }
  }

  if (!coveredActionClass) {
    return { effect: "DENY", denyReason: "no_policy_covers_action_class", matchedRules, firedRules: [] };
  }
  if (seenDeny) {
    return {
      effect: "DENY",
      denyReason: "deny_rule_fired",
      matchedRules,
      firedRules: matchedRules.filter((r) => r.fired && r.effect === "DENY").map((r) => r.ruleName)
    };
  }
  if (seenEscalate) {
    return {
      effect: "ESCALATE",
      denyReason: "",
      matchedRules,
      firedRules: matchedRules.filter((r) => r.fired && r.effect === "ESCALATE").map((r) => r.ruleName)
    };
  }
  if (seenAllow) {
    return {
      effect: "ALLOW",
      denyReason: "",
      matchedRules,
      firedRules: matchedRules.filter((r) => r.fired && r.effect === "ALLOW").map((r) => r.ruleName)
    };
  }
  return { effect: "DENY", denyReason: "no_rule_fired", matchedRules, firedRules: [] };
}
