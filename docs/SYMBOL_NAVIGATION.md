# Symbol Navigation Index

Generated from source: **1534 symbols**, **61 HTTP routes**. Full machine-readable index: `docs/SYMBOL_INDEX.json`.

## HTTP route → handler definitions

| Method | Path | Handler | Definition |
|---|---|---|---|
| GET | `/healthz` | `health` | `internal/adminapi/server.go:126` |
| GET | `/readyz` | `ready` | `internal/adminapi/server.go:129` |
| POST | `/api/v1/auth/login` | `login` | `internal/adminapi/auth.go:19` |
| POST | `/api/v1/auth/logout` | `logout` | `internal/adminapi/auth.go:40` |
| POST | `/api/v1/accounts` | `createAccount` | `internal/adminapi/accounts_write.go:40` |
| POST | `/api/v1/accounts/preview` | `accountPreview` | `internal/adminapi/account_preview.go:27` |
| PUT | `/api/v1/accounts/{id}` | `updateAccount` | `internal/adminapi/accounts_manage.go:33` |
| DELETE | `/api/v1/accounts/{id}` | `deleteAccount` | `internal/adminapi/accounts_manage.go:234` |
| GET | `/api/v1/accounts/{id}/options` | `accountOptions` | `internal/adminapi/account_options.go:10` |
| POST | `/api/v1/accounts/{id}/identity` | `accountIdentity` | `internal/adminapi/account_identity.go:8` |
| POST | `/api/v1/accounts/{id}/preflight` | `accountPreflight` | `internal/adminapi/account_preflight.go:11` |
| POST | `/api/v1/accounts/{id}/console-session` | `createVultrBrowserTicket` | `internal/adminapi/server.go:164` |
| GET | `/vultr-browser/{path...}` | `vultrBrowserProxy` | `internal/adminapi/server.go:138` |
| GET | `/websockify` | `vultrBrowserProxy` | `internal/adminapi/server.go:138` |
| POST | `/api/v1/proxies` | `createProxy` | `internal/adminapi/proxies_write.go:31` |
| PUT | `/api/v1/proxies/{id}` | `updateProxy` | `internal/adminapi/proxies_manage.go:8` |
| DELETE | `/api/v1/proxies/{id}` | `deleteProxy` | `internal/adminapi/proxies_manage.go:67` |
| GET | `/api/v1/proxies/{id}` | `proxyDetails` | `internal/adminapi/proxy_details.go:8` |
| GET | `/api/v1/residential-proxies` | `residentialProxies` | `internal/adminapi/residential.go:20` |
| POST | `/api/v1/residential-proxies` | `createResidentialProxy` | `internal/adminapi/residential.go:39` |
| PUT | `/api/v1/residential-proxies/{id}` | `updateResidentialProxy` | `internal/adminapi/residential.go:96` |
| DELETE | `/api/v1/residential-proxies/{id}` | `deleteResidentialProxy` | `internal/adminapi/residential.go:133` |
| POST | `/api/v1/residential-proxies/{id}/test` | `testResidentialProxy` | `internal/adminapi/residential.go:94` |
| GET | `/api/v1/configs` | `getGlobalConfigs` | `internal/adminapi/configs_global.go:58` |
| GET | `/api/v1/config-capacity` | `configCapacity` | `internal/adminapi/config_capacity.go:7` |
| POST | `/api/v1/config-capacity/delete-all-clients` | `deleteAllCapacityClients` | `internal/adminapi/capacity_cleanup.go:44` |
| GET | `/api/v1/config-capacity/cleanup/{id}` | `cleanupJobStatus` | `internal/adminapi/capacity_cleanup.go:84` |
| GET | `/api/v1/output` | `outputConfigs` | `internal/adminapi/output.go:285` |
| POST | `/api/v1/output/share` | `createOutputShare` | `internal/adminapi/output_snapshot.go:63` |
| GET | `/share/output/{token}` | `sharedOutput` | `internal/adminapi/output_snapshot.go:75` |
| GET | `/api/v1/config-panels` | `getConfigPanels` | `internal/adminapi/configs.go:73` |
| PUT | `/api/v1/configs` | `putGlobalConfig` | `internal/adminapi/configs_global.go:20` |
| POST | `/api/v1/proxies/{id}/test` | `testProxy` | `internal/adminapi/proxy_details.go:24` |
| PUT | `/api/v1/accounts/{id}/network` | `assignProxy` | `internal/adminapi/proxies_write.go:80` |
| GET | `/api/v1/accounts/{id}/discovery` | `accountDiscovery` | `internal/adminapi/discovery.go:12` |
| POST | `/api/v1/accounts/{id}/refresh` | `accountDiscovery` | `internal/adminapi/discovery.go:12` |
| POST | `/api/v1/accounts/{id}/proxy-test` | `testAccountProxy` | `internal/adminapi/proxy_health.go:12` |
| GET | `/api/v1/templates` | `listTemplates` | `internal/adminapi/templates.go:14` |
| POST | `/api/v1/templates` | `uploadTemplate` | `internal/adminapi/templates.go:37` |
| GET | `/api/v1/accounts/{id}/dashboard` | `accountDashboard` | `internal/adminapi/dashboard.go:13` |
| GET | `/api/v1/accounts/{id}/resources` | `accountResources` | `internal/adminapi/resources_api.go:9` |
| GET | `/api/v1/accounts/{id}/capacity` | `accountCapacity` | `internal/adminapi/resources_api.go:39` |
| GET | `/api/v1/accounts/{id}/runtime` | `accountRuntime` | `internal/adminapi/runtime_api.go:5` |
| GET | `/api/v1/accounts` | `accounts` | `internal/adminapi/resources.go:10` |
| GET | `/api/v1/proxies` | `proxies` | `internal/adminapi/resources.go:151` |
| GET | `/api/v1/build-activity` | `buildActivity` | `internal/adminapi/build_activity.go:8` |
| GET | `/api/v1/build-activity/{id}/provision` | `provisionActivity` | `internal/adminapi/provision_activity.go:7` |
| GET | `/api/v1/provision-errors` | `provisionErrors` | `internal/adminapi/provision_errors.go:5` |
| POST | `/api/v1/installers/sanaei/preview` | `previewSanaeiInstaller` | `internal/adminapi/sanaei_installer.go:26` |
| POST | `/api/v1/installers/sanaei` | `createSanaeiInstaller` | `internal/adminapi/sanaei_installer.go:11` |
| GET | `/api/v1/installers` | `listInstallers` | `internal/adminapi/installers.go:9` |
| POST | `/api/v1/installers` | `createInstaller` | `internal/adminapi/installers.go:31` |
| GET | `/api/v1/install-scripts` | `listInstallScripts` | `internal/adminapi/install_scripts.go:10` |
| POST | `/api/v1/install-scripts` | `createInstallScript` | `internal/adminapi/install_scripts.go:29` |
| POST | `/api/v1/deployments/{id}/installer` | `selectDeploymentInstaller` | `internal/adminapi/installer_selection.go:8` |
| POST | `/api/v1/deployments/{id}/installer/rearm` | `rearmDeploymentInstaller` | `internal/adminapi/installer_rearm.go:9` |
| GET | `/api/v1/audit` | `audit` | `internal/adminapi/status.go:5` |
| GET | `/api/v1/system` | `system` | `internal/adminapi/status.go:25` |
| GET | `/api/v1/providers` | `providersMetadata` | `internal/adminapi/providers.go:5` |
| GET | `/api/v1/panel-settings` | `getPanelSettings` | `internal/adminapi/panel_settings.go:18` |
| PUT | `/api/v1/panel-settings` | `updatePanelSettings` | `internal/adminapi/panel_settings.go:26` |

