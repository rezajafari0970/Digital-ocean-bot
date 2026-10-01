# Database / Schema Navigation Index

Statically reconstructed from **102 up migrations**: **63 table names** observed. This is a source index, not proof of live DB migration level.

For exact production state, query the live migration table/schema before mutation. Full machine-readable index: `docs/SCHEMA_INDEX.json`.

## `account_browser_identities`
- Created/first observed: `000050_browser_identity_audit.up.sql`
- Known columns from static migration parsing: 32; alter operations: 13; indexes: 1
- Columns: `account_id`, `profile_namespace`, `platform`, `user_agent`, `timezone`, `language`, `screen`, `hardware_concurrency`, `device_memory`, `touch_support`, `canvas_hash`, `webgl_vendor`, `webgl_renderer`, `webgl_hash`, `audio_hash`, `client_rects_hash`, `fonts_hash`, `webrtc_candidates`, `webrtc_leak`, `raw`, `checked_at`, `runtime`, `audit_status`, `audit_error`, `runtime_version`, `runtime_engine`, `expected_country`, `expected_timezone`, `expected_locale`, `timezone_match`, `locale_match`, `geo_consistency`

## `account_capacity_events`
- Created/first observed: `000038_capacity_history.up.sql`
- Known columns from static migration parsing: 7; alter operations: 0; indexes: 1
- Columns: `id`, `account_id`, `old_limit`, `new_limit`, `delta`, `detected_at`, `snapshot_id`
- Code usage: `internal/adminapi/resources.go`, `internal/app/provider_refresh.go`

## `account_network_identities`
- Created/first observed: `000041_account_network_identity.up.sql`
- Known columns from static migration parsing: 15; alter operations: 7; indexes: 0
- Columns: `account_id`, `timezone`, `locale`, `exit_ip`, `subnet_key`, `asn`, `country`, `updated_at`, `sticky_session`, `preferred_country_code`, `preferred_country`, `rotation_started_at`, `fallback_active`, `last_health_at`, `last_health_ok`
- Code usage: `internal/adminapi/accounts_manage.go`, `internal/adminapi/dashboard.go`, `internal/adminapi/network_identity_auto.go`, `internal/adminapi/network_isolation.go`, `internal/adminapi/proxies_manage.go`, `internal/adminapi/proxies_write.go`, `internal/adminapi/proxy_health.go`, `internal/app/network_identity.go`, `internal/app/proxy_pool.go`, `internal/app/repository.go`, `internal/app/sticky_proxy.go`, `internal/geoctx/account.go`

## `account_proxy_pool`
- Created/first observed: `000099_account_proxy_pool.up.sql`
- Known columns from static migration parsing: 6; alter operations: 0; indexes: 1
- Columns: `account_id`, `proxy_id`, `priority`, `enabled`, `created_at`, `updated_at`
- Code usage: `cmd/vultr-browser-session/main.go`, `cmd/vultr-capacity-probe/main.go`, `internal/adminapi/accounts_manage.go`, `internal/adminapi/accounts_write.go`, `internal/adminapi/dashboard.go`, `internal/adminapi/proxy_pool.go`, `internal/adminapi/resources.go`, `internal/app/proxy_pool.go`

## `account_runtime_state`
- Created/first observed: `000002_operations.up.sql`
- Known columns from static migration parsing: 7; alter operations: 0; indexes: 0
- Columns: `account_id`, `circuit_state`, `consecutive_failures`, `retry_after`, `rate_remaining`, `rate_reset_at`, `updated_at`
- Code usage: `internal/adminapi/dashboard.go`, `internal/adminapi/runtime_api.go`, `internal/resilience/store.go`

## `account_security_profiles`
- Created/first observed: `000003_security_profiles.up.sql`
- Known columns from static migration parsing: 8; alter operations: 0; indexes: 1
- Columns: `account_id`, `profile_version`, `tls_policy`, `http_policy`, `browser_policy`, `browser_namespace`, `created_at`, `updated_at`

