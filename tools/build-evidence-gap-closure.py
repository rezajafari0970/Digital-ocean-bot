#!/usr/bin/env python3
import json,os,subprocess,re
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
# Record verified explanations for current knowledge gaps; do not invent missing tests/access.
closures=[
 {'gap':'residential/tests','status':'confirmed-gap','evidence':'grep across internal/**/*_test.go found no residential/residentialsync references','interpretation':'No direct Residential test evidence exists in current repository; keep as knowledge/product verification gap, do not mark covered.'},
 {'gap':'database/migration-origin/provider_catalog_cache.catalog','status':'explained','evidence':'table is pre-existing/alter-only in static migration parser output','interpretation':'Origin cannot be assigned to a CREATE migration from current parser evidence.'},
 {'gap':'database/migration-origin/provider_catalog_cache.refreshed_at','status':'explained','evidence':'table is pre-existing/alter-only in static migration parser output','interpretation':'Origin cannot be assigned to a CREATE migration from current parser evidence.'},
 {'gap':'database/migration-origin/schema_migrations.*','status':'explained','evidence':'internal/migrate/runner.go creates/alters schema_migrations at runtime before applying migration files','interpretation':'These columns are bootstrap-owned by migration runner, not a numbered migration.'},
 {'gap':'database/code-mentions/security_profile_columns','status':'confirmed-indirect','evidence':'columns exist from 000003_security_profiles.up.sql but no direct function-level lexical mention','interpretation':'Do not fabricate reader/writer ownership; access may be indirect/unused.'},
 {'gap':'database/code-mentions/proxies.expected_exit_ip','status':'confirmed-indirect','evidence':'column originates in 000004_proxy_health.up.sql but no direct function-level lexical mention','interpretation':'Keep as direct-evidence gap until source proves access.'}
]
# Curated history evidence from actual Git subjects for previously unguarded domains.
history={
 'sanaei_panel':['6514987','9c7bbbe','12f07e7','202f056','18c1464','ab6b19a'],
 'reality':['3a6ba7a','09f0488','26d4077','b2dfc38','c6433c3'],
 'admin_api':['c3b75d5','c3cdd87','9652470'],
 'frontend':['6191ea8','2de0245','ed4dccf','e1261a6','252dd4b','208583d'],
 'residential':[]
}
resolved={}
for area,shorts in history.items():
 rows=[]
 for s in shorts:
  p=subprocess.run(['git','show','-s','--format=%H\t%ad\t%s','--date=short',s],text=True,capture_output=True)
  if p.returncode==0:
   a=p.stdout.strip().split('\t',2);rows.append({'commit':a[0],'date':a[1],'subject':a[2]})
 resolved[area]=rows
out={'schema_version':1,'source_commit':head,'closures':closures,'historical_direct_evidence':resolved,'principle':'Explained gaps remain visible unless direct evidence closes them. No missing test, migration origin, or code ownership is fabricated.'}
json.dump(out,open('docs/EVIDENCE_GAP_CLOSURE.json','w'),ensure_ascii=False,indent=2);open('docs/EVIDENCE_GAP_CLOSURE.json','a').write('\n')
md=['# Evidence Gap Closure','',f'Commit `{head}`. This file distinguishes **closed/explained** gaps from **confirmed remaining** gaps. Missing evidence is never converted into coverage by assumption.','', '## Findings']
for x in closures:md += [f'- `{x["gap"]}` — **{x["status"]}**: {x["interpretation"]}']
md += ['','## Direct historical Git evidence']
for a,rows in resolved.items():md.append(f'- `{a}` — {len(rows)} directly selected historical commits')
open('docs/EVIDENCE_GAP_CLOSURE.md','w').write('\n'.join(md)+'\n');print('gap closure findings',len(closures),'history areas', {k:len(v) for k,v in resolved.items()})
