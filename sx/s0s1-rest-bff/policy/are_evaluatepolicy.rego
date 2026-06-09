# Maps EvaluatePolicy JSON input to { effect, reason }.
# Query: data.are.evaluatepolicy.decision (POST /v1/data/are/evaluatepolicy/decision)
#
# This policy is intentionally small enough to inspect while still modeling a
# real foundation rule: model promotion is allowed only for governed model
# resources and remains check-only at the API boundary.
package are.evaluatepolicy

default decision = {"effect": "DENY", "reason": "no policy matched action class"}

decision = {"effect": "DENY", "reason": "forbidden action class for demo"} {
	input.action_class == "demo.forbidden_action"
} else = {"effect": "ALLOW", "reason": "read scope allowed"} {
	input.action_class == "demo.read"
	startswith(input.resource, "urn:dataset/")
} else = {"effect": "ALLOW", "reason": "model promotion policy passed for governed model resource"} {
	input.action_class == "model.promote_to_production"
	input.agent_id != ""
	startswith(input.resource, "model/")
	not contains(lower(input.resource), "experimental")
} else = {"effect": "DENY", "reason": "model promotion requires a governed non-experimental model resource"} {
	input.action_class == "model.promote_to_production"
} else = {"effect": "DENY", "reason": "action class contains deny"} {
	contains(upper(input.action_class), "DENY")
} else = {"effect": "ALLOW", "reason": "policy-evaluated"}