## `accounts`
- Created/first observed: `000001_core.up.sql`
- Known columns from static migration parsing: 55; alter operations: 47; indexes: 3
- Columns: `id`, `provider`, `name`, `external_id`, `email`, `secret_ref`, `enabled`, `created_at`, `updated_at`, `preferred_region`, `auto_interval_seconds`, `auto_batch_size`, `auto_max_concurrent`, `preferred_regions`, `preferred_sizes`, `preferred_image`, `server_lifetime_seconds`, `runtime_status`, `runtime_status_detail`, `runtime_status_at`, `desired_server_count`, `fallback_any_region`, `preferred_images`, `server_lifetime_min_seconds`, `server_lifetime_max_seconds`, `build_spacing_minutes`, `build_spacing_max_minutes`, `next_build_at`, `login_email`, `login_password_secret_ref`, `password_rotation_status`, `password_rotated_at`, `password_rotation_detail`, `assigned_browser`, `browser_assigned_at`, `browser_login_status`, `browser_login_detail`, `browser_login_checked_at`, `browser_challenge_type`, `browser_challenge_at`
- Code usage: `cmd/vultr-browser-session/main.go`, `cmd/vultr-capacity-probe/main.go`, `cmd/worker/main.go`, `internal/accounts/cell.go`, `internal/accounts/cell_test.go`, `internal/accounts/context.go`, `internal/accounts/context_test.go`, `internal/accounts/identity.go`, `internal/accounts/identity_test.go`, `internal/adminapi/account_automation.go`, `internal/adminapi/account_identity.go`, `internal/adminapi/account_options.go`, `internal/adminapi/account_preflight.go`, `internal/adminapi/accounts_manage.go`, `internal/adminapi/accounts_write.go`

## `admin_sessions`
- Created/first observed: `000017_admin_auth.up.sql`
- Known columns from static migration parsing: 6; alter operations: 0; indexes: 2
- Columns: `id`, `user_id`, `token_hash`, `expires_at`, `created_at`, `last_seen_at`
- Code usage: `internal/auth/store.go`

## `admin_users`
- Created/first observed: `000017_admin_auth.up.sql`
- Known columns from static migration parsing: 7; alter operations: 0; indexes: 0
- Columns: `id`, `username`, `password_hash`, `role`, `enabled`, `created_at`, `updated_at`
- Code usage: `cmd/admin-bootstrap/main.go`, `internal/auth/store.go`

## `audit_events`
- Created/first observed: `000016_observability.up.sql`
- Known columns from static migration parsing: 10; alter operations: 0; indexes: 2
- Columns: `id`, `account_id`, `actor`, `action`, `resource_type`, `resource_id`, `result`, `message`, `metadata`, `created_at`
- Code usage: `internal/adminapi/status.go`, `internal/audit/audit.go`

## `browser_audit_queue`
- Created/first observed: `000053_browser_audit_queue.up.sql`
- Known columns from static migration parsing: 6; alter operations: 0; indexes: 0
- Columns: `account_id`, `requested_at`, `not_before`, `reason`, `attempts`, `last_error`

## `deployment_events`
- Created/first observed: `000013_deployments.up.sql`
- Known columns from static migration parsing: 6; alter operations: 0; indexes: 1
- Columns: `id`, `deployment_id`, `step`, `state`, `message`, `created_at`
- Code usage: `internal/adminapi/build_activity.go`, `internal/app/lifecycle.go`, `internal/app/postinstall_rearm.go`, `internal/app/postinstall_reconcile.go`, `internal/workflow/store.go`

## `deployment_installer_selections`
- Created/first observed: `000068_installer_selection.up.sql`
- Known columns from static migration parsing: 6; alter operations: 3; indexes: 1
- Columns: `deployment_id`, `installer_name`, `installer_version`, `source`, `selected_at`, `generation`
- Code usage: `internal/adminapi/installer_rearm.go`, `internal/adminapi/installer_selection.go`, `internal/app/installer_activation.go`, `internal/app/recovery.go`, `internal/worker/failures.go`, `internal/worker/recovery.go`

## `deployment_profiles`
- Created/first observed: `000014_deployment_profiles.up.sql`
- Known columns from static migration parsing: 8; alter operations: 0; indexes: 1
- Columns: `id`, `account_id`, `name`, `version`, `config`, `enabled`, `created_at`, `updated_at`
- Code usage: `internal/adminapi/account_automation.go`, `internal/droplets/postgres_e2e_test.go`, `internal/scheduler/engine.go`, `internal/workflow/postgres_e2e_test.go`, `internal/workflow/profile_store.go`

## `deployment_step_attempts`
- Created/first observed: `000058_deployment_step_attempts.up.sql`
- Known columns from static migration parsing: 9; alter operations: 5; indexes: 2
- Columns: `deployment_id`, `step`, `attempts`, `last_started_at`, `last_finished_at`, `last_error`, `last_error_class`, `next_retry_at`, `generation`
- Code usage: `internal/workflow/postgres_e2e_test.go`, `internal/workflow/store.go`