## Package `adminapi` key exported symbols

- `struct previewAccount` — `internal/adminapi/account_preview.go:16`
- `struct previewCredential` — `internal/adminapi/account_preview.go:21`
- `method Get` — `internal/adminapi/account_preview.go:23`
- `struct accountUpdate` — `internal/adminapi/accounts_manage.go:8`
- `struct accountWrite` — `internal/adminapi/accounts_write.go:12`
- `struct principalKey` — `internal/adminapi/auth.go:12`
- `struct loginRequest` — `internal/adminapi/auth.go:13`
- `struct cleanupInbound` — `internal/adminapi/capacity_cleanup.go:15`
- `struct cleanupJob` — `internal/adminapi/capacity_cleanup.go:26`
- `struct cleanupResult` — `internal/adminapi/capacity_cleanup.go:38`
- `struct item` — `internal/adminapi/capacity_cleanup.go:170`
- `func TestConfigExpressions` — `internal/adminapi/config_expr_test.go:5`
- `struct configRequest` — `internal/adminapi/configs.go:9`
- `struct globalConfigRequest` — `internal/adminapi/configs_global.go:8`
- `struct accountDashboard` — `internal/adminapi/dashboard.go:13`
- `struct managedServerInfo` — `internal/adminapi/dashboard.go:216`
- `struct result` — `internal/adminapi/network_identity_auto.go:17`
- `struct outputInbound` — `internal/adminapi/output.go:16`
- `struct outputCacheEntry` — `internal/adminapi/output.go:26`
- `struct outputRecord` — `internal/adminapi/output.go:31`
- `method WarmOutputCache` — `internal/adminapi/output.go:128`
- `func TestOutputVisibleUntilTenSecondsBeforeExpiry` — `internal/adminapi/output_expiry_test.go:8`
- `func TestOutputVisibleUntilNoExpiry` — `internal/adminapi/output_expiry_test.go:17`
- `struct panelSettings` — `internal/adminapi/panel_settings.go:12`
- `struct proxyWrite` — `internal/adminapi/proxies_write.go:10`
- `struct assignProxy` — `internal/adminapi/proxies_write.go:80`
- `struct ProxyObservation` — `internal/adminapi/proxy_observe.go:13`
- `func ObserveProxy` — `internal/adminapi/proxy_observe.go:26`
- `struct residentialWrite` — `internal/adminapi/residential.go:9`
- `struct loginBucket` — `internal/adminapi/security.go:9`
- `struct LoginLimiter` — `internal/adminapi/security.go:14`
- `func NewLoginLimiter` — `internal/adminapi/security.go:22`
- `method Allow` — `internal/adminapi/security.go:25`
- `func Secure` — `internal/adminapi/security.go:57`
- `struct Server` — `internal/adminapi/server.go:21`
- `func New` — `internal/adminapi/server.go:41`
- `method Routes` — `internal/adminapi/server.go:44`
- `func TestHealthEndpoint` — `internal/adminapi/server_test.go:8`
- `func TestReadinessWithoutDatabase` — `internal/adminapi/server_test.go:17`

## Package `providers` key exported symbols

