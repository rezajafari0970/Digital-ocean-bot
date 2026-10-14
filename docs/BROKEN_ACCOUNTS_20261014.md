# Broken accounts — 2026-10-14
Status at 2026-10-14T15:46:16.161641+00:00: source approved; publication and runtime acceptance pending.
Private base 4f3b273869e79e01c2c73a5ce2c2ae3dff855151; prior production/public main+stable be435f7865649598e1d2df841aea1e5c7c8f0638.

## Live evidence
Initial primary observation 2026-10-14T14:25:12Z: 7 enabled accounts; dashboard reported 3 broken, 3 healthy, 1 capacity-full. Server counts:48 tracked,34 active,1 provisioning,13 pending deletion,0 broken,0 degraded.

| Account | Evidence | Required outcome |
|---|---|---|
| S1 | Explicit DigitalOcean account-lock response misclassified as transport failure | Correct LOCKED diagnosis; account owner/provider support resolves external restriction |
| Sor oc | Same explicit account-lock response | Same |
| Digi oc | Last complete account status locked; stored ACTIVE; local network-not-ready error | Retain lock evidence and independently report network health; no forced unlock |
| Oceano new1 | Provider warning at exact10/10 capacity | Capacity-full; existing servers are not broken by capacity |
| Linode mari | Healthy observation | Preserve |
| Vultr Usman1 | Healthy observation;9 managed | Preserve configured capacity/spacing |
| Vultr Usman12 | Healthy observation;10 managed | Preserve |

Digi's provider text lists lack of payment OR improper use; no sole cause is inferred. At2026-10-14T15:03:24Z its native network readiness predicates were all true: account proxy healthy, IPv4 present, identity healthy/fresh, base fresh, no pending recovery, healthy closed circuit, generation86. That demonstrates current network recovery, not removal of the provider lock. The precise historical predicate that failed is unavailable.

## Changes
- Normalize only explicit typed account-lock responses at400/403/422; preserve401/429/5xx and ambiguous mutation handling.
- Reject successful-HTTP locked account status before later inventory/catalog failures; persist account-only evidence without claiming complete inventory.
- Share the network-not-ready sentinel. Native proxy recovery no longer clears provider restrictions or independently writes READY.
- Bind all account observation writers to start time, credential reference and actual secret version, including in-place credential replacement. Preserve independent semantic and transient errors; rate limits cannot erase locks.
- Publish complete snapshot/state/capacity events transactionally. Lock current network rows through READY/error-clear publication and reject stale runtime identity/generation/epoch.
- Separate scheduled provider probe timing from local failure timestamps. Unknown, warning and trial status cannot resolve existing hard restrictions.
- Show bounded per-account diagnostic rows, independent errors, last complete/account-only evidence and timestamps. Retained locked evidence is checked against credential age/newer resolution; account details and dashboard agree.
- Add migration174 only for evidence columns; it does not rewrite provider states.
- Stabilize existing browser fixtures around popup visibility, repeatable update-status GET and document-global initialization after reload, and supplying the isolated Linode authenticated bootstrap endpoint before waiting for visible route/event-handler readiness.

## Verification
{
  "provider_and_postgres_regressions": "PASS",
  "full_go": "PASS full-r6 all packages",
  "race": "PASS four affected packages",
  "vet": "PASS",
  "browser": "PASS 27 real Chromium scenarios",
  "astra": "harness-resolution PASS",
  "reviewed_file_count": 29,
  "runtime_acceptance": "PENDING",
  "publication": "PENDING"
}
Earlier failed logs remain in the evidence directory. The full-r2 failure was an obsolete test fixture ON CONFLICT target; production code was unchanged by its correction. A later unchanged clientops test collided on a database-wide advisory lock during concurrent package testing; it passed three isolated repetitions. The full-r4 and full-r5 failures were confined to the Linode browser fixture: it could click before initialization and lacked the authenticated bootstrap endpoint. That fixture now returns the isolated authenticated principal, waits for visible initialized account controls and includes timeout diagnostics; three isolated repetitions passed. The final full-r6 suite, focused regressions and race checks pass with the corrected fixtures; public publication independently reruns its full gates.
No provider CREATE/DELETE acceptance mutations, credential replacement, proxy bypass/forced rotation, quota/Desired/spacing/protection changes, or manual account-state resets were performed in production.

