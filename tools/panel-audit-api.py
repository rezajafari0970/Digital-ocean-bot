#!/usr/bin/env python3
"""One measured API generation; preserves the complete receipt before parsing."""
import datetime,json,os,pathlib,sys,urllib.request,uuid
prompt=sys.stdin.read()
if not prompt.strip():raise SystemExit(64)
base=pathlib.Path(__file__).resolve().parents[1]/'.local/panel-audit/api'
base.mkdir(parents=True,exist_ok=True)
payload={'model':os.environ.get('OPENAI_MODEL','gpt-5.6-sol'),'input':prompt,'max_output_tokens':int(os.environ.get('OPENAI_MAX_OUTPUT_TOKENS','3000'))}
if os.environ.get('OPENAI_REASONING_EFFORT'):payload['reasoning']={'effort':os.environ['OPENAI_REASONING_EFFORT']}
req=urllib.request.Request('https://api.openai.com/v1/responses',json.dumps(payload).encode(),{'Authorization':'Bearer '+os.environ['OPENAI_API_KEY'],'Content-Type':'application/json'})
with urllib.request.urlopen(req,timeout=280) as f:r=json.load(f)
rid=r.get('id') or str(uuid.uuid4())
(base/(rid+'.json')).write_text(json.dumps(r,indent=2)+'\n')
text=''.join(c.get('text','') for o in r.get('output',[]) for c in o.get('content',[]) if c.get('type')=='output_text').strip()
print(json.dumps({'id':rid,'text':text,'usage':r.get('usage'),'model':r.get('model')}))
