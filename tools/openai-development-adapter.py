#!/usr/bin/env python3
import json, os, sys, urllib.request, urllib.error
prompt=sys.stdin.read()
if not prompt.strip(): raise SystemExit(64)
key=os.environ.get("OPENAI_API_KEY")
if not key: raise SystemExit(78)
payload={"model":os.environ.get("OPENAI_MODEL","gpt-5.6"),"input":prompt,"max_output_tokens":int(os.environ.get("OPENAI_MAX_OUTPUT_TOKENS","8000"))}
req=urllib.request.Request("https://api.openai.com/v1/responses",json.dumps(payload).encode(),{"Authorization":"Bearer "+key,"Content-Type":"application/json"})
try:
    with urllib.request.urlopen(req,timeout=180) as r: data=json.load(r)
except urllib.error.HTTPError as e:
    raise SystemExit(75 if e.code==429 or e.code>=500 else 1)
except Exception:
    raise SystemExit(75)
parts=[]
for o in data.get("output",[]):
    for c in o.get("content",[]):
        if c.get("type")=="output_text": parts.append(c.get("text",""))
print(json.dumps({"id":data.get("id"),"text":"".join(parts).strip()},ensure_ascii=False))