## `deployments`
- Created/first observed: `000013_deployments.up.sql`
- Known columns from static migration parsing: 16; alter operations: 6; indexes: 4
- Columns: `id`, `account_id`, `profile_id`, `droplet_id`, `provider_id`, `state`, `current_step`, `attempt`, `last_error`, `created_at`, `updated_at`, `profile_snapshot`, `lock_version`, `installer_generation`, `host`, `postinstall_generation`
- Code usage: `internal/adminapi/build_activity.go`, `internal/adminapi/dashboard.go`, `internal/adminapi/installer_rearm.go`, `internal/adminapi/installer_selection.go`, `internal/adminapi/output.go`, `internal/adminapi/output_snapshot.go`, `internal/adminapi/provision_activity.go`, `internal/adminapi/provision_errors.go`, `internal/adminapi/server.go`, `internal/adminapi/status.go`, `internal/app/consistency.go`, `internal/app/deploy.go`, `internal/app/installer_activation.go`, `internal/app/lifecycle.go`, `internal/app/orphan_reconcile.go`

## `droplets`
- Created/first observed: `000007_droplet_lifecycle.up.sql`
- Known columns from static migration parsing: 11; alter operations: 4; indexes: 6
- Columns: `id`, `account_id`, `provider_resource_id`, `state`, `profile`, `created_at`, `updated_at`, `ready_at`, `expires_at`, `profile_id`, `replacement_deployment_id`
- Code usage: `cmd/lifecycle-once/main.go`, `cmd/worker/main.go`, `internal/adminapi/accounts_manage.go`, `internal/adminapi/dashboard.go`, `internal/adminapi/output.go`, `internal/adminapi/output_snapshot.go`, `internal/adminapi/provision_errors.go`, `internal/adminapi/resources.go`, `internal/app/consistency.go`, `internal/app/deployment_ready.go`, `internal/app/lifecycle.go`, `internal/app/orphan_reconcile.go`, `internal/app/profile.go`, `internal/app/workflow.go`, `internal/capacity/service.go`

## `global_config_policies`
- Created/first observed: `000085_global_config_policy.up.sql`
- Known columns from static migration parsing: 13; alter operations: 0; indexes: 0
- Columns: `policy_key`, `enabled`, `ports`, `target_users_per_inbound`, `user_quota_bytes`, `user_lifetime_seconds`, `device_limit`, `users_per_second`, `sni_selection_mode`, `manual_snis`, `revision`, `created_at`, `updated_at`
- Code usage: `internal/adminapi/capacity_cleanup.go`, `internal/adminapi/configs_global.go`, `internal/panels/globalreality/service.go`, `internal/panels/panelbootstrap/service.go`, `internal/panels/usercapacity/legacy_fill.go`, `internal/panels/usercapacity/service.go`

## `inbound_export_metadata`
- Created/first observed: `000087_inbound_export_metadata.up.sql`
- Known columns from static migration parsing: 4; alter operations: 0; indexes: 0
- Columns: `panel_id`, `remote_id`, `public_key`, `updated_at`
- Code usage: `internal/adminapi/output.go`, `internal/panels/desired/service.go`

## `inbound_structural_snapshots`
- Created/first observed: `000091_inbound_structural_snapshots.up.sql`
- Known columns from static migration parsing: 5; alter operations: 0; indexes: 1
- Columns: `panel_id`, `remote_id`, `port`, `payload`, `updated_at`
- Code usage: `internal/adminapi/capacity_cleanup.go`, `internal/adminapi/output.go`

## `install_scripts`
- Created/first observed: `000062_install_script_registry.up.sql`
- Known columns from static migration parsing: 12; alter operations: 0; indexes: 0
- Columns: `id`, `name`, `version`, `category`, `precheck`, `execute`, `verify`, `timeout_seconds`, `max_attempts`, `sha256`, `active`, `created_at`
- Code usage: `internal/adminapi/install_scripts.go`, `internal/adminapi/installers.go`, `internal/provisioning/installer.go`, `internal/provisioning/script_registry.go`

## `installer_runs`
- Created/first observed: `000067_installer_orchestration.up.sql`
- Known columns from static migration parsing: 11; alter operations: 3; indexes: 2
- Columns: `id`, `deployment_id`, `provision_run_id`, `installer_id`, `manifest_snapshot`, `manifest_sha256`, `state`, `last_error`, `created_at`, `updated_at`, `generation`
- Code usage: `internal/adminapi/provision_activity.go`, `internal/app/installer_activation.go`, `internal/app/postinstall_capabilities.go`, `internal/app/recovery.go`, `internal/provisioning/installer_orchestrator.go`, `internal/worker/recovery.go`

