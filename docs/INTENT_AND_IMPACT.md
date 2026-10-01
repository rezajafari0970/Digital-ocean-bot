# Intent / Requirements Brain + Impact Graph

`INTENT_REQUIREMENTS_BRAIN.json` gives stable IDs to the project's major non-negotiable intentions and explains why they exist. `IMPACT_GRAPH.json` connects those requirements to code paths/files, symbols, HTTP routes, database tables and test groups where static evidence is available.

For change planning, start from the relevant requirement ID, traverse its covered files, then use Semantic/Symbol/Schema/Test knowledge for exact implementation and verification. The graph is a navigation/impact aid, not a substitute for compiler/runtime evidence.