- `struct Error` — `internal/providers/errors.go:27`
- `method Error` — `internal/providers/errors.go:36`
- `method Unwrap` — `internal/providers/errors.go:48`
- `func Class` — `internal/providers/errors.go:55`
- `func IsClass` — `internal/providers/errors.go:65`
- `func IsRetryable` — `internal/providers/errors.go:66`
- `struct ImagePolicy` — `internal/providers/metadata.go:3`
- `struct Defaults` — `internal/providers/metadata.go:8`
- `struct Metadata` — `internal/providers/metadata.go:21`
- `struct Capabilities` — `internal/providers/provider.go:8`
- `interface Driver` — `internal/providers/provider.go:16`
- `interface AccountReader` — `internal/providers/provider.go:22`
- `interface CatalogReader` — `internal/providers/provider.go:27`
- `interface ComputeDriver` — `internal/providers/provider.go:31`
- `interface SSHKeyDriver` — `internal/providers/provider.go:39`
- `interface InventoryReader` — `internal/providers/provider.go:44`
- `interface ObservationReader` — `internal/providers/provider.go:48`
- `interface CredentialSource` — `internal/providers/provider.go:52`
- `struct OpenRequest` — `internal/providers/provider.go:56`
- `interface Factory` — `internal/providers/provider.go:62`
- `func CanCreateCapacity` — `internal/providers/provider.go:68`
- `struct testDriver` — `internal/providers/provider_v2_test.go:11`
- `method Name` — `internal/providers/provider_v2_test.go:13`
- `method Capabilities` — `internal/providers/provider_v2_test.go:14`
- `method Health` — `internal/providers/provider_v2_test.go:15`
- `struct testFactory` — `internal/providers/provider_v2_test.go:17`
- `method Name` — `internal/providers/provider_v2_test.go:23`
- `method Metadata` — `internal/providers/provider_v2_test.go:24`
- `method Open` — `internal/providers/provider_v2_test.go:27`
- `func TestRegistryNormalizesAndOpens` — `internal/providers/provider_v2_test.go:34`
- `func TestRegistryRejectsDuplicate` — `internal/providers/provider_v2_test.go:55`
- `func TestRegistryNamesSorted` — `internal/providers/provider_v2_test.go:65`
- `func TestProviderErrorTaxonomy` — `internal/providers/provider_v2_test.go:81`
- `func TestCapabilityInterfacesRemainIndependent` — `internal/providers/provider_v2_test.go:101`
- `struct Registry` — `internal/providers/registry.go:10`
- `func NewRegistry` — `internal/providers/registry.go:15`
- `method Register` — `internal/providers/registry.go:19`
- `method Has` — `internal/providers/registry.go:36`
- `method Names` — `internal/providers/registry.go:43`
- `method Metadata` — `internal/providers/registry.go:54`
- `method MetadataAll` — `internal/providers/registry.go:63`
- `method Open` — `internal/providers/registry.go:74`
- `struct Account` — `internal/providers/types.go:16`
- `struct Capacity` — `internal/providers/types.go:22`
- `struct Region` — `internal/providers/types.go:29`
- `struct Plan` — `internal/providers/types.go:35`
- `struct Image` — `internal/providers/types.go:47`
- `struct Catalog` — `internal/providers/types.go:56`
- `struct Server` — `internal/providers/types.go:62`
- `struct CreateServerRequest` — `internal/providers/types.go:74`
- `struct CreateServerResult` — `internal/providers/types.go:92`
- `struct SSHKey` — `internal/providers/types.go:98`
- `struct Inventory` — `internal/providers/types.go:104`
- `struct Resource` — `internal/providers/types.go:110`
- `struct Observation` — `internal/providers/types.go:119`

## Package `digitalocean` key exported symbols

- `struct AccountInfo` — `internal/providers/digitalocean/account.go:5`
- `struct Limits` — `internal/providers/digitalocean/account.go:15`
- `struct accountEnvelope` — `internal/providers/digitalocean/account.go:20`
- `method GetAccount` — `internal/providers/digitalocean/account.go:24`
- `method GetLimits` — `internal/providers/digitalocean/account.go:31`
- `method Catalog` — `internal/providers/digitalocean/catalog.go:5`
- `func TestGetLimitsMirrorsAccountLimits` — `internal/providers/digitalocean/characterization_test.go:31`
- `func TestCatalogFetchesAccountRegionsSizesImages` — `internal/providers/digitalocean/characterization_test.go:47`
- `func TestDiscoverIsFailFast` — `internal/providers/digitalocean/characterization_test.go:85`
- `func TestDropletMutationAndReadCharacterization` — `internal/providers/digitalocean/characterization_test.go:110`
- `func TestSSHKeyAndActionCharacterization` — `internal/providers/digitalocean/characterization_test.go:142`
- `interface SecretReader` — `internal/providers/digitalocean/client.go:19`
- `struct Client` — `internal/providers/digitalocean/client.go:23`
- `func NewClient` — `internal/providers/digitalocean/client.go:31`
- `method Validate` — `internal/providers/digitalocean/client.go:38`
- `struct secretStub` — `internal/providers/digitalocean/client_test.go:12`
- `method Get` — `internal/providers/digitalocean/client_test.go:14`
- `method RoundTrip` — `internal/providers/digitalocean/client_test.go:20`
- `func TestClientLoadsTokenAndDecodesAccount` — `internal/providers/digitalocean/client_test.go:22`
- `struct Region` — `internal/providers/digitalocean/discovery.go:5`
- `struct Size` — `internal/providers/digitalocean/discovery.go:14`
- `struct Image` — `internal/providers/digitalocean/discovery.go:25`
- `struct Droplet` — `internal/providers/digitalocean/discovery.go:33`
- `struct Project` — `internal/providers/digitalocean/discovery.go:48`
- `struct SSHKey` — `internal/providers/digitalocean/discovery.go:54`
- `struct Firewall` — `internal/providers/digitalocean/discovery.go:59`
- `struct DiscoveryResult` — `internal/providers/digitalocean/discovery.go:65`
- `method Discover` — `internal/providers/digitalocean/discovery.go:77`
- `struct Driver` — `internal/providers/digitalocean/driver_v2.go:15`
- `struct credentialBridge` — `internal/providers/digitalocean/driver_v2.go:17`
- `method Get` — `internal/providers/digitalocean/driver_v2.go:19`
- `struct Factory` — `internal/providers/digitalocean/driver_v2.go:23`
- `method Name` — `internal/providers/digitalocean/driver_v2.go:25`
- `method Metadata` — `internal/providers/digitalocean/driver_v2.go:26`
- `method Open` — `internal/providers/digitalocean/driver_v2.go:29`
- `func NewDriver` — `internal/providers/digitalocean/driver_v2.go:40`
- `method Name` — `internal/providers/digitalocean/driver_v2.go:47`
- `method Capabilities` — `internal/providers/digitalocean/driver_v2.go:48`
- `method Health` — `internal/providers/digitalocean/driver_v2.go:51`
- `method Account` — `internal/providers/digitalocean/driver_v2.go:55`
- `method Capacity` — `internal/providers/digitalocean/driver_v2.go:62`
- `method Catalog` — `internal/providers/digitalocean/driver_v2.go:73`
- `method CreateServer` — `internal/providers/digitalocean/driver_v2.go:80`
- `method GetServer` — `internal/providers/digitalocean/driver_v2.go:104`
- `method ListServers` — `internal/providers/digitalocean/driver_v2.go:115`
- `method DeleteServer` — `internal/providers/digitalocean/driver_v2.go:126`
- `method FindServerByIdentity` — `internal/providers/digitalocean/driver_v2.go:133`
- `method CreateSSHKey` — `internal/providers/digitalocean/driver_v2.go:146`
- `method DeleteSSHKey` — `internal/providers/digitalocean/driver_v2.go:153`
- `method Inventory` — `internal/providers/digitalocean/driver_v2.go:160`
- `method Observe` — `internal/providers/digitalocean/driver_v2.go:168`
- `func TestNormalizeCatalogToCanonicalModels` — `internal/providers/digitalocean/driver_v2_test.go:12`
- `func TestNormalizeServerUsesStringIDAndReadiness` — `internal/providers/digitalocean/driver_v2_test.go:25`
- `func TestReadyRequiresPublicIPv4` — `internal/providers/digitalocean/driver_v2_test.go:32`
- `func TestNormalizeProviderErrors` — `internal/providers/digitalocean/driver_v2_test.go:38`
- `func TestDriverFindServerByIdentity` — `internal/providers/digitalocean/driver_v2_test.go:60`
- `func TestFactoryMetadataPolicy` — `internal/providers/digitalocean/driver_v2_test.go:77`
- `func TestDigitalOceanCapacityLimitIsKnown` — `internal/providers/digitalocean/driver_v2_test.go:88`
- `struct HTTPError` — `internal/providers/digitalocean/errors.go:13`
- `method Error` — `internal/providers/digitalocean/errors.go:21`
- `func IsCapacityError` — `internal/providers/digitalocean/errors.go:40`
- `func ClassifyError` — `internal/providers/digitalocean/errors.go:55`
- `func TestParseAPIError` — `internal/providers/digitalocean/errors_parse_test.go:5`
- `func TestParseAPIErrorUsesCode` — `internal/providers/digitalocean/errors_parse_test.go:12`
- `func TestParseAPIErrorRejectsNonJSON` — `internal/providers/digitalocean/errors_parse_test.go:19`
- `func TestCapacityClassification` — `internal/providers/digitalocean/errors_test.go:10`
- `struct CreateDropletRequest` — `internal/providers/digitalocean/mutations.go:14`
- `struct SSHKeyCreateRequest` — `internal/providers/digitalocean/mutations.go:25`
- `struct SSHKeyCreated` — `internal/providers/digitalocean/mutations.go:29`
- `method CreateSSHKey` — `internal/providers/digitalocean/mutations.go:34`
- `method DeleteSSHKey` — `internal/providers/digitalocean/mutations.go:41`
- `struct Action` — `internal/providers/digitalocean/mutations.go:45`
- `struct createDropletEnvelope` — `internal/providers/digitalocean/mutations.go:50`
- `struct actionEnvelope` — `internal/providers/digitalocean/mutations.go:54`
- `method CreateDroplet` — `internal/providers/digitalocean/mutations.go:111`
- `method DeleteDroplet` — `internal/providers/digitalocean/mutations.go:116`
- `method GetAction` — `internal/providers/digitalocean/mutations.go:119`
- `func TestExtractNextStripsAPIVersionPrefix` — `internal/providers/digitalocean/pagination_regression_test.go:8`
- `struct Resource` — `internal/providers/digitalocean/resources.go:9`
- `method ListResources` — `internal/providers/digitalocean/resources.go:15`