## `installers`
- Created/first observed: `000067_installer_orchestration.up.sql`
- Known columns from static migration parsing: 7; alter operations: 0; indexes: 0
- Columns: `id`, `name`, `version`, `manifest`, `sha256`, `active`, `created_at`
- Code usage: `internal/adminapi/installer_rearm.go`, `internal/adminapi/installer_selection.go`, `internal/adminapi/installers.go`, `internal/adminapi/provision_activity.go`, `internal/adminapi/server.go`, `internal/panels/sanaei/installer_adapter.go`, `internal/provisioning/installer.go`, `internal/scheduler/engine.go`

## `lifecycle_events`
- Created/first observed: `000007_droplet_lifecycle.up.sql`
- Known columns from static migration parsing: 5; alter operations: 0; indexes: 0
- Columns: `id`, `account_id`, `resource_id`, `state`, `created_at`
- Code usage: `internal/app/deployment_ready.go`, `internal/app/orphan_reconcile.go`, `internal/droplets/lifecycle.go`

## `network_profiles`
- Created/first observed: `000001_core.up.sql`
- Known columns from static migration parsing: 6; alter operations: 0; indexes: 0
- Columns: `id`, `account_id`, `mode`, `proxy_id`, `created_at`, `updated_at`
- Code usage: `cmd/worker/main.go`, `internal/adminapi/account_preflight.go`, `internal/adminapi/accounts_manage.go`, `internal/adminapi/accounts_write.go`, `internal/adminapi/dashboard.go`, `internal/adminapi/discovery.go`, `internal/adminapi/network_identity_auto.go`, `internal/adminapi/network_isolation.go`, `internal/adminapi/proxies_manage.go`, `internal/adminapi/proxies_write.go`, `internal/adminapi/resources.go`, `internal/app/network_identity.go`, `internal/app/proxy_pool.go`, `internal/app/repository.go`, `internal/app/sticky_proxy.go`

## `operations`
- Created/first observed: `000002_operations.up.sql`
- Known columns from static migration parsing: 13; alter operations: 1; indexes: 2
- Columns: `id`, `account_id`, `kind`, `idempotency_key`, `state`, `provider_action_id`, `resource_id`, `attempt`, `error_code`, `error_message`, `created_at`, `updated_at`, `lock_version`
- Code usage: `cmd/worker/main.go`, `internal/adminapi/dashboard.go`, `internal/app/container.go`, `internal/app/lifecycle.go`, `internal/app/recovery.go`, `internal/app/workflow.go`, `internal/capacity/service.go`, `internal/droplets/executor.go`, `internal/droplets/executor_test.go`, `internal/droplets/reconcile.go`, `internal/droplets/reconcile_test.go`, `internal/droplets/safety_characterization_test.go`, `internal/jobs/store.go`, `internal/panels/sanaei/mutation.go`, `internal/scheduler/engine.go`

## `output_config_snapshots`
- Created/first observed: `000090_output_snapshots.up.sql`
- Known columns from static migration parsing: 5; alter operations: 1; indexes: 2
- Columns: `panel_id`, `uri`, `first_seen_at`, `last_seen_at`, `visible_until`
- Code usage: `internal/adminapi/output.go`, `internal/adminapi/output_snapshot.go`

## `output_share_tokens`
- Created/first observed: `000090_output_snapshots.up.sql`
- Known columns from static migration parsing: 3; alter operations: 0; indexes: 0
- Columns: `token`, `created_at`, `revoked_at`
- Code usage: `internal/adminapi/output_snapshot.go`

## `panel_ad_proxies`
- Created/first observed: `000083_panel_ad_proxies.up.sql`
- Known columns from static migration parsing: 8; alter operations: 0; indexes: 1
- Columns: `id`, `panel_id`, `proxy_id`, `outbound_tag`, `priority`, `enabled`, `created_at`, `updated_at`

## `panel_capability_snapshots`
- Created/first observed: `000074_panel_driver_foundation.up.sql`
- Known columns from static migration parsing: 8; alter operations: 0; indexes: 1
- Columns: `id`, `panel_id`, `driver`, `version`, `capabilities`, `evidence`, `observed_at`, `created_at`
- Code usage: `internal/panels/store.go`