## Astra API
{
  "responses": 8,
  "input_tokens": 175323,
  "output_tokens": 17316,
  "total_tokens": 192639,
  "billed_usd": null,
  "scope": "This broken-accounts task only; prior tasks excluded",
  "receipts": [
    {
      "phase": "final",
      "id": "resp_0640cb89751d78ae006acf9d1a9c0487d1811523089a868980",
      "model": "gpt-6-astra",
      "usage": {
        "input_tokens": 56930,
        "input_tokens_details": {
          "cache_write_tokens": 56927,
          "cached_tokens": 0
        },
        "output_tokens": 3072,
        "output_tokens_details": {
          "reasoning_tokens": 1490
        },
        "total_tokens": 60002
      }
    },
    {
      "phase": "harness-final",
      "id": "resp_03943e007877db35006acfa0c2569c87d1a16addb1d3a16a44",
      "model": "gpt-6-astra",
      "usage": {
        "input_tokens": 4267,
        "input_tokens_details": {
          "cache_write_tokens": 4264,
          "cached_tokens": 0
        },
        "output_tokens": 596,
        "output_tokens_details": {
          "reasoning_tokens": 57
        },
        "total_tokens": 4863
      }
    },
    {
      "phase": "harness-resolution",
      "id": "resp_0331bd2e51cefb68006acfa1f6151087d1ad2056434e3df929",
      "model": "gpt-6-astra",
      "usage": {
        "input_tokens": 8070,
        "input_tokens_details": {
          "cache_write_tokens": 8067,
          "cached_tokens": 0
        },
        "output_tokens": 667,
        "output_tokens_details": {
          "reasoning_tokens": 55
        },
        "total_tokens": 8737
      }
    },
    {
      "phase": "ordering-final",
      "id": "resp_0267bf1694cc3f58006acf9f08eae487d1a5c12e3c0fcf6f67",
      "model": "gpt-6-astra",
      "usage": {
        "input_tokens": 12648,
        "input_tokens_details": {
          "cache_write_tokens": 12645,
          "cached_tokens": 0
        },
        "output_tokens": 1161,
        "output_tokens_details": {
          "reasoning_tokens": 302
        },
        "total_tokens": 13809
      }
    },
    {
      "phase": "plan",
      "id": "resp_095fd72aa489bb88006acf916eb39487d1a925c0c49a8920e8",
      "model": "gpt-6-astra",
      "usage": {
        "input_tokens": 20080,
        "input_tokens_details": {
          "cache_write_tokens": 20077,
          "cached_tokens": 0
        },
        "output_tokens": 3255,
        "output_tokens_details": {
          "reasoning_tokens": 165
        },
        "total_tokens": 23335
      }
    },
    {
      "phase": "publication-final",
      "id": "resp_0fa3a09d71f0cf8c006acfa039fa0887d1aa76abbf4489a6f0",
      "model": "gpt-6-astra",
      "usage": {
        "input_tokens": 12485,
        "input_tokens_details": {
          "cache_write_tokens": 12482,
          "cached_tokens": 0
        },
        "output_tokens": 784,
        "output_tokens_details": {
          "reasoning_tokens": 144
        },
        "total_tokens": 13269
      }
    },
    {
      "phase": "resolution",
      "id": "resp_07abe0aa066fe79c006acf9970539487d182eff7c8a9b02629",
      "model": "gpt-6-astra",
      "usage": {
        "input_tokens": 37548,
        "input_tokens_details": {
          "cache_write_tokens": 37545,
          "cached_tokens": 0
        },
        "output_tokens": 4218,
        "output_tokens_details": {
          "reasoning_tokens": 2070
        },
        "total_tokens": 41766
      }
    },
    {
      "phase": "source",
      "id": "resp_0893f17058deca6d006acf957cebc087d1b868a0ca4a0fa2b0",
      "model": "gpt-6-astra",
      "usage": {
        "input_tokens": 23295,
        "input_tokens_details": {
          "cache_write_tokens": 23292,
          "cached_tokens": 0
        },
        "output_tokens": 3563,
        "output_tokens_details": {
          "reasoning_tokens": 1423
        },
        "total_tokens": 26858
      }
    }
  ]
}
Actual receipts are authoritative. Billing dollars and this ChatGPT session's token accounting are unavailable.

## Release acceptance
Pending gated public export, secret scanning, unprivileged regressions, exact-candidate Ubuntu24 install/update, atomic main/stable publication and both-host preservation/runtime checks.
