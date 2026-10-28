# Kamatera catalog pricing

Kamatera's authenticated CloudCLI discovery supplies regional hardware capabilities,
provider presets and Ubuntu image identities. Bounded custom presets are derived
only from those capabilities. `DetailsByRegion` identifies the CPU type, preset
origin and hourly billing mode. Selection IDs and mutation behavior are unchanged.

Pricing is optional enrichment from the live [official calculator](https://www.kamatera.com/pricing/).
The reader parses the JSON in `calculation-js-js-extra` and the first IPv4 option;
it never executes JavaScript. CPU, RAM and primary disk prices are exact component
lookups, including signed RAM adjustments. Every selectable Ubuntu image must
match the public image code or ID and a zero licence fee. If only the public
regional mapping is missing, a bounded authenticated GET to the official native
configuration endpoint may verify that exact regional image ID and its explicit
zero licencePrice. Missing, ambiguous, conflicting or paid evidence leaves the
price unavailable. Public hardware and regional tariff entries remain mandatory.

The calculator's powered-on formula adds the component monthly equivalents and
divides by 730. `PricesByRegion` carries USD base hourly estimates, the unrounded
730-hour equivalent, components, and scope `kamatera-public-base-hourly-v1`.
These are public estimates, **not account quotations, monthly caps or invoices**.
The UI makes the base scope explicit, including an extra CPU usage qualifier for
Type T. Tax, discounts, traffic and Type T usage charges are excluded. Powered-off
storage/IP charges are separate. The driver disables backup and managed hosting.

CloudCLI clears `monthlypackage` for hourly creation. Consequently the UI never
claims a monthly 5 TB/1 TB package is included in hourly server prices. See the
[official hourly traffic explanation](https://www.kamatera.com/faq/answer/why-doesnt-the-dedicated-hourly-plan-include-internet-traffic/)
and Kamatera/cloudcli-server `ProxyServerPostProcessingMethods.php`.

The public request uses the account's existing proxy transport, without provider
headers or a cookie jar, and refuses redirects. It runs alongside capability reads,
with a five-second attempt limit and at most one retry for transient transport,
read or HTTP 5xx failures. The total budget is twelve seconds, with two seconds
reserved in a bounded parent deadline. Redirects, authorization errors and invalid
price data are not retried. Regional reads retain three bounded concurrent bundles.
There is no hardcoded price fallback or new mutation. A public-site outage leaves
native account validation and plan availability intact. Existing catalog caching
preserves the tariff observation time; cache version v5 invalidates older catalogs.

Verification includes exact regional totals, negative adjustments, missing/null
prices, duplicate JSON keys and lookup entries, licences, unknown regions/images,
credential-free routing, redirects, cancellation, and native catalog success when
pricing fails. Mobile browser coverage exercises both New and Edit forms, regional
price changes, unavailable plans, the single-save guard and late response isolation.

A deployed CloudCLI empty-account inventory returns HTTP 500 with the exact
message `No servers found (name='.*')`. Only that wildcard-read response triggers
a second, authenticated basic inventory read. A valid empty array confirms an
empty inventory; contradictory, malformed or failed basic reads remain errors.
Other 500 responses never establish absence. Exact-ID disappearance confirmation,
create-intent replay protection and deletion ownership checks remain required.

The native licence fallback uses the same account proxy, pinned console.kamatera.com
host, native API headers, no cookie jar and no redirects. One read per affected
region is shared across its plans; each read has a five-second limit and all such
reads share an eight-second catalog budget. A failure is advisory only. This does
not create a resource or substitute another region's price.
