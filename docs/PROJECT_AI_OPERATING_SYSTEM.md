# Project AI Operating System

Protocol version: 4

This document is the mandatory engineering protocol for every ChatGPT session, API agent, continuation, and automated development job in this repository.

## Core rule
No AI model is authoritative. Repository source, requirements, tests, runtime evidence, database invariants, and reproducible verification are authoritative.

## Session bootstrap
Every fresh chat must:
1. Run tools/new-chat-bootstrap.sh.
2. Read docs/NEW_CHAT_ENTRYPOINT.md and this document.
3. Read the current development-orchestrator state and active job.
4. Continue from the current Git HEAD and current job boundary; never reconstruct from chat history alone.
5. Use deterministic retrieval before exact source, schema, route, runtime, or production claims.

## Engineering lanes
Work is split into independent lanes whenever the task is non-trivial:
- Architect: requirement decomposition, ownership, impact graph, invariants.
- Implementer: bounded edits only.
- Reviewer: independent architecture and code review.
- Adversary: race, fault, security, stale-state, retry, isolation, rollback analysis.
- Test Designer: tests from requirements before trusting implementation.
- Verifier: deterministic gates only.
- ChatGPT Chief Engineer: resolves disagreements using evidence, not model voting.

## Speed model
Prefer parallel independent analysis over sequential prompting. Use the cheapest capable model for retrieval/classification, stronger models for implementation/review, and the strongest reasoning mode for architecture, concurrency, security, and adjudication.

## Mandatory change pipeline
For material product changes use:
REQUIREMENT -> IMPACT -> INVARIANTS -> PARALLEL PLANS -> ADVERSARIAL REVIEW -> IMPLEMENT -> INDEPENDENT TESTS -> STATIC CHECKS -> RACE/FUZZ/FAULT WHEN RELEVANT -> FULL REGRESSION -> CHECKPOINT -> CANARY/SHADOW WHEN AVAILABLE -> RUNTIME EVIDENCE -> COMPLETE.

A phase cannot be marked COMPLETE merely because an AI says it is correct.

## Evidence gates
Every completed feature must preserve an evidence bundle containing:
- requirement and non-goals
- allowed paths and ownership
- chosen design and rejected alternatives
- unresolved disagreements (must be zero for completion)
- implementation diff/commit
- focused tests
- regression/race/fault evidence as applicable
- provider parity evidence when shared provider code is touched
- runtime/canary evidence for deployed behavior
- rollback point

## Project invariants
Cross-account isolation, fail-closed proxy-required behavior, generation monotonicity, stale-generation mutation denial, stale-observation denial, circuit admission, single half-open lease, idempotent provider mutations, no secret leakage, and provider-specific frozen contracts are release blockers.

## Throughput rules
- Batch multiple compatible phases under separate checkpoints.
- Fail fast on deterministic gates.
- Cache commit-pinned knowledge and reuse it across agents.
- Send minimal owner-specific context instead of the whole repository.
- Prefer exact structured edits and local diff construction.
- Run independent reviews in parallel.
- Do not rerun completed expensive phases unless source/requirements affecting them changed.

## Persistent state
The shared source of continuation truth is:
- Git HEAD and canonical branch
- .local/dev-orchestrator/state.json
- docs/development-jobs/
- SESSION_HANDOFF_BUNDLE / HANDOFF / PROJECT_STATE
- commit-pinned Knowledge System
- deterministic evidence and audit artifacts

Chat history is supplemental, never the continuity authority.
