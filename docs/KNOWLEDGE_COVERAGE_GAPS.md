# Knowledge Coverage Gap Brain

Commit `808edeff184c594f54dae47fe8b8abddaf5158f7` — **8 knowledge-evidence gaps**: 1 high, 2 medium, 5 low.

These are gaps in direct evidence available to a fresh chat, not automatically defects in the product. They identify where understanding still relies on indirect inference.

| Feature | Dimension | Severity | Reason |
|---|---|---|---|
| `residential` | `tests` | **high** | no directly matched test evidence |
| `database` | `migration-origin` | **medium** | 5 live columns lack parsed migration-origin evidence |
| `database` | `code-mentions` | **medium** | 6 live columns lack direct function mention evidence |
| `sanaei_panel` | `history` | **low** | no dedicated incident guardrail currently encoded |
| `reality` | `history` | **low** | no dedicated incident guardrail currently encoded |
| `residential` | `history` | **low** | no dedicated incident guardrail currently encoded |
| `admin_api` | `history` | **low** | no dedicated incident guardrail currently encoded |
| `frontend` | `history` | **low** | no dedicated incident guardrail currently encoded |
