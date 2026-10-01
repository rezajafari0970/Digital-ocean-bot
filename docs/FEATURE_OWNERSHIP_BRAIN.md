# Feature / Cross-Ownership Brain

Commit `950ade7ea0c050655dc59960e1c1032f93cd2dc2` — **18 feature-level ownership views** across physical package boundaries.

This fixes the package-vs-feature gap: a feature can own UI actions, admin API routes, provider/network functions, DB columns and tests even when those live in different packages. Matching is vocabulary-based navigation evidence, not an assertion of exclusive ownership.

| Feature | Req | Routes | Funcs | Columns | Tests | UI | Incidents |
|---|---:|---:|---:|---:|---:|---:|---:|
| `accounts` | 9 | 17 | 91 | 104 | 13 | 3 | 1 |
| `digitalocean` | 5 | 0 | 127 | 18 | 26 | 0 | 1 |
| `vultr` | 9 | 7 | 105 | 29 | 19 | 1 | 2 |
| `proxy_network` | 9 | 13 | 138 | 30 | 33 | 3 | 2 |
| `lifecycle` | 10 | 2 | 127 | 85 | 14 | 0 | 2 |
| `provisioning` | 7 | 8 | 195 | 66 | 48 | 2 | 1 |
| `sanaei_panel` | 13 | 6 | 304 | 119 | 78 | 2 | 0 |
| `reality` | 5 | 0 | 64 | 65 | 25 | 0 | 0 |
| `config_users` | 14 | 8 | 83 | 65 | 14 | 3 | 1 |
| `output` | 13 | 3 | 44 | 63 | 5 | 7 | 3 |
| `residential` | 5 | 5 | 7 | 6 | 0 | 4 | 0 |
| `database` | 5 | 0 | 16 | 19 | 3 | 0 | 1 |
| `admin_api` | 5 | 62 | 118 | 0 | 5 | 16 | 0 |
| `frontend` | 6 | 0 | 0 | 0 | 0 | 3 | 0 |
| `tests` | 8 | 3 | 375 | 0 | 251 | 2 | 0 |
| `deployment_runtime` | 12 | 6 | 157 | 105 | 33 | 0 | 1 |
| `architecture` | 15 | 1 | 212 | 27 | 42 | 1 | 2 |
| `continuity` | 7 | 0 | 0 | 0 | 0 | 0 | 1 |
