# State Transition Brain

Commit `9a57a8921e235351c57943af23724dc7e119474f` — **54 known state-like values**, **62 owner functions with transition/guard evidence**.

- Owners with explicit state-write evidence: **48**
- Owners with guard/precondition evidence: **52**
- Terminal-like states by naming evidence: **22**

This brain identifies owner functions, exact source ranges, state mentions, explicit write evidence, guards/preconditions, retry/timing evidence and DB touch points. It intentionally does not fabricate a from→to edge when source does not make direction explicit.
