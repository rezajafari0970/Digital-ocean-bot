# Provider Parity Matrix

Status: READY / PARTIAL / BLOCKED / NOT-APPLICABLE

|#|Area|DigitalOcean|Vultr|Gap|
|---:|---|---|---|---|
|1|registration|READY|READY|none|
|2|metadata|READY|BLOCKED|Vultr status=development; UI only status=ready|
|3|credential_transport|READY|READY|shared core proxy isolation|
|4|secret_lifetime|READY|PARTIAL|Vultr materializes token string in Client|
|5|health|READY|READY|contract tests exist|
|6|account_identity|READY|BLOCKED|Vultr Account.ID empty; create requires external_id|
|7|capacity|READY|PARTIAL|Vultr LimitKnown=false|
|8|capacity_admission|READY|BLOCKED|legacy guards reject unknown limit|
|9|catalog_regions|READY|READY|implemented|
|10|catalog_plans|READY|READY|implemented|
|11|catalog_images|READY|READY|implemented|
|12|image_validation|READY|BLOCKED|hard-coded DO Ubuntu slugs reject Vultr numeric OS IDs|
|13|option_fallback|READY|BLOCKED|hard-coded DO images|
|14|pagination|READY|PARTIAL|Vultr per_page=500 without traversal|
|15|create_server|READY|READY|implemented + contract tests|
|16|ambiguous_create|READY|READY|identity adoption shared; Vultr mutation test covers ambiguity|
|17|identity_recovery|READY|READY|implemented|
|18|get_server|READY|READY|implemented|
|19|list_servers|READY|PARTIAL|Vultr pagination limitation|
|20|delete_server|READY|READY|Vultr 404 idempotence tested|
|21|ssh_key_create|READY|READY|implemented|
|22|ssh_key_delete|READY|READY|implemented|
|23|error_taxonomy|READY|PARTIAL|Vultr taxonomy smaller; no explicit account-locked/capacity/image-unavailable mapping|
|24|observe|READY|PARTIAL|Vultr identity/capacity/pagination gaps propagate|
|25|snapshot|READY|READY|shared|
|26|resource_mirror|READY|BLOCKED|provider hard-coded digitalocean|
|27|operation_ledger|READY|READY|provider-neutral despite legacy names|
|28|scheduler|READY|PARTIAL|shared scheduler supports unknown; StartDeployment guard blocks Vultr|
|29|lifecycle_rotation|READY|BLOCKED|unknown capacity treated as blocked|
|30|provisioning|READY|READY|provider-neutral after server IP|
|31|panel_registration|READY|READY|depends on internal droplet_id not provider name|
|32|output|READY|READY|provider-neutral downstream|
|33|ui_provider_select|READY|BLOCKED|filters status=ready|
|34|ui_region|READY|BLOCKED|fallback uses regs[0].slug instead of ID|
|35|ui_errors|PARTIAL|PARTIAL|DO-specific error names/text remain|
|36|live_certification|READY|BLOCKED|DO certified/runtime evidence; no Vultr account/credential yet|