## Package `vultr` key exported symbols

- `struct HTTPError` — `internal/providers/vultr/client.go:21`
- `method Error` — `internal/providers/vultr/client.go:27`
- `struct Client` — `internal/providers/vultr/client.go:29`
- `func NewClient` — `internal/providers/vultr/client.go:36`
- `func NewClientFromSource` — `internal/providers/vultr/client.go:39`
- `func TestClientAuthorizationAndHealth` — `internal/providers/vultr/client_test.go:10`
- `func TestGET429RetriesButMutationDoesNot` — `internal/providers/vultr/client_test.go:30`
- `func TestInstancesFollowsPaginationNext` — `internal/providers/vultr/client_test.go:61`
- `struct Factory` — `internal/providers/vultr/driver.go:13`
- `method Name` — `internal/providers/vultr/driver.go:15`
- `method Metadata` — `internal/providers/vultr/driver.go:16`
- `method Open` — `internal/providers/vultr/driver.go:19`
- `struct Driver` — `internal/providers/vultr/driver.go:35`
- `method Name` — `internal/providers/vultr/driver.go:37`
- `method Capabilities` — `internal/providers/vultr/driver.go:38`
- `method Health` — `internal/providers/vultr/driver.go:41`
- `func TestFactoryMetadata` — `internal/providers/vultr/driver_test.go:9`
- `func TestErrorTaxonomy` — `internal/providers/vultr/driver_test.go:18`
- `struct createInstanceRequest` — `internal/providers/vultr/mutation.go:8`
- `method CreateInstance` — `internal/providers/vultr/mutation.go:21`
- `method DeleteInstance` — `internal/providers/vultr/mutation.go:28`
- `method CreateSSHKey` — `internal/providers/vultr/mutation.go:31`
- `method DeleteSSHKey` — `internal/providers/vultr/mutation.go:38`
- `method CreateServer` — `internal/providers/vultr/mutation_driver.go:14`
- `method DeleteServer` — `internal/providers/vultr/mutation_driver.go:50`
- `method FindServerByIdentity` — `internal/providers/vultr/mutation_driver.go:61`
- `method CreateSSHKey` — `internal/providers/vultr/mutation_driver.go:74`
- `method DeleteSSHKey` — `internal/providers/vultr/mutation_driver.go:101`
- `func TestMutationContracts` — `internal/providers/vultr/mutation_driver_test.go:15`
- `func TestCreate429IsAmbiguousWithRetryAfter` — `internal/providers/vultr/mutation_driver_test.go:76`
- `func TestDelete404IsIdempotent` — `internal/providers/vultr/mutation_driver_test.go:95`
- `method Account` — `internal/providers/vultr/read.go:11`
- `method Regions` — `internal/providers/vultr/read.go:16`
- `method Plans` — `internal/providers/vultr/read.go:35`
- `method OS` — `internal/providers/vultr/read.go:54`
- `method Instances` — `internal/providers/vultr/read.go:73`
- `method Instance` — `internal/providers/vultr/read.go:92`
- `method SSHKey` — `internal/providers/vultr/read.go:100`
- `method RegionAvailability` — `internal/providers/vultr/read.go:108`
- `method Account` — `internal/providers/vultr/read_driver.go:16`
- `method Capacity` — `internal/providers/vultr/read_driver.go:28`
- `method Catalog` — `internal/providers/vultr/read_driver.go:35`
- `method GetServer` — `internal/providers/vultr/read_driver.go:112`
- `method ListServers` — `internal/providers/vultr/read_driver.go:119`
- `method Inventory` — `internal/providers/vultr/read_driver.go:130`
- `method Observe` — `internal/providers/vultr/read_driver.go:137`
- `func TestReadContracts` — `internal/providers/vultr/read_driver_test.go:38`
- `func TestNormalizeUbuntuVersions` — `internal/providers/vultr/read_driver_test.go:68`
- `struct paginationMeta` — `internal/providers/vultr/types.go:3`
- `struct accountResponse` — `internal/providers/vultr/types.go:9`
- `struct region` — `internal/providers/vultr/types.go:17`
- `struct regionsResponse` — `internal/providers/vultr/types.go:24`
- `struct plan` — `internal/providers/vultr/types.go:28`
- `struct plansResponse` — `internal/providers/vultr/types.go:38`
- `struct osItem` — `internal/providers/vultr/types.go:42`
- `struct osResponse` — `internal/providers/vultr/types.go:48`
- `struct instance` — `internal/providers/vultr/types.go:52`
- `struct instancesResponse` — `internal/providers/vultr/types.go:70`
- `struct sshKey` — `internal/providers/vultr/types.go:74`
- `struct sshKeysResponse` — `internal/providers/vultr/types.go:80`