## `panel_inbound_inventory`
- Created/first observed: `000075_panel_inbound_inventory.up.sql`
- Known columns from static migration parsing: 18; alter operations: 0; indexes: 3
- Columns: `panel_id`, `remote_id`, `remark`, `protocol`, `port`, `listen`, `enabled`, `transport`, `security`, `client_count`, `upload_bytes`, `download_bytes`, `total_bytes`, `raw_hash`, `present`, `first_seen_at`, `last_seen_at`, `missing_since`
- Code usage: `internal/panels/globalreality/service.go`, `internal/panels/inventory/store.go`

## `panel_inbound_policies`
- Created/first observed: `000076_panel_inbound_policies.up.sql`
- Known columns from static migration parsing: 25; alter operations: 1; indexes: 1
- Columns: `id`, `panel_id`, `policy_key`, `revision`, `enabled`, `desired_count`, `protocol`, `transport`, `security`, `listen`, `preferred_ports`, `dynamic_port_start`, `dynamic_port_end`, `reserved_ports`, `clients_per_inbound`, `allow_delete`, `created_at`, `updated_at`, `user_quota_bytes`, `user_lifetime_seconds`, `device_limit`, `bulk_user_count`, `users_per_second`, `sni_selection_mode`, `manual_snis`
- Code usage: `internal/adminapi/configs.go`, `internal/panels/policy/store.go`

## `panel_instances`
- Created/first observed: `000074_panel_driver_foundation.up.sql`
- Known columns from static migration parsing: 11; alter operations: 0; indexes: 0
- Columns: `id`, `account_id`, `droplet_id`, `driver`, `base_url`, `auth_secret_ref`, `version`, `enabled`, `last_seen_at`, `created_at`, `updated_at`
- Code usage: `cmd/worker/main.go`, `internal/adminapi/config_capacity.go`, `internal/adminapi/configs.go`, `internal/adminapi/output.go`, `internal/adminapi/output_snapshot.go`, `internal/panels/desired/service.go`, `internal/panels/globalreality/service.go`, `internal/panels/panelbootstrap/service.go`, `internal/panels/readyworker/sql_source.go`, `internal/panels/residentialsync/service.go`, `internal/panels/sanaei/runtime.go`, `internal/panels/store.go`, `internal/panels/usercapacity/service.go`

## `panel_inventory_syncs`
- Created/first observed: `000075_panel_inbound_inventory.up.sql`
- Known columns from static migration parsing: 9; alter operations: 0; indexes: 0
- Columns: `id`, `panel_id`, `state`, `observed_count`, `changed_count`, `missing_count`, `started_at`, `finished_at`, `error_code`
- Code usage: `internal/panels/inventory/store.go`

## `panel_settings`
- Created/first observed: `000022_panel_settings.up.sql`
- Known columns from static migration parsing: 5; alter operations: 0; indexes: 0
- Columns: `id`, `listen_host`, `listen_port`, `web_path`, `updated_at`
- Code usage: `internal/adminapi/panel_settings.go`, `internal/app/panel.go`

## `provider_capacity_observations`
- Created/first observed: `000100_provider_capacity_observations.up.sql`
- Known columns from static migration parsing: 5; alter operations: 0; indexes: 1
- Columns: `account_id`, `compute_limit`, `source`, `observed_at`, `updated_at`
- Code usage: `cmd/vultr-capacity-probe/main.go`, `internal/adminapi/accounts_manage.go`, `internal/app/provider_refresh.go`

## `provider_catalog_cache`
- Created/first observed: `000027_provider_catalog_cache.up.sql`
- Known columns from static migration parsing: 1; alter operations: 0; indexes: 0
- Columns: `account_id`
- Code usage: `internal/app/catalog_sync.go`

## `provider_snapshots`
- Created/first observed: `000006_provider_registry.up.sql`
- Known columns from static migration parsing: 7; alter operations: 1; indexes: 1
- Columns: `id`, `account_id`, `provider`, `version`, `data`, `created_at`, `canonical`
- Code usage: `internal/adminapi/account_options.go`, `internal/adminapi/accounts_manage.go`, `internal/adminapi/dashboard.go`, `internal/adminapi/discovery.go`, `internal/adminapi/resources.go`, `internal/app/catalog_sync.go`, `internal/app/deploy.go`, `internal/app/orphan_reconcile.go`, `internal/app/provider_refresh.go`, `internal/app/recovery.go`, `internal/capacity/service.go`

