# Source Memory Priming Protocol

The purpose of this protocol is to improve and measure
**post-ingestion unaided source recall**.

This is different from:

- zero-context memory
- Project Brain retrieval
- exact source lookup
- Fresh Session retrieval mastery

## Three memory layers

### 1. Topology / Name

Feed:

`docs/source-memory-packs/topology-*.json`

These teach:

- files
- ordered symbols
- function names
- route → handler topology

### 2. Conceptual Source

Feed:

`docs/source-memory-packs/concept-*.json`

These teach:

- symbol purpose
- signatures
- neighboring symbols
- calls
- DB effects
- routes
- side effects
- structural roles

### 3. Verbatim Source

Feed selected:

`docs/source-memory-packs/verbatim-*.json`

These contain exact source blocks.

Verbatim training must be progressive.

A score for a small primed cohort must never be reported as
whole-repository verbatim recall.

## Priming procedure

1. Start a fresh chat.

2. Feed topology packs first.

3. Feed conceptual packs second.

4. Feed an explicitly declared cohort of verbatim packs.

5. During ingestion the chat may read only the supplied packs.

6. After ingestion explicitly enter:

   CLOSED-BOOK MODE

7. From that point until answers are frozen:

   - no tools
   - no files
   - no server
   - no repository
   - no Project Brain
   - no search
   - no web
   - no previous chats
   - no source lookup

8. Run a new holdout memory exam whose questions were not shown
   during priming.

9. Freeze answers.

10. Grade only after freeze.

## Required reporting

Always report independently:

- Conceptual Recall %
- Topology / Name Recall %
- Verbatim Recall %

Also report:

- number of conceptual symbols primed
- number of topology files primed
- number of routes primed
- number of exact source blocks primed
- percentage of total repository source represented by the
  verbatim priming cohort

Retrieval success must never be counted as memory success.