## Package `vultrconsole` key exported symbols

- `struct BrowserObserver` — `internal/providers/vultrconsole/browser.go:18`
- `method Observe` — `internal/providers/vultrconsole/browser.go:23`
- `func TestMaximumInstancesPattern` — `internal/providers/vultrconsole/browser_test.go:5`
- `struct ProxyBridge` — `internal/providers/vultrconsole/proxy_bridge.go:15`
- `method Start` — `internal/providers/vultrconsole/proxy_bridge.go:21`
- `method Close` — `internal/providers/vultrconsole/proxy_bridge.go:31`

## Package `sanaei` key exported symbols

- `struct Credentials` — `internal/panels/sanaei/api.go:17`
- `struct APIClient` — `internal/panels/sanaei/api.go:21`
- `func NewAPIClient` — `internal/panels/sanaei/api.go:28`
- `method Login` — `internal/panels/sanaei/api.go:39`
- `struct failingTransport` — `internal/panels/sanaei/api_error_test.go:11`
- `method RoundTrip` — `internal/panels/sanaei/api_error_test.go:13`
- `func TestLoginPreservesTransportCause` — `internal/panels/sanaei/api_error_test.go:14`
- `struct BearerAuth` — `internal/panels/sanaei/auth.go:13`
- `method Authorize` — `internal/panels/sanaei/auth.go:20`
- `struct SessionCredential` — `internal/panels/sanaei/auth.go:59`
- `func ValidateSessionTransport` — `internal/panels/sanaei/auth.go:64`
- `method Get` — `internal/panels/sanaei/auth_test.go:11`
- `func TestBearerAuthReadsSecretAndSetsHeader` — `internal/panels/sanaei/auth_test.go:23`
- `func TestSessionCredentialsRefusePlainHTTP` — `internal/panels/sanaei/auth_test.go:60`
- `func AddClientsSession` — `internal/panels/sanaei/client_mutation.go:14`
- `func DeleteClientSession` — `internal/panels/sanaei/client_mutation.go:57`
- `struct Client` — `internal/panels/sanaei/clients.go:13`
- `struct ClientRecord` — `internal/panels/sanaei/clients.go:22`
- `interface ClientStore` — `internal/panels/sanaei/clients.go:32`
- `struct ClientManager` — `internal/panels/sanaei/clients.go:36`
- `method CreateMany` — `internal/panels/sanaei/clients.go:41`
- `func UUIDv4` — `internal/panels/sanaei/clients.go:65`
- `func TestUUIDv4` — `internal/panels/sanaei/clients_test.go:8`
- `struct DatabasePaths` — `internal/panels/sanaei/database.go:5`
- `func DefaultDatabasePaths` — `internal/panels/sanaei/database.go:11`
- `func ImportCommand` — `internal/panels/sanaei/database.go:15`
- `struct DirectSessionExecutor` — `internal/panels/sanaei/direct_session.go:11`
- `method Do` — `internal/panels/sanaei/direct_session.go:13`
- `interface AuthProvider` — `internal/panels/sanaei/driver.go:13`
- `struct Driver` — `internal/panels/sanaei/driver.go:17`
- `method Name` — `internal/panels/sanaei/driver.go:22`
- `method Discover` — `internal/panels/sanaei/driver.go:53`
- `method Health` — `internal/panels/sanaei/driver.go:78`
- `func DiscoverWithExecutor` — `internal/panels/sanaei/driver.go:92`
- `func TestDriverDiscoveryClassifiesRuntimeCapabilities` — `internal/panels/sanaei/driver_test.go:11`
- `func TestDiscoverWithExecutorIsReadOnly` — `internal/panels/sanaei/driver_test.go:37`
- `func GetInbound` — `internal/panels/sanaei/inbound_get.go:12`
- `struct Inbound` — `internal/panels/sanaei/inbounds.go:11`
- `struct inboundListResponse` — `internal/panels/sanaei/inbounds.go:18`
- `struct genericResponse` — `internal/panels/sanaei/inbounds.go:23`
- `method ListInbounds` — `internal/panels/sanaei/inbounds.go:28`
- `method AddClient` — `internal/panels/sanaei/inbounds.go:39`
- `method AddClients` — `internal/panels/sanaei/inbounds.go:43`
- `method DeleteClient` — `internal/panels/sanaei/inbounds.go:62`
- `func VerifyCommand` — `internal/panels/sanaei/installer.go:3`
- `struct InstallerDefinition` — `internal/panels/sanaei/installer_adapter.go:19`
- `func DefaultInstallerDefinition` — `internal/panels/sanaei/installer_adapter.go:32`
- `method Build` — `internal/panels/sanaei/installer_adapter.go:36`
- `func RegisterInstaller` — `internal/panels/sanaei/installer_adapter.go:67`
- `func TestInstallerAdapterBuildsImmutableCoreDefinition` — `internal/panels/sanaei/installer_adapter_test.go:10`
- `func TestInstallerAdapterRejectsUnpinnedOrInsecureArtifact` — `internal/panels/sanaei/installer_adapter_test.go:31`
- `func TestInstallerAdapterCompatibilityDefaults` — `internal/panels/sanaei/installer_adapter_test.go:39`
- `func TestLegacyInstallerCommandDoesNotBecomeAdapterSource` — `internal/panels/sanaei/installer_adapter_test.go:56`
- `struct inventoryEnvelope` — `internal/panels/sanaei/inventory.go:17`
- `struct inboundSummary` — `internal/panels/sanaei/inventory.go:22`
- `func ReadInventory` — `internal/panels/sanaei/inventory.go:44`
- `func InventoryFromRaw` — `internal/panels/sanaei/inventory.go:233`
- `func TestReadInventoryNormalizesWithoutSecrets` — `internal/panels/sanaei/inventory_test.go:8`
- `interface SecretReader` — `internal/panels/sanaei/manager.go:14`
- `interface Runner` — `internal/panels/sanaei/manager.go:17`
- `interface Uploader` — `internal/panels/sanaei/manager.go:20`
- `struct DatabaseManager` — `internal/panels/sanaei/manager.go:23`
- `method Import` — `internal/panels/sanaei/manager.go:29`
- `struct mutationEnvelope` — `internal/panels/sanaei/mutation.go:23`
- `func AddInbound` — `internal/panels/sanaei/mutation.go:31`
- `func DeleteInbound` — `internal/panels/sanaei/mutation.go:96`
- `func UpdateInbound` — `internal/panels/sanaei/mutation.go:124`
- `func UpdateInboundRaw` — `internal/panels/sanaei/mutation.go:165`
- `struct captureMutationExecutor` — `internal/panels/sanaei/mutation_test.go:12`
- `method Do` — `internal/panels/sanaei/mutation_test.go:18`
- `func TestAddInboundUsesRestrictedEndpoint` — `internal/panels/sanaei/mutation_test.go:28`
- `func TestAddInboundRejectsSuccessFalse` — `internal/panels/sanaei/mutation_test.go:112`
- `interface SecretWriterReader` — `internal/panels/sanaei/panel_configurer.go:16`
- `struct PanelConfigurer` — `internal/panels/sanaei/panel_configurer.go:20`
- `func NewPanelPassword` — `internal/panels/sanaei/panel_configurer.go:27`
- `method Configure` — `internal/panels/sanaei/panel_configurer.go:44`
- `method RepairCompleted` — `internal/panels/sanaei/panel_configurer.go:120`
- `func TestPanelDefaultsDeterministicAndScoped` — `internal/panels/sanaei/panel_configurer_test.go:9`
- `func TestPanelPasswordsAreRandomURLSafeAndNotReused` — `internal/panels/sanaei/panel_configurer_test.go:24`
- `func TestPanelConfigureCommandDoesNotContainCredential` — `internal/panels/sanaei/panel_configurer_test.go:43`

