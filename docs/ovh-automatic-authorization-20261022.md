# OVH automatic authorization — 2026-10-22

The New account wizard defaults to automatic OVH authorization. Enter Application key and Application secret; the driver requests a Consumer Key with the project's read and instance create/delete permissions. The operator must approve the official OVH validation URL. The panel observes activation, checks the actual granted rights, discovers projects and loads the existing region/plan/image wizard. Existing manual three-key onboarding remains available.

The Application key alone cannot authenticate signed account requests. OVH login/approval is not bypassed. This feature does not automatically create a Public Cloud project or purchase resources. Saving the reviewed account retains the existing automation behavior.

Implementation: separate optional provider AuthorizationFactory interface; official endpoint allowlists and no redirects; durable request intent before POST; ambiguous outcomes never retried automatically; encrypted owner/request-bound pending data, 30-minute local expiry, key rotation retention checks, owner/admin authorization, rate limiting and shared database locks. Local cancellation clears pending data; it does not revoke an already approved key at OVH. Account creation, encrypted credential transfer, schedules and the consumed authorization receipt commit atomically. Browser storage contains only an opaque request UUID; Consumer Keys never enter browser JSON or the DOM. Interrupted requests and page reloads resume observation.

Validation before release: 125 mandatory database/race/native regressions passed with zero skips, seven new authorization regressions included. Mobile Chromium exercised lost start response, close/reopen, full reload, automatic activation/discovery and account save, as well as existing AWS onboarding. Focused Go vet and JS syntax checks passed. Real OVH activation requires the owner's approval and has not been claimed as tested.

Bounded OpenAI API review: gpt-5.6-sol, two requests. First reached its output limit without a review (13,725 input + 2,200 output = 15,925 tokens). Follow-up completed and reported no concrete P1/P2 defects (13,725 input + 1,345 output = 15,070 tokens). Total: 27,450 input + 3,545 output = 30,995 tokens. No real credentials were included.

Release and installation evidence will be appended after both production installations are verified.
