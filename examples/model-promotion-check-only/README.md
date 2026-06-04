# Model Promotion Check-Only

This example checks whether a release agent has scoped authority and policy permission for `model.promote_to_production`.

It does not promote a model.

Run:

```bash
make up
make smoke
```

Expected proof basics:

- scope decision: `ALLOW`
- policy decision: `ALLOW`
- executed: `false`
- receipt created: `false`

