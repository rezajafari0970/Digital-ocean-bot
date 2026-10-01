#!/usr/bin/env python3
import os,re,json,glob
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
tests=sorted(glob.glob('internal/**/*_test.go',recursive=True))
rows=[]
for p in tests:
 txt=open(p,encoding='utf-8').read(); pkg=re.search(r'(?m)^package\s+(\w+)',txt); pkg=pkg.group(1) if pkg else ''
 names=re.findall(r'(?m)^func\s+(Test\w+)\s*\(',txt)
 rows.append({'file':p,'package':pkg,'tests':names,'test_count':len(names)})
features={
 'providers_common':['internal/providers/'], 'digitalocean':['internal/providers/digitalocean/'], 'vultr':['internal/providers/vultr/','internal/providers/vultrconsole/'],
 'accounts':['internal/accounts/','internal/adminapi/account'], 'network_proxy':['internal/network/'], 'compute_lifecycle':['internal/droplets/'],
 'sanaei':['internal/panels/sanaei/'], 'reality_policy':['internal/panels/policy/','internal/panels/reality'], 'panel_lifecycle':['internal/panels/create/','internal/panels/update/','internal/panels/desired/','internal/panels/readyworker/'],
 'output':['internal/adminapi/output'], 'adminapi':['internal/adminapi/'], 'auth':['internal/auth/'], 'jobs':['internal/jobs/']}
fmap={}
for feat,prefixes in features.items():
 fs=[r for r in rows if any(r['file'].startswith(x) for x in prefixes)]; fmap[feat]={'files':[r['file'] for r in fs],'test_functions':sum(r['test_count'] for r in fs),'command':'go test ./'+common_pkg(prefixes[0]) if False else None}
# explicit useful commands avoid pretending overlapping prefixes form one package
commands={
 'providers_common':'go test ./internal/providers/...','digitalocean':'go test ./internal/providers/digitalocean/...','vultr':'go test ./internal/providers/vultr/... ./internal/providers/vultrconsole/...','accounts':'go test ./internal/accounts/... ./internal/adminapi/...','network_proxy':'go test ./internal/network/...','compute_lifecycle':'go test ./internal/droplets/...','sanaei':'go test ./internal/panels/sanaei/...','reality_policy':'go test ./internal/panels/policy/... ./internal/panels/realityconfig/... ./internal/panels/realitycontract/... ./internal/panels/globalreality/...','panel_lifecycle':'go test ./internal/panels/create/... ./internal/panels/update/... ./internal/panels/desired/... ./internal/panels/readyworker/...','output':'go test ./internal/adminapi/... ./internal/panels/sanaei/...','adminapi':'go test ./internal/adminapi/...','auth':'go test ./internal/auth/...','jobs':'go test ./internal/jobs/...'}
for k,v in fmap.items(): v['command']=commands[k]
idx={'schema_version':1,'test_file_count':len(rows),'test_function_count':sum(r['test_count'] for r in rows),'test_files':rows,'features':fmap,'verification_tiers':{'focused':'Run mapped feature/package tests first.','broad':'go test ./internal/...','build':'go build ./cmd/api ./cmd/worker','runtime':'After deploy verify systemd API/worker health and exercise the real feature flow.'}}
json.dump(idx,open('docs/TEST_MAP.json','w',encoding='utf-8'),ensure_ascii=False,indent=2); open('docs/TEST_MAP.json','a').write('\n')
out=['# Test / Verification Map','',f'Indexed **{len(rows)} test files** containing **{idx["test_function_count"]} Test* functions**. Full machine-readable map: `docs/TEST_MAP.json`.','', 'Use the narrowest relevant tests first, then broader verification appropriate to blast radius. A passing unit test does not replace runtime smoke verification for deployed behavior.','']
for feat,v in fmap.items():
 out += [f'## `{feat}`',f'- Test files: **{len(v["files"])}**; Test functions: **{v["test_functions"]}**',f'- Focused command: `{v["command"]}`']
 if v['files']: out.append('- Files: '+', '.join(f'`{x}`' for x in v['files'][:18]))
 out.append('')
out += ['## Verification tiers','- Focused: mapped subsystem/package tests.','- Broad: `go test ./internal/...` when cross-cutting behavior changed.','- Build: `go build ./cmd/api ./cmd/worker` before production deployment of Go changes.','- Runtime: verify API/worker service health, migrations when relevant, then exercise the real endpoint/UI/provider flow.']
open('docs/TEST_NAVIGATION.md','w',encoding='utf-8').write('\n'.join(out)+'\n')
print('test-files',len(rows),'test-functions',idx['test_function_count'],'features',len(fmap))
