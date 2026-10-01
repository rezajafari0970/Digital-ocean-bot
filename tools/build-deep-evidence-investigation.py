#!/usr/bin/env python3
import json,subprocess,os
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
findings={
'database':{
 'unused_or_migration_only_columns':['account_security_profiles.profile_version','account_security_profiles.tls_policy','account_security_profiles.http_policy','account_security_profiles.browser_policy','account_security_profiles.browser_namespace','proxies.expected_exit_ip'],
 'evidence':'Repository-wide exact-name search found these names only in their migrations, with no cmd/internal/web/deploy references.',
 'knowledge_conclusion':'Their lack of function mentions is fully characterized: current source has no direct lexical application access. This is known absence, not unknown ownership.'},
'residential':{
 'direct_tests':[],
 'implementation_evidence':['cmd/worker/main.go:260-275','internal/adminapi/residential.go','internal/panels/residentialsync/service.go','migrations/000086_residential_proxies.up.sql','web/static/app.js:96-100'],
 'git_history_candidates':['3270e99ecf8c1d1de7eaab3a790ddf060a543105'],
 'knowledge_conclusion':'Residential implementation/API/UI/sync/schema are directly evidenced. No direct residential test exists. Git -S/-G finds historical presence, but no dedicated residential regression commit subject sufficient to claim an incident guardrail.'}}
out={'schema_version':1,'source_commit':head,'findings':findings,'principle':'Mastery of project knowledge can be complete about an evidence absence: known-no-direct-test is different from unknown. Product test coverage itself is not upgraded by documenting that absence.'}
json.dump(out,open('docs/DEEP_EVIDENCE_INVESTIGATION.json','w'),indent=2);open('docs/DEEP_EVIDENCE_INVESTIGATION.json','a').write('\n')
open('docs/DEEP_EVIDENCE_INVESTIGATION.md','w').write(f'''# Deep Evidence Investigation\n\nCommit `{head}`.\n\n## Database\nThe six previously indirect columns were searched repository-wide. They occur only in their migrations and have no direct application lexical references. Their ownership is therefore no longer *unknown*: current source provides evidence of **no direct access**.\n\n## Residential\nImplementation is directly evidenced across worker sync, Admin API CRUD/test endpoint, `residentialsync`, migration `000086`, and frontend actions. No direct Residential test exists in the current test suite. Git history contains Residential code presence, but no dedicated regression/incident subject strong enough to invent an incident guardrail.\n\nThis distinction matters: knowledge can be complete about a known absence without pretending product test/history evidence exists.\n''')
print('deep evidence investigation written',head[:12])