## Package `app` key exported symbols

- `func ClassifyAccountProviderError` — `internal/app/account_state.go:22`
- `func ProviderStateRuntimeStatus` — `internal/app/account_state.go:44`
- `func IsProviderObservationError` — `internal/app/account_state.go:51`
- `func ProviderProbeInterval` — `internal/app/account_state.go:54`
- `func TestClassifyAccountProviderError` — `internal/app/account_state_test.go:13`
- `func TestProviderStateRecoveryPolicy` — `internal/app/account_state_test.go:36`
- `struct Application` — `internal/app/bootstrap.go:13`
- `func Bootstrap` — `internal/app/bootstrap.go:19`
- `method Close` — `internal/app/bootstrap.go:56`
- `struct CreateCapacity` — `internal/app/capacity_guard.go:13`
- `method Available` — `internal/app/capacity_guard.go:21`
- `method RequireCreateCapacity` — `internal/app/capacity_guard.go:31`
- `func TestCreateCapacityAvailable` — `internal/app/capacity_guard_test.go:8`
- `func TestCreateCapacityUnknownLimitAllowsCreate` — `internal/app/capacity_guard_test.go:19`
- `method SyncCatalogs` — `internal/app/catalog_sync.go:11`
- `method RunDailyCatalogSync` — `internal/app/catalog_sync.go:62`
- `struct Config` — `internal/app/config.go:12`
- `func LoadConfig` — `internal/app/config.go:20`
- `method ReconcileLocalState` — `internal/app/consistency.go:11`
- `struct Container` — `internal/app/container.go:18`
- `struct AccountRuntime` — `internal/app/container.go:24`
- `struct accountCredentialSource` — `internal/app/container.go:33`
- `method Get` — `internal/app/container.go:39`
- `method Runtime` — `internal/app/container.go:65`
- `method StartDeployment` — `internal/app/deploy.go:22`
- `struct deploymentReadyFinalizer` — `internal/app/deployment_ready.go:11`
- `method MarkReady` — `internal/app/deployment_ready.go:16`
- `struct deploymentFailureFinalizer` — `internal/app/deployment_ready.go:28`
- `method MarkFailed` — `internal/app/deployment_ready.go:30`
- `method ProcessLifecycle` — `internal/app/lifecycle.go:12`
- `method ConfirmDeleted` — `internal/app/lifecycle.go:198`
- `struct result` — `internal/app/network_identity.go:19`
- `method EnsureFreshNetworkIdentity` — `internal/app/network_identity.go:59`
- `method ReconcileOwnedOrphans` — `internal/app/orphan_reconcile.go:17`
- `struct PanelSettings` — `internal/app/panel.go:8`
- `method PanelSettings` — `internal/app/panel.go:14`
- `method Addr` — `internal/app/panel.go:19`
- `method PostInstallWorkflow` — `internal/app/postinstall.go:14`
- `method RequirePostInstallCapabilities` — `internal/app/postinstall_capabilities.go:13`
- `method RearmPostInstall` — `internal/app/postinstall_rearm.go:11`
- `method ReconcileCompletedPostInstall` — `internal/app/postinstall_reconcile.go:12`
- `method DeploymentConfigFromSnapshot` — `internal/app/profile.go:16`
- `method RefreshProviderSnapshots` — `internal/app/provider_refresh.go:13`
- `method RecordProviderObservation` — `internal/app/provider_refresh.go:120`
- `struct ProxyCapabilities` — `internal/app/proxy_provider.go:5`
- `struct ProxySessionRequest` — `internal/app/proxy_provider.go:6`
- `interface ProxyProviderAdapter` — `internal/app/proxy_provider.go:10`
- `struct genericProxyAdapter` — `internal/app/proxy_provider.go:17`
- `method Name` — `internal/app/proxy_provider.go:19`
- `method Match` — `internal/app/proxy_provider.go:20`
- `method Capabilities` — `internal/app/proxy_provider.go:21`
- `method Username` — `internal/app/proxy_provider.go:22`
- `struct suffixSessionAdapter` — `internal/app/proxy_provider.go:24`
- `method Name` — `internal/app/proxy_provider.go:26`
- `method Match` — `internal/app/proxy_provider.go:27`
- `method Capabilities` — `internal/app/proxy_provider.go:30`
- `method Username` — `internal/app/proxy_provider.go:33`
- `func ProxySessionUsername` — `internal/app/proxy_provider.go:64`
- `func ProxyAdapterCapabilities` — `internal/app/proxy_provider.go:68`
- `func TestGenericDoesNotRewrite` — `internal/app/proxy_provider_test.go:5`
- `func TestSuffixAdapterPreservesPolicy` — `internal/app/proxy_provider_test.go:14`
- `struct RecoveryHandler` — `internal/app/recovery.go:14`
- `method RecoverOperation` — `internal/app/recovery.go:16`
- `method RecoverDeployment` — `internal/app/recovery.go:80`
- `method BypassDeploymentBackoff` — `internal/app/recovery.go:163`
- `struct AccountConfig` — `internal/app/repository.go:15`
- `struct Repository` — `internal/app/repository.go:26`
- `method Account` — `internal/app/repository.go:28`
- `struct ScheduledStarter` — `internal/app/scheduler.go:5`
- `method PrepareScheduledAccount` — `internal/app/scheduler.go:7`
- `method StartScheduledDeployment` — `internal/app/scheduler.go:11`
- `struct stickyGeo` — `internal/app/sticky_proxy.go:19`
- `method MaintainStickyIdentity` — `internal/app/sticky_proxy.go:82`
- `struct DeploymentConfig` — `internal/app/workflow.go:12`
- `method Workflow` — `internal/app/workflow.go:20`

