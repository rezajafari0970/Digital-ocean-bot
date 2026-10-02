#!/usr/bin/env python3
import json, os, pathlib, subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1]
ENV=pathlib.Path("/etc/digital-ocean-bot/openai.env")
if not os.environ.get("OPENAI_API_KEY") and ENV.exists():
 for raw in ENV.read_text().splitlines():
  raw=raw.strip()
  if raw and not raw.startswith("#") and "=" in raw:
   k,v=raw.split("=",1); os.environ.setdefault(k.strip(),v.strip().strip(chr(34)).strip(chr(39)))
AD=ROOT/"tools/openai-development-adapter.py"
candidates=[
 "gpt-6-astra","gpt-6-sol","gpt-6-luna",
 "gpt-5.6","gpt-5.6-sol","gpt-5.6-terra","gpt-5.6-luna"
]
out=[]
for model in candidates:
 env=os.environ.copy(); env["OPENAI_MODEL"]=model; env["OPENAI_MAX_OUTPUT_TOKENS"]="64"; env["OPENAI_HTTP_TIMEOUT_SECONDS"]="45"
 p=subprocess.run([str(AD)],input="Reply with exactly OK.",text=True,capture_output=True,cwd=ROOT,env=env,timeout=55)
 item={"model":model,"ok":p.returncode==0,"returncode":p.returncode}
 if p.returncode==0:
  try: item["response_id"]=json.loads(p.stdout).get("id")
  except Exception: item["ok"]=False; item["parse_error"]=True
 out.append(item)
print(json.dumps({"schema":1,"models":out},ensure_ascii=False))