## `provision_events`
- Created/first observed: `000009_provisioning.up.sql`
- Known columns from static migration parsing: 19; alter operations: 13; indexes: 3
- Columns: `id`, `run_id`, `step`, `state`, `error_code`, `created_at`, `substep`, `attempt`, `error_class`, `error_message`, `error_fingerprint`, `exit_code`, `signal`, `stdout_tail`, `stderr_tail`, `duration_ms`, `retryable`, `next_retry_at`, `metadata`
- Code usage: `internal/adminapi/provision_activity.go`, `internal/adminapi/provision_errors.go`, `internal/provisioning/events.go`

## `provision_runs`
- Created/first observed: `000009_provisioning.up.sql`
- Known columns from static migration parsing: 11; alter operations: 3; indexes: 1
- Columns: `id`, `account_id`, `droplet_id`, `state`, `current_step`, `attempt`, `created_at`, `updated_at`, `last_error`, `next_retry_at`, `ssh_host_key_sha256`
- Code usage: `internal/adminapi/provision_activity.go`, `internal/adminapi/provision_errors.go`, `internal/app/installer_activation.go`, `internal/app/recovery.go`, `internal/provisioning/hostkey.go`, `internal/provisioning/store.go`

## `provision_step_attempts`
- Created/first observed: `000059_provision_step_attempts.up.sql`
- Known columns from static migration parsing: 8; alter operations: 0; indexes: 1
- Columns: `run_id`, `step`, `attempts`, `last_started_at`, `last_finished_at`, `last_error`, `next_retry_at`, `terminal`
- Code usage: `internal/adminapi/provision_activity.go`, `internal/app/recovery.go`, `internal/provisioning/step_attempts.go`

## `proxies`
- Created/first observed: `000001_core.up.sql`
- Known columns from static migration parsing: 21; alter operations: 8; indexes: 1
- Columns: `id`, `name`, `type`, `host`, `port`, `username`, `secret_ref`, `status`, `exit_ip`, `country`, `asn`, `failure_count`, `last_checked_at`, `last_success_at`, `created_at`, `updated_at`, `consecutive_successes`, `latency_ms`, `health_error`, `expected_exit_ip`, `adapter`
- Code usage: `cmd/vultr-browser-session/main.go`, `cmd/vultr-capacity-probe/main.go`, `cmd/worker/main.go`, `internal/adminapi/account_preflight.go`, `internal/adminapi/account_preview.go`, `internal/adminapi/accounts_manage.go`, `internal/adminapi/accounts_write.go`, `internal/adminapi/dashboard.go`, `internal/adminapi/network_identity_auto.go`, `internal/adminapi/network_isolation.go`, `internal/adminapi/proxies_manage.go`, `internal/adminapi/proxies_write.go`, `internal/adminapi/proxy_details.go`, `internal/adminapi/proxy_health.go`, `internal/adminapi/residential.go`

## `reality_credentials`
- Created/first observed: `000078_reality_credentials.up.sql`
- Known columns from static migration parsing: 9; alter operations: 0; indexes: 1
- Columns: `id`, `panel_id`, `managed_key`, `uuid_secret_ref`, `private_key_secret_ref`, `public_key`, `short_id`, `created_at`, `updated_at`
- Code usage: `internal/panels/desired/service.go`, `internal/reality/credentials/registry.go`

## `reality_probe_nodes`
- Created/first observed: `000080_reality_probe_nodes.up.sql`
- Known columns from static migration parsing: 18; alter operations: 2; indexes: 1
- Columns: `id`, `name`, `host`, `port`, `ssh_user`, `account_id`, `ssh_key_secret_ref`, `enabled`, `status`, `consecutive_successes`, `consecutive_failures`, `last_latency_ms`, `last_exit_ip`, `last_checked_at`, `last_success_at`, `created_at`, `updated_at`, `ssh_host_key_sha256`
- Code usage: `internal/panels/proberegistry/hostkey.go`, `internal/panels/proberegistry/store.go`

## `reality_target_observations`
- Created/first observed: `000077_reality_target_observations.up.sql`
- Known columns from static migration parsing: 18; alter operations: 0; indexes: 2
- Columns: `id`, `panel_id`, `target`, `server_name`, `port`, `reachable`, `tls_version`, `cert_valid`, `http2`, `samples`, `successes`, `median_latency_ms`, `success_ratio`, `eligible`, `score`, `reason`, `observed_at`, `created_at`
- Code usage: `internal/reality/store.go`