## Package `droplets` key exported symbols

- `struct computeStub` — `internal/droplets/compute_stub_test.go:8`
- `method CreateServer` — `internal/droplets/compute_stub_test.go:14`
- `method GetServer` — `internal/droplets/compute_stub_test.go:21`
- `method ListServers` — `internal/droplets/compute_stub_test.go:24`
- `method DeleteServer` — `internal/droplets/compute_stub_test.go:25`
- `method FindServerByIdentity` — `internal/droplets/compute_stub_test.go:26`
- `interface MutationGate` — `internal/droplets/executor.go:14`
- `struct Executor` — `internal/droplets/executor.go:16`
- `method Create` — `internal/droplets/executor.go:23`
- `method Delete` — `internal/droplets/executor.go:90`
- `struct gateStub` — `internal/droplets/executor_test.go:9`
- `method AllowMutation` — `internal/droplets/executor_test.go:11`
- `struct opStore` — `internal/droplets/executor_test.go:13`
- `method Reserve` — `internal/droplets/executor_test.go:18`
- `method Get` — `internal/droplets/executor_test.go:27`
- `method Update` — `internal/droplets/executor_test.go:28`
- `func TestCreateIsIdempotent` — `internal/droplets/executor_test.go:30`
- `func BuildCreateOperation` — `internal/droplets/factory.go:5`
- `struct LifecycleItem` — `internal/droplets/lifecycle.go:9`
- `struct LifecycleStore` — `internal/droplets/lifecycle.go:20`
- `method Due` — `internal/droplets/lifecycle.go:22`
- `method Transition` — `internal/droplets/lifecycle.go:45`
- `method Event` — `internal/droplets/lifecycle.go:53`
- `interface LifecycleExecutor` — `internal/droplets/lifecycle_engine.go:12`
- `struct LifecycleEngine` — `internal/droplets/lifecycle_engine.go:15`
- `method Process` — `internal/droplets/lifecycle_engine.go:20`
- `struct Profile` — `internal/droplets/model.go:20`
- `struct Droplet` — `internal/droplets/model.go:32`
- `func TestRetiringWithoutExpiryIsDueE2E` — `internal/droplets/postgres_e2e_test.go:12`
- `struct Reconciler` — `internal/droplets/reconcile.go:13`
- `method VerifyCreate` — `internal/droplets/reconcile.go:18`
- `method AdoptUnknownCreate` — `internal/droplets/reconcile.go:36`
- `func BuildDeleteOperation` — `internal/droplets/reconcile.go:67`
- `func TestAdoptUnknownCreateUsesUniqueTag` — `internal/droplets/reconcile_test.go:11`
- `func TestAdoptUnknownCreateAmbiguousTagStaysUnknown` — `internal/droplets/reconcile_test.go:26`
- `func TestUnknownExistingCreateNeverIssuesSecondMutation` — `internal/droplets/safety_characterization_test.go:11`
- `func TestDeleteSuccessRequiresLaterConfirmation` — `internal/droplets/safety_characterization_test.go:24`
- `func TestDeleteErrorIsAmbiguousUnknown` — `internal/droplets/safety_characterization_test.go:44`

## Package `network` key exported symbols

