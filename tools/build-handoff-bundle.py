#!/usr/bin/env python3
import os,json,subprocess,datetime
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
def sh(*a): return subprocess.check_output(a,text=True).strip()
def read(p):
    try:return open(p,encoding='utf-8').read().strip()
    except:return ''
head=sh('git','rev-parse','HEAD'); branch=sh('git','branch','--show-current')
manifest=json.load(open('docs/PROJECT_MANIFEST.json',encoding='utf-8'))
snapshot=json.load(open('docs/CURRENT_SNAPSHOT.json',encoding='utf-8'))
parts=[
'# SESSION HANDOFF BUNDLE',
'> Generated continuity entrypoint. Read this first; source/runtime remain authoritative.',
f'Generated UTC: {datetime.datetime.now(datetime.timezone.utc).isoformat()}\nBranch: `{branch}`\nHEAD at bundle generation: `{head}`',
'## Mandatory startup procedure\n1. Read this bundle completely.\n2. Run `tools/check-project-continuity.sh`.\n3. Read `HANDOFF.md` and inspect the exact source files/commits relevant to Current Work.\n4. Verify production/runtime and live migration state before mutation.\n5. Continue from the stated boundary; do not redesign from zero.',
'## Current State\n'+read('PROJECT_STATE.md'),
'## Exact Handoff\n'+read('HANDOFF.md'),
'## Master Context\n'+read('docs/CHATGPT_MASTER_CONTEXT.md'),
'## Architecture / Codebase Map\n'+read('docs/CODEBASE_MAP.md'),
'## Feature Flow Index\n'+read('docs/FEATURE_FLOW_INDEX.md'),
'## Historical Decisions\n'+read('docs/HISTORICAL_DECISION_LEDGER.md'),
'## Requirements / Invariants\n'+read('docs/REQUIREMENTS_MATRIX.md'),
'## Change Protocol\n'+read('docs/CHANGE_PROTOCOL.md'),
'## Machine Snapshot\n```json\n'+json.dumps({'branch':branch,'head':head,'services':snapshot.get('services',{}),'inventory':manifest.get('inventory',{}),'route_count':len(manifest.get('routes',[])),'recent_migrations':manifest.get('recent_migrations',[]),'current_work':manifest.get('current_work',{}),'recent_commits':snapshot.get('recent_commits',[])},ensure_ascii=False,indent=2)+'\n```',
'## Source-of-truth rule\nThis bundle is an accelerator, not a substitute for code. If prose conflicts with Git source, live DB migration state, service logs or production runtime, investigate the drift and update continuity records after resolving it.'
]
open('docs/SESSION_HANDOFF_BUNDLE.md','w',encoding='utf-8').write('\n\n'.join(parts)+'\n')
print('bundle written',sum(len(x) for x in parts),'chars')