## `reality_target_selections`
- Created/first observed: `000077_reality_target_observations.up.sql`
- Known columns from static migration parsing: 15; alter operations: 1; indexes: 0
- Columns: `panel_id`, `target`, `server_name`, `port`, `score`, `selected_at`, `updated_at`, `server_names`, `tls_version`, `alpn`, `curve_id`, `cert_valid`, `cert_chain_valid`, `latency_ms`, `scanned_at`
- Code usage: `internal/panels/desired/service.go`, `internal/panels/globalreality/service.go`, `internal/reality/store.go`

## `residential_proxies`
- Created/first observed: `000086_residential_proxies.up.sql`
- Known columns from static migration parsing: 6; alter operations: 0; indexes: 1
- Columns: `proxy_id`, `outbound_tag`, `priority`, `enabled`, `created_at`, `updated_at`
- Code usage: `internal/adminapi/residential.go`, `internal/panels/residentialsync/service.go`

## `resources`
- Created/first observed: `000006_provider_registry.up.sql`
- Known columns from static migration parsing: 10; alter operations: 0; indexes: 1
- Columns: `id`, `account_id`, `provider`, `provider_resource_id`, `type`, `state`, `managed`, `metadata`, `created_at`, `updated_at`
- Code usage: `internal/adminapi/accounts_manage.go`, `internal/adminapi/dashboard.go`, `internal/adminapi/resources_api.go`, `internal/adminapi/server.go`, `internal/app/consistency.go`, `internal/app/lifecycle.go`, `internal/app/orphan_reconcile.go`, `internal/app/recovery.go`, `internal/providers/types.go`, `internal/resources/errors.go`, `internal/resources/model.go`, `internal/resources/postgres.go`, `internal/resources/reconcile.go`, `internal/resources/reconcile_test.go`, `internal/resources/registry.go`

## `schedules`
- Created/first observed: `000019_schedules.up.sql`
- Known columns from static migration parsing: 12; alter operations: 1; indexes: 2
- Columns: `id`, `account_id`, `profile_id`, `enabled`, `interval_seconds`, `batch_size`, `max_concurrent`, `next_run_at`, `last_run_at`, `created_at`, `updated_at`, `lease_until`
- Code usage: `internal/adminapi/account_automation.go`, `internal/app/lifecycle.go`, `internal/scheduler/lease.go`, `internal/scheduler/store.go`

## `secrets`
- Created/first observed: `000005_secrets.up.sql`
- Known columns from static migration parsing: 11; alter operations: 12; indexes: 4
- Columns: `id`, `account_id`, `kind`, `ciphertext`, `nonce`, `key_version`, `created_at`, `updated_at`, `proxy_id`, `owner_key`, `probe_id`
- Code usage: `cmd/ready-panel-worker/main.go`, `cmd/vultr-browser-session/main.go`, `cmd/vultr-capacity-probe/main.go`, `cmd/worker/main.go`, `internal/accounts/context.go`, `internal/adminapi/account_preview.go`, `internal/adminapi/accounts_manage.go`, `internal/adminapi/accounts_write.go`, `internal/adminapi/capacity_cleanup.go`, `internal/adminapi/network_identity_auto.go`, `internal/adminapi/output.go`, `internal/adminapi/proxies_manage.go`, `internal/adminapi/proxies_write.go`, `internal/adminapi/proxy_details.go`, `internal/adminapi/proxy_health.go`

## `server_readiness_issues`
- Created/first observed: `000065_readiness_decisions.up.sql`
- Known columns from static migration parsing: 10; alter operations: 0; indexes: 2
- Columns: `id`, `readiness_id`, `run_id`, `code`, `severity`, `action`, `remediation`, `state`, `detail`, `created_at`
- Code usage: `internal/adminapi/provision_activity.go`, `internal/provisioning/readiness.go`

## `server_readiness_snapshots`
- Created/first observed: `000064_server_readiness.up.sql`
- Known columns from static migration parsing: 20; alter operations: 0; indexes: 2
- Columns: `id`, `run_id`, `account_id`, `droplet_id`, `status`, `os_id`, `os_version`, `architecture`, `cpu_count`, `memory_mb`, `disk_free_mb`, `is_root`, `package_manager`, `package_health`, `dns_ok`, `outbound_https_ok`, `time_sync`, `reboot_required`, `checks`, `created_at`
- Code usage: `internal/adminapi/provision_activity.go`, `internal/provisioning/readiness.go`

