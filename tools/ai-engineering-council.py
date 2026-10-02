#!/usr/bin/env python3
import concurrent.futures, hashlib, json, os, pathlib, subprocess, sys
from datetime import datetime, timezone
ROOT=pathlib.Path(__file__).resolve().parents[1]
ENV=pathlib.Path("/etc/digital-ocean-bot/openai.env")
if not os.environ.get("OPENAI_API_KEY") and ENV.exists():
 for raw in ENV.read_text().splitlines():
  raw=raw.strip()
  if raw and not raw.startswith("#") and "=" in raw:
   k,v=raw.split("=",1); os.environ.setdefault(k.strip(),v.strip().strip(chr(34)).strip(chr(39)))
REG=json.loads((ROOT/"docs/AI_MODEL_REGISTRY.json").read_text())
AD=ROOT/"tools/openai-development-adapter.py"; OUT=ROOT/".local/ai-council"
ROLE={
"architect":"Design the smallest correct architecture. State invariants, ownership, impact, assumptions, non-goals, rollback and blockers.",
"reviewer":"Independently find correctness, architecture, compatibility, SQL, concurrency, security and maintainability defects.",
"adversary":"Try to break the change with races, stale state, partial failure, retries, timeout, isolation, leakage and rollback failure.",
"test_designer":"Design tests independently from requirements: deterministic edge, regression, race, fault, property and provider-parity tests where relevant."
}
def invoke(role,models,base):
 errors=[]
 for model in models:
  env=os.environ.copy(); env["OPENAI_MODEL"]=model
  p=subprocess.run([str(AD)],input=f"ROLE={role}\n{ROLE.get(role,'Adjudicate by evidence.')}\nTASK:\n{base}",text=True,capture_output=True,cwd=ROOT,env=env,timeout=180)
  if p.returncode==0:
   try:
    x=json.loads(p.stdout); return {"role":role,"ok":True,"model":model,"response_id":x.get("id"),"text":x["text"]}
   except Exception as e: errors.append(model+":parse:"+repr(e)); continue
  errors.append(model+":rc="+str(p.returncode))
 return {"role":role,"ok":False,"models":models,"errors":errors}
base=sys.stdin.read()
if not base.strip(): raise SystemExit(64)
roles=["architect","reviewer","adversary","test_designer"]
with concurrent.futures.ThreadPoolExecutor(max_workers=REG["parallelism"]) as ex:
 results=list(ex.map(lambda r: invoke(r,REG["lanes"][r]["models"],base),roles))
ok=[x for x in results if x["ok"]]
bundle={"schema":2,"created_at":datetime.now(timezone.utc).isoformat(),"head":subprocess.check_output(["git","rev-parse","HEAD"],cwd=ROOT,text=True).strip(),"input_sha256":hashlib.sha256(base.encode()).hexdigest(),"results":results}
if len(ok)<REG["minimum_successful_roles"]:
 bundle["status"]="INSUFFICIENT_COUNCIL"; OUT.mkdir(parents=True,exist_ok=True); (OUT/"latest-council.json").write_text(json.dumps(bundle,indent=2)+"\n"); print(json.dumps(bundle)); raise SystemExit(75)
reports="\n\n".join(f"ROLE={x['role']} MODEL={x['model']}\n{x['text']}" for x in ok)
judge_prompt=("TASK:\n"+base+"\n\nINDEPENDENT REPORTS:\n"+reports+
"\n\nAct as evidence adjudicator. Majority vote is not evidence. Return ONLY JSON with keys decision (PASS|REVISE|BLOCK), agreements, disagreements, unresolved, required_gates, chosen_plan. PASS requires unresolved=[] and a bounded chosen_plan.")
judge=invoke("adjudicator",REG["lanes"]["adjudicator"]["models"],judge_prompt)
bundle["adjudicator"]=judge
if not judge["ok"]: bundle["status"]="NO_ADJUDICATION"; verdict={"decision":"REVISE","unresolved":["adjudicator unavailable"]}
else:
 try: verdict=json.loads(judge["text"])
 except Exception: verdict={"decision":"REVISE","unresolved":["adjudicator returned non-JSON"],"raw":judge["text"]}
 bundle["verdict"]=verdict; bundle["status"]="READY" if verdict.get("decision")=="PASS" and not verdict.get("unresolved") else "REVIEW_REQUIRED"
OUT.mkdir(parents=True,exist_ok=True); (OUT/"latest-council.json").write_text(json.dumps(bundle,indent=2,ensure_ascii=False)+"\n")
print(json.dumps(bundle,ensure_ascii=False))
if bundle["status"]!="READY": raise SystemExit(2)
