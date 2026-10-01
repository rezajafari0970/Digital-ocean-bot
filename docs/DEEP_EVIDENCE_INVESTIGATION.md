# Deep Evidence Investigation

Commit `17a923b74411e528c2a8cec61bb3c08daafa2675`.

## Database
The six previously indirect columns were searched repository-wide. They occur only in their migrations and have no direct application lexical references. Their ownership is therefore no longer *unknown*: current source provides evidence of **no direct access**.

## Residential
Implementation is directly evidenced across worker sync, Admin API CRUD/test endpoint, `residentialsync`, migration `000086`, and frontend actions. No direct Residential test exists in the current test suite. Git history contains Residential code presence, but no dedicated regression/incident subject strong enough to invent an incident guardrail.

This distinction matters: knowledge can be complete about a known absence without pretending product test/history evidence exists.