## `traffic_baselines`
- Created/first observed: `000012_traffic_guard.up.sql`
- Known columns from static migration parsing: 6; alter operations: 0; indexes: 0
- Columns: `client_id`, `ewma_bps`, `variance`, `samples`, `consecutive_anomalies`, `updated_at`

## `traffic_events`
- Created/first observed: `000012_traffic_guard.up.sql`
- Known columns from static migration parsing: 9; alter operations: 0; indexes: 2
- Columns: `id`, `client_id`, `suspicious`, `confirmed`, `rate_bps`, `threshold_bps`, `reason`, `action`, `created_at`

## `traffic_policies`
- Created/first observed: `000018_traffic_policies.up.sql`
- Known columns from static migration parsing: 10; alter operations: 0; indexes: 0
- Columns: `id`, `account_id`, `min_samples`, `alpha`, `sigma_multiplier`, `min_bps`, `hard_bps`, `confirmations`, `action`, `updated_at`

## `user_capacity_snapshots`
- Created/first observed: `000088_user_capacity_snapshots.up.sql`
- Known columns from static migration parsing: 12; alter operations: 0; indexes: 1
- Columns: `panel_id`, `inbound_id`, `port`, `target_users`, `active_users`, `expired_users`, `quota_exhausted_users`, `deficit`, `created_last_cycle`, `deleted_last_cycle`, `last_error`, `observed_at`
- Code usage: `internal/adminapi/capacity_cleanup.go`, `internal/adminapi/config_capacity.go`, `internal/panels/usercapacity/service.go`

## `worker_heartbeats`
- Created/first observed: `000016_observability.up.sql`
- Known columns from static migration parsing: 4; alter operations: 0; indexes: 0
- Columns: `worker_id`, `kind`, `last_seen_at`, `metadata`
- Code usage: `internal/adminapi/status.go`, `internal/worker/heartbeat.go`

## `worker_item_failures`
- Created/first observed: `000060_worker_failure_observability.up.sql`
- Known columns from static migration parsing: 8; alter operations: 0; indexes: 1
- Columns: `kind`, `item_id`, `account_id`, `failures`, `last_error`, `first_failed_at`, `last_failed_at`, `next_retry_at`
- Code usage: `internal/adminapi/installer_rearm.go`, `internal/worker/failures.go`

## `xui_clients`
- Created/first observed: `000011_xui_clients.up.sql`
- Known columns from static migration parsing: 8; alter operations: 0; indexes: 1
- Columns: `id`, `account_id`, `droplet_id`, `inbound_id`, `email`, `enabled`, `created_at`, `updated_at`
- Code usage: `internal/panels/sanaei/store.go`

## `xui_database_deployments`
- Created/first observed: `000010_xui_templates.up.sql`
- Known columns from static migration parsing: 8; alter operations: 2; indexes: 3
- Columns: `id`, `account_id`, `droplet_id`, `template_id`, `state`, `created_at`, `updated_at`, `generation`
- Code usage: `internal/app/postinstall_rearm.go`, `internal/app/postinstall_reconcile.go`, `internal/workflow/runtime.go`

## `xui_database_templates`
- Created/first observed: `000010_xui_templates.up.sql`
- Known columns from static migration parsing: 8; alter operations: 0; indexes: 0
- Columns: `id`, `name`, `version`, `storage_path`, `sha256`, `size_bytes`, `active`, `created_at`
- Code usage: `internal/adminapi/templates.go`, `internal/app/deploy.go`, `internal/app/profile.go`

## `xui_panel_deployments`
- Created/first observed: `000071_xui_panel_deployments.up.sql`
- Known columns from static migration parsing: 11; alter operations: 3; indexes: 1
- Columns: `id`, `account_id`, `droplet_id`, `username`, `password_secret_ref`, `port`, `web_path`, `state`, `created_at`, `updated_at`, `generation`
- Code usage: `internal/app/postinstall_reconcile.go`, `internal/panels/desired/service.go`, `internal/panels/panelbootstrap/service.go`, `internal/panels/readyworker/sql_source.go`, `internal/panels/residentialsync/service.go`, `internal/panels/sanaei/panel_configurer.go`, `internal/panels/sanaei/runtime.go`, `internal/panels/store.go`

## `xui_traffic_samples`
- Created/first observed: `000011_xui_clients.up.sql`
- Known columns from static migration parsing: 5; alter operations: 0; indexes: 1
- Columns: `id`, `client_id`, `up_bytes`, `down_bytes`, `sampled_at`
