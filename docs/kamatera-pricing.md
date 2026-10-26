# Kamatera catalog pricing

Kamatera's authenticated CloudCLI discovery supplies regional hardware capabilities,
provider presets and Ubuntu image identities. Bounded custom presets are derived
only from those capabilities. `DetailsByRegion` identifies the CPU type, preset
origin and hourly billing mode. Selection IDs and mutation behavior are unchanged.

Pricing is optional enrichment from the live [official calculator](https://www.kamatera.com/pricing/).
The reader parses the JSON in `calculation-js-js-extra` and the first IPv4 option;
it never executes JavaScript. CPU, RAM and primary disk prices are exact component
lookups, including signed RAM adjustments. Every selectable Ubuntu image must
match the public image code or ID, region and a zero licence fee. Missing,
ambiguous or paid image metadata leaves the price unavailable.

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
with a five-second maximum and two seconds reserved in a bounded parent deadline.
There is no hardcoded price fallback or new mutation. A public-site outage leaves
native account validation and plan availability intact. Existing catalog caching
preserves the tariff observation time; cache version v3 invalidates older catalogs.

Verification includes exact regional totals, negative adjustments, missing/null
prices, duplicate JSON keys and lookup entries, licences, unknown regions/images,
credential-free routing, redirects, cancellation, and native catalog success when
pricing fails. Mobile browser coverage exercises both New and Edit forms, regional
price changes, unavailable plans, the single-save guard and late response isolation.
