# OpenAI access restored and bounded request outcome — 2026-10-25

## Connection and credential verification

At 11:08 UTC (14:38 Tehran), Remote Desktop Commander for the original
serverprojects.ptr.network was online. The existing
/etc/digital-ocean-bot/openai.env file was present, root-owned and mode0600,
with a nonempty OPENAI_API_KEY. Neither the key value nor a key fingerprint was
printed; the key was not copied to the recovery host or included in GitHub.
GET /v1/models/gpt-6-astra returned HTTP200. The former access blocker is resolved.

## One real review attempt; no result receipt

One Responses request began at 11:14:16 UTC (14:44:16 Tehran), using
gpt-6-astra, reasoning.effort=max, default service tier, store=false, no tools,
30-minute cache TTL and max_output_tokens=16384. Its limited public-source prompt
was 50,399 UTF-8 bytes; SHA256
a49e285b24fa26819a44cf7c6713824460cba1deb19eefb11cba71ca33b4aa86.
Both live-secret comparisons and gitleaks passed before sending that prompt.

The request timed out after 300 seconds. No response ID, usage receipt or review
text was received. This is NOT a successful model review, a proven server-side
cancellation, or evidence of zero token consumption. Actual input/output tokens
and billed cost remain unknown. The ledger keeps USD1.501011 reserved and blocks
all further reviews while this outcome is unresolved. No automatic retry occurred.
There was one generation attempt plus one read-only model-metadata request.

The authoritative durable ledger is on the ORIGINAL host:
/var/lib/datacenter-bot-audit/openai/ledger.json. It has one status=unknown row.
Do not delete, reinitialize, clear, or duplicate that ledger on another host to
make another request possible. Reconcile the provider's usage/cost record before
any further paid review. The USD1.501011 value is a retained conservative budget
reservation, not a bill. The cap remains USD2 and three attempts; the unresolved
outcome guard is stricter than either numeric allowance.

The current store=false synchronous helper did not receive an ID that would allow
retrieval of this response. A future design should preserve a provider receipt
before long-running reasoning and support bounded status polling, without replaying
an uncertain submission. This request was not resubmitted to implement that idea.

## Cache-write budget defect fixed before the paid request

Official GPT-6 Astra pricing charges default 30-minute cache writes at USD12.50/M,
versus USD10/M ordinary input. The previous helper charged all input at USD10/M.
A new controlled fixture reproduced its underestimate: USD0.030 versus USD0.035.
The patch pins prompt_cache_options.ttl=30m and reserves/accounts all input at a
rounded-up USD13/M upper bound. Output remains USD50/M. Cache-read discounts do
not loosen the bound. Changed ledger policies still fail closed; no ledger
existed on either audit host before this first real request.

Sources checked on 2026-10-25:
- https://developers.openai.com/api/docs/models/gpt-6-astra
- https://developers.openai.com/api/docs/guides/prompt-caching

All nine fake-transport tests pass, including the new failing-before/passing-after
cache-write accounting and preflight-reservation regression. Exact source
c71387262a7da570ef2f5bb9d8511ef646250ac8 passed independent CI40187985276:
full Go/vet/browser checks and all 192 required PostgreSQL/native/race regressions,
zero skipped. PR#6 merged as 1aaf70b545ea58fe278378acfeacd5b64f200000.
https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40187985276
https://github.com/rezajafari0970/Digital-ocean-bot/pull/6

## Operational boundaries

Original PostgreSQL and API/worker services remain inactive/quarantined. Only
lightweight metadata/API checks and private review evidence writes ran there;
no production database or application service was started. Both active recovery
instances still pass all five readiness checks. No production binary, account
setting, key or release branch changed in this API-control patch. Public main/stable
and default remain80c85157; production remains5dc2c256 pending disk headroom.
Private request evidence is /root/dob-api-review-20261025 on the original host;
source/test evidence is /root/dob-audit-followup-20261025 on the recovery host.
