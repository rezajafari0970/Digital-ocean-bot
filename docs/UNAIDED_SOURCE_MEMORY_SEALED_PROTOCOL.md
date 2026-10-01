# Sealed Unaided Source-Memory Protocol

This protocol measures memory rather than retrieval.

The immutable public exam is:

`docs/UNAIDED_SOURCE_MEMORY_SEALED_PROMPT.txt`

Its embedded SHA256 binds the exact public question set.

## Valid test conditions

A valid run starts in a fresh chat that has not loaded:

- repository content
- Project Brain
- source snapshots
- previous project chats
- answer keys

The new chat receives only the sealed prompt.

From prompt delivery until the answer JSON is frozen, the answering
chat must make zero retrieval/tool calls.

Unknown answers must be represented as `null`.

The resulting file must contain exactly 80 answer IDs.

## Three independent scores

The grader must report these separately:

1. Conceptual Source Recall
2. File / Function / Route Topology Recall
3. Verbatim Source Recall

A retrieval score must never be substituted for any of these metrics.

## Freeze rule

After answers are frozen, retrieval and grading are allowed.

Post-freeze lookup cannot modify or improve the frozen memory score.
