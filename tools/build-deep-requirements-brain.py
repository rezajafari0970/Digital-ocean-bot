#!/usr/bin/env python3
import json,os,subprocess
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT)
head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
areas={
'architecture':['provider-neutral core','provider-specific behavior remains in drivers','modular independently testable subsystems','source/runtime are authoritative over stale prose'],
'accounts':['account CRUD and provider identity','independent account isolation cell','explicit account/provider states','optional browser credentials do not replace API token flow','automation settings remain account-scoped'],
'digitalocean':['preserve mature DigitalOcean behavior','provider catalog drives region/size/image availability','classified provider errors','capacity/history/snapshots remain observable','catalog synchronization does not regress active lifecycle'],
'vultr':['provider-specific API semantics','accurate capacity is first-class','interactive browser capacity observation isolated from generic provider core','manual challenge path remains human-interactive','authorized browser state may be reused after manual challenge'],
'proxy_network':['HTTP/SOCKS proxy model','per-account proxy binding','sticky identity while healthy','country affinity and controlled fallback','required proxy failure is fail-closed','network identity/geo context remain account-scoped','health/recovery must not silently leak direct egress'],
'lifecycle':['desired server count is reconciled state','build spacing and concurrency are controlled','one coherent lifecycle execution owner','creation/readiness/install/panel stages are observable','expiry/deletion/replacement are reconciled','recovery and retries remain idempotent where possible'],
'provisioning':['provision run/steps are observable','SSH readiness/host-key behavior is explicit','installer registry and selection are modular','retry/backoff and error classification are visible','rearm/recovery does not duplicate ownership','post-install completion is separate from provider create success'],
'sanaei_panel':['panel abstraction remains modular','Sanaei API is live read/write plane where implemented','authentication/session behavior is resilient','inbound/client mutation and inventory are API-driven','desired/create/update/planner/ready-worker responsibilities remain separated','panel bootstrap and user capacity are observable'],
'reality':['Reality credentials are generated/managed explicitly','target discovery/observations/scoring/selection are dedicated concerns','SNI selection follows policy/target finder rather than arbitrary fixed target','probe/health verification is explicit','Reality contract/global policy remain separate from provider lifecycle'],
'config_users':['global config and port policies are configurable','inbound allocation follows policy','bulk user creation scales without changing semantics','quota/lifetime/device limit remain independent controls','slot/capacity handling reflects live panel state','client deletion frees capacity'],
'output':['Output uses Sanaei/API state rather than SSH scraping','Output is fast/live','snapshots support performance without stale validity','direct and share views use same canonical visibility rules','first_seen/last_seen are preserved','visible_until/exact expiry controls disappearance','configs may disappear shortly before scheduled invalidation','refresh must avoid OOM regressions','no-cache user expectation is preserved'],
'residential':['residential CRUD/test is independent','panel association/outbound tag/priority are explicit','Sanaei synchronization is isolated from account control-plane proxy','residential country/egress behavior is observable'],
'database':['migrations are append-only ordered history','migration checksum integrity is enforced','live schema must be checked before schema-sensitive work','continuity snapshots store metadata not user rows/secrets','schema ownership is traceable to code and migrations'],
'admin_api':['routes remain explicit and traceable to handlers','auth protects mutating/admin operations','account/provider/config/output/proxy/residential APIs preserve module boundaries','errors should remain classifiable/actionable','settings changes do not silently alter unrelated modules'],
'frontend':['seven operational module pages remain navigable','UI actions map to explicit APIs','mobile/operational usability is preserved','refresh/save should preserve working context where possible','interactive challenge flows provide visible browser feedback','frontend must not invent backend state'],
'tests':['focused subsystem tests precede broad verification','provider regressions have characterization tests','runtime-visible changes require real flow smoke verification','test maps trace features to verification commands','handoff knowledge gates do not substitute for product tests'],
'deployment_runtime':['builds are stamped with commit/time','artifact hashes bind binaries to build manifest','health and readiness are checked after deploy','production revision claims require verifier success','Git HEAD may differ from production until deploy','secrets never enter continuity docs'],
'continuity':['new chat starts from canonical repo not main assumptions','knowledge artifacts are commit-pinned','exact line questions use source snapshot not guesses','intent/history/impact are read before behavioral changes','audit evidence is preserved','100/100 means tested recoverability/traceability not unaided token memorization']}
# Map areas to existing requirement IDs where possible.
old=json.load(open('docs/INTENT_REQUIREMENTS_BRAIN.json',encoding='utf-8'))['requirements']; byarea={}
for x in old:byarea.setdefault(x['area'],[]).append(x['id'])
# aliases
aliases={'architecture':'architecture','accounts':'accounts','digitalocean':'digitalocean','vultr':'vultr','proxy_network':'proxy_network','lifecycle':'lifecycle','sanaei_panel':'sanaei','reality':'reality','output':'output','residential':'residential','deployment_runtime':'production','continuity':'continuity','frontend':'frontend'}
items=[]
for area,vals in areas.items():
 for n,text in enumerate(vals,1):
  items.append({'id':f'DEEP-{area.upper().replace("_","-")}-{n:02d}','area':area,'requirement':text,'parent_requirement_ids':byarea.get(aliases.get(area,area),[]),'status':'normative','source_commit':head})
out={'schema_version':1,'source_commit':head,'requirement_count':len(items),'areas':len(areas),'requirements':items,'principle':'These are durable project intent/behavior requirements for knowledge continuity; exact implementation is verified through traceability/source/runtime brains.'}
json.dump(out,open('docs/DEEP_REQUIREMENTS_BRAIN.json','w'),ensure_ascii=False,indent=2);open('docs/DEEP_REQUIREMENTS_BRAIN.json','a').write('\n')
md=['# Deep Requirements & Intent Brain','',f'Commit `{head}` — **{len(items)} requirements across {len(areas)} knowledge areas**.','', 'This expands the coarse requirement brain into durable behavioral intent. It exists to teach a fresh session *why* the project behaves as it does, not merely where code lives.','']
for area in areas:
 md += [f'## {area}']+[f'- `{x["id"]}` — {x["requirement"]}' for x in items if x['area']==area]+['']
open('docs/DEEP_REQUIREMENTS_BRAIN.md','w').write('\n'.join(md)+'\n')
print('deep requirements',len(items),'areas',len(areas),'commit',head[:12])