- `struct ClientBundle` — `internal/network/client.go:9`
- `func NewIsolatedDirectClient` — `internal/network/client.go:16`
- `method Validate` — `internal/network/client.go:33`
- `method CloseIdleConnections` — `internal/network/client.go:40`
- `func TestClientsAreNotSharedAcrossAccounts` — `internal/network/client_test.go:5`
- `struct EgressGuard` — `internal/network/egress_guard.go:14`
- `method Observe` — `internal/network/egress_guard.go:20`
- `method Baseline` — `internal/network/egress_guard.go:46`
- `struct seqRT` — `internal/network/egress_guard_test.go:11`
- `method RoundTrip` — `internal/network/egress_guard_test.go:13`
- `func TestEgressGuardDetectsChange` — `internal/network/egress_guard_test.go:21`
- `struct AccountGate` — `internal/network/gate.go:7`
- `method AllowMutation` — `internal/network/gate.go:13`
- `func TestAccountGateRequiresHealthyProxy` — `internal/network/gate_test.go:5`
- `func ValidateRoute` — `internal/network/guard.go:10`
- `func TestProxyRequiredFailsClosed` — `internal/network/guard_test.go:5`
- `func TestProxyRequiredAllowsHealthyProxy` — `internal/network/guard_test.go:14`
- `struct HealthResult` — `internal/network/health.go:17`
- `struct ipPayload` — `internal/network/health.go:25`
- `func CheckProxy` — `internal/network/health.go:29`
- `struct HealthPolicy` — `internal/network/health_state.go:5`
- `struct HealthState` — `internal/network/health_state.go:11`
- `method Apply` — `internal/network/health_state.go:21`
- `func TestHealthStateUsesHysteresis` — `internal/network/health_state_test.go:8`
- `interface HealthStore` — `internal/network/health_store.go:11`
- `struct SQLHealthStore` — `internal/network/health_store.go:15`
- `method Save` — `internal/network/health_store.go:17`
- `func TestProxyHealthValidatesExitIP` — `internal/network/health_test.go:10`
- `func TestProxyHealthRejectsUnexpectedExitIP` — `internal/network/health_test.go:23`
- `func ResolveIPv4` — `internal/network/ipv4.go:13`
- `func DialContextIPv4` — `internal/network/ipv4.go:31`
- `func IPv4Endpoint` — `internal/network/ipv4.go:43`
- `struct ipv4ProxyDialer` — `internal/network/ipv4.go:51`
- `method Dial` — `internal/network/ipv4.go:53`
- `func TestResolveIPv4RejectsIPv6Literal` — `internal/network/ipv4_test.go:9`
- `func TestResolveIPv4AcceptsIPv4Literal` — `internal/network/ipv4_test.go:15`
- `struct AccountContext` — `internal/network/isolation.go:12`
- `func AccountNamespace` — `internal/network/isolation.go:20`
- `method Validate` — `internal/network/isolation.go:25`
- `func TestAccountContextsCannotCross` — `internal/network/isolation_test.go:5`
- `func TestAccountNamespaceStableAndDistinct` — `internal/network/isolation_test.go:12`
- `struct LeakObservation` — `internal/network/leakcheck.go:14`
- `func ValidateLeakObservation` — `internal/network/leakcheck.go:21`
- `func TestLeakObservationPassesExpectedProxy` — `internal/network/leakcheck_test.go:8`
- `func TestLeakObservationRejectsServerIP` — `internal/network/leakcheck_test.go:15`
- `func TestLeakObservationRejectsForwardingHeader` — `internal/network/leakcheck_test.go:22`
- `struct MetadataTransport` — `internal/network/metadata.go:17`
- `method RoundTrip` — `internal/network/metadata.go:19`
- `func TestMetadataTransportDoesNotLeakInternalIdentity` — `internal/network/metadata_test.go:10`
- `struct Proxy` — `internal/network/model.go:21`
- `struct Profile` — `internal/network/model.go:38`
- `struct Monitor` — `internal/network/monitor.go:11`
- `method Run` — `internal/network/monitor.go:20`
- `struct item` — `internal/network/monitor.go:43`
- `struct PrivacyTransport` — `internal/network/privacy.go:21`
- `method RoundTrip` — `internal/network/privacy.go:25`
- `func HasForbiddenOutboundHeader` — `internal/network/privacy.go:40`
- `method RoundTrip` — `internal/network/privacy_test.go:12`
- `func TestPrivacyTransportStripsLeakHeaders` — `internal/network/privacy_test.go:14`
- `struct TLSProfile` — `internal/network/profiles.go:9`
- `struct HTTPProfile` — `internal/network/profiles.go:15`
- `struct BrowserProfile` — `internal/network/profiles.go:22`
- `func NewTLSProfile` — `internal/network/profiles.go:30`
- `func NewHTTPProfile` — `internal/network/profiles.go:36`
- `func NewBrowserProfile` — `internal/network/profiles.go:45`
- `func TestSecurityProfilesHaveSeparateState` — `internal/network/profiles_test.go:5`
- `func TestGateBlocksDegradedAndDown` — `internal/network/proxy_failure_test.go:11`
- `func TestRouteRejectsMissingOrUnhealthyProxy` — `internal/network/proxy_failure_test.go:21`
- `func TestHealthRejectsServerIPLeak` — `internal/network/proxy_failure_test.go:36`
- `func TestHighLatencyDegradesThenDown` — `internal/network/proxy_failure_test.go:48`
- `func TestRecoveryRequiresConsecutiveSuccesses` — `internal/network/proxy_failure_test.go:63`
- `struct ProxyCredentials` — `internal/network/proxy_gateway.go:20`
- `struct Gateway` — `internal/network/proxy_gateway.go:22`
- `func NewProxyGateway` — `internal/network/proxy_gateway.go:30`
- `func NewProxyProbeGateway` — `internal/network/proxy_gateway.go:39`
- `struct result` — `internal/network/proxy_gateway.go:76`
- `method Validate` — `internal/network/proxy_gateway.go:96`
- `method CloseIdleConnections` — `internal/network/proxy_gateway.go:102`
- `method DirectDial` — `internal/network/proxy_gateway.go:107`
- `func TestHTTPGatewayIsAccountScoped` — `internal/network/proxy_gateway_test.go:5`

## Package `accounts` key exported symbols

- `struct Cell` — `internal/accounts/cell.go:10`
- `struct CellManager` — `internal/accounts/cell.go:14`
- `func NewCellManager` — `internal/accounts/cell.go:19`
- `method Register` — `internal/accounts/cell.go:21`
- `method Acquire` — `internal/accounts/cell.go:32`
- `func TestCellManagerKeepsAccountsSeparate` — `internal/accounts/cell_test.go:5`
- `struct Context` — `internal/accounts/context.go:10`
- `func NewContext` — `internal/accounts/context.go:19`
- `method Authorize` — `internal/accounts/context.go:30`
- `func TestAccountNamespacesAreDistinct` — `internal/accounts/context_test.go:5`
- `func SecureID` — `internal/accounts/identity.go:11`
- `struct RuntimeIdentity` — `internal/accounts/identity.go:19`
- `func NewRuntimeIdentity` — `internal/accounts/identity.go:26`
- `func TestRuntimeIdentitiesAreIndependent` — `internal/accounts/identity_test.go:5`
