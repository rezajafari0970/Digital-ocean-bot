# AWS discovery form correction — 2026-10-24

Baseline source ba81bf2a82438d52655bce46769fd346f0a6772f; baseline deployed/public
272c46317edff036c32e302c8adce4f0ff3dcda7.

The complete 50-second screen recording was inspected at one-second intervals.
Its audio track is silent. It shows duplicate additional region selections,
two empty choices (Select and None), and old subnet options remaining selectable
after changing the primary region. Server-side duplicate rejection was correct;
the discovery UI lacked the uniqueness rule already used by random region fields.
The previous mobile fixture exposed only one extra region without the primary
region in its options, so it did not cover this failure.

The AWS UI adapter now normalizes all five region slots, preserves primary
ownership, disables already-used options, and clears region-owned subnet choices
when their region changes. Disabled stale subnets cannot enter another preview.
A pre-request normalization also handles restored/programmatically changed values
without change events. The generic renderer creates a placeholder only when the
provider has not already supplied an empty option. Provider credentials and
server-side rejection boundaries remain unchanged.

The mobile fixture now has all four optional slots and multiple primary regions,
and rejects any duplicate region or stale subnet reaching its preview endpoint.
It exercises primary/extra conflicts, None removal, stale-state normalization,
reload, scheduling draft preservation and explicit network consent.

Validation and rollout results will be appended after execution.
No new OpenAI API calls. Prior ledger: 3 calls, 6,809 tokens,
USD 0.05716284695 estimated; three-call cap respected.
