# Maps EvaluatePolicy JSON input to { effect, reason }.
# Query: data.are.evaluatepolicy.decision (POST /v1/data/are/evaluatepolicy/decision)
package are.evaluatepolicy

decision = {"effect": "DENY", "reason": "forbidden action class for demo"} {
	input.action_class == "demo.forbidden_action"
} else = {"effect": "ALLOW", "reason": "read scope allowed"} {
	input.action_class == "demo.read"
} else = {"effect": "DENY", "reason": "action class contains deny"} {
	contains(upper(input.action_class), "DENY")
} else = {"effect": "ALLOW", "reason": "policy-evaluated"}
