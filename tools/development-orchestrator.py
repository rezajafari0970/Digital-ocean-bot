#!/usr/bin/env python3
import fcntl,json,os,pathlib,subprocess,tempfile,time
from datetime import datetime,timezone
ROOT=pathlib.Path(__file__).resolve().parents[1]
os.environ["PATH"] = "/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:" + os.environ.get("PATH", "")
os.environ.setdefault("HOME", "/root")
os.environ.setdefault("GOPATH", "/root/go")
os.environ.setdefault("GOMODCACHE", "/root/go/pkg/mod")
BASE=ROOT/".local"/"dev-orchestrator"; STATE=BASE/"state.json"; LOCK=BASE/"lock"; LOG=BASE/"events.jsonl"
JOBS=ROOT/"docs"/"development-jobs"; ADAPTER=ROOT/"tools"/"openai-development-adapter.py"; COUNCIL=ROOT/"tools"/"ai-engineering-council.py"; ROUTER=ROOT/"tools"/"ai-risk-router.py"
PHASES=["PLAN","IMPLEMENT","TEST","VERIFY","CHECKPOINT","COMPLETE"]
def now(): return datetime.now(timezone.utc).isoformat()
def write_state(x):
    BASE.mkdir(parents=True,exist_ok=True)
    fd,p=tempfile.mkstemp(dir=BASE,prefix="state.")
    with os.fdopen(fd,"w") as f: json.dump(x,f,indent=2); f.write("\n"); f.flush(); os.fsync(f.fileno())
    os.replace(p,STATE)
def state():
    if STATE.exists(): return json.loads(STATE.read_text())
    return {"schema":1,"jobs":{},"updated_at":now()}
def ev(**x):
    BASE.mkdir(parents=True,exist_ok=True)
    with LOG.open("a") as f: f.write(json.dumps({"time":now(),**x})+"\n")
def cmd(c,timeout=240):
    p=subprocess.run(c,cwd=ROOT,shell=True,text=True,capture_output=True,timeout=timeout)
    ev(event="command",command=c,returncode=p.returncode,stdout=p.stdout[-4000:],stderr=p.stderr[-4000:])
    return p.returncode
def git_status():
    p=subprocess.run(["git","status","--porcelain=v1","-z"],cwd=ROOT,capture_output=True)
    if p.returncode: raise RuntimeError("git status failed")
    out={}
    for raw in p.stdout.split(b"\0"):
        if not raw: continue
        s=raw.decode("utf-8","surrogateescape")
        status=s[:2]; path=s[3:]
        if " -> " in path: path=path.split(" -> ",1)[1]
        out[path]=status
    return out
def scope_ok(baseline,allowed):
    current=git_status()
    changed={p for p,v in current.items() if baseline.get(p)!=v} | {p for p in baseline if current.get(p)!=baseline.get(p)}
    bad=sorted(p for p in changed if not any(p==a or p.startswith(a.rstrip("/")+"/") for a in allowed))
    return bad
def allowed_path(path,allowed):
    return any(path==a or path.startswith(a.rstrip("/")+"/") for a in allowed)
def clean_patch(text):
    text=text.strip()
    if text.startswith("```"):
        lines=text.splitlines()
        if lines and lines[0].startswith("```"): lines=lines[1:]
        if lines and lines[-1].strip()=="```": lines=lines[:-1]
        text="\n".join(lines)
    return text.strip()+"\n"
def patch_paths(patch):
    paths=set()
    for line in patch.splitlines():
        if line.startswith("+++ b/") or line.startswith("--- a/"):
            paths.add(line[6:])
    return paths
def parse_file_replacements(text,allowed):
    try: data=json.loads(text)
    except Exception: return None
    changes=data.get("files") if isinstance(data,dict) else None
    if not isinstance(changes,list) or not changes: return None
    out=[]; seen=set()
    for item in changes:
        if not isinstance(item,dict): return None
        path=item.get("path"); content=item.get("content")
        if not isinstance(path,str) or not isinstance(content,str) or not allowed_path(path,allowed) or path in seen: return None
        seen.add(path); out.append((path,content))
    return out
def parse_edit_operations(text,allowed):
    try: data=json.loads(text)
    except Exception: return None
    edits=data.get("edits") if isinstance(data,dict) else None
    if not isinstance(edits,list) or not edits: return None
    out=[]
    for item in edits:
        if not isinstance(item,dict): return None
        path,old,new=item.get("path"),item.get("old"),item.get("new")
        if not all(isinstance(x,str) for x in (path,old,new)) or not old or not allowed_path(path,allowed): return None
        out.append((path,old,new))
    return out
def apply_edit_operations(job,edits):
    backup={}
    try:
        for rel,old,new in edits:
            p=ROOT/rel
            if not p.is_file(): raise RuntimeError("edit target missing: "+rel)
            if rel not in backup: backup[rel]=p.read_bytes()
            text=p.read_text()
            if text.count(old)!=1: raise RuntimeError("edit old block must match exactly once: "+rel)
            p.write_text(text.replace(old,new,1))
        paths=sorted(backup)
        p=subprocess.run(["git","diff","--check","--",*paths],cwd=ROOT,text=True,capture_output=True)
        if p.returncode: raise RuntimeError(p.stderr[-4000:])
        ev(event="api_structured_edits_applied",job=job,paths=paths,count=len(edits)); return True
    except Exception as e:
        for rel,data in backup.items(): (ROOT/rel).write_bytes(data)
        ev(event="api_structured_edit_failure",job=job,error=str(e)); return False
def apply_file_replacements(job,changes):
    backup={}
    try:
        for rel,content in changes:
            p=ROOT/rel
            if not p.is_file(): raise RuntimeError("replacement target missing: "+rel)
            backup[rel]=p.read_bytes()
            p.write_text(content)
        p=subprocess.run(["git","diff","--check","--",*[x[0] for x in changes]],cwd=ROOT,text=True,capture_output=True)
        if p.returncode: raise RuntimeError(p.stderr[-4000:])
        ev(event="api_structured_files_applied",job=job,paths=[x[0] for x in changes])
        return True
    except Exception as e:
        for rel,data in backup.items(): (ROOT/rel).write_bytes(data)
        ev(event="api_structured_apply_failure",job=job,error=str(e)); return False
def api_implement(job,m,j):
    allowed=m.get("allowed_paths",[])
    if not allowed: return False
    chunks=[]
    budget=int(m.get("source_context_bytes",120000))
    for rel in allowed:
        p=ROOT/rel
        if not p.is_file(): continue
        body=p.read_text(errors="replace")
        block=f"\n--- FILE: {rel} ---\n{body}\n"
        if sum(len(x) for x in chunks)+len(block)>budget: break
        chunks.append(block)
    plan_path=BASE/(job+"-plan.json")
    plan=plan_path.read_text(errors="replace") if plan_path.exists() else ""
    structured=m.get("structured_files",False)
    edit_ops=m.get("structured_edits",False)
    if edit_ops:
        output_rule=("Return ONLY one JSON object: {\"edits\":[{\"path\":\"allowed/file\",\"old\":\"EXACT EXISTING BLOCK\",\"new\":\"REPLACEMENT BLOCK\"}]}. "
                     "Each old block must be copied exactly from SOURCE and must uniquely occur in that file. Keep edits minimal. No markdown, diff, prose, or extra keys. ")
    elif structured:
        output_rule=("Return ONLY one JSON object of the form {\"files\":[{\"path\":\"allowed/file\",\"content\":\"COMPLETE FILE CONTENT\"}]}. "
                     "Return complete replacement content only for files that must change. No markdown, diff, prose, or extra keys. ")
    else:
        output_rule="Return ONLY a valid git unified diff beginning with 'diff --git'. No explanation, prose, JSON, or markdown fences. "
    prompt=(output_rule+
            "Every modified path MUST be in ALLOWED_PATHS. Implement the bounded goal using only the supplied source. "
            "Do not invent files or APIs outside the supplied source. Preserve existing behavior unless the goal requires change. "
            "Include focused tests when a test file is allowed.\nGOAL:\n"+m["goal"]+
            "\nCONTEXT:\n"+m.get("context","")+"\nALLOWED_PATHS:\n"+"\n".join(allowed)+"\nPLAN:\n"+plan+"\nSOURCE:\n"+"".join(chunks))
    p=subprocess.run([str(ADAPTER)],input=prompt,text=True,capture_output=True,cwd=ROOT,timeout=int(m.get("api_timeout_seconds",240)))
    if p.returncode!=0:
        ev(event="api_implement_failure",job=job,returncode=p.returncode,stderr=p.stderr[-2000:]); return False
    try: model_text=json.loads(p.stdout)["text"]
    except Exception as e:
        ev(event="api_patch_parse_failure",job=job,error=str(e)); return False
    if edit_ops:
        edits=parse_edit_operations(model_text,allowed)
        if not edits:
            ev(event="api_structured_edit_parse_failure",job=job); return False
        if not apply_edit_operations(job,edits): return False
        j["api_patch_saved"]=True
        return True
    if structured:
        changes=parse_file_replacements(model_text,allowed)
        if not changes:
            ev(event="api_structured_parse_failure",job=job); return False
        if not apply_file_replacements(job,changes): return False
        j["api_patch_saved"]=True
        return True
    patch=clean_patch(model_text)
    paths=patch_paths(patch)
    if not paths or any(not allowed_path(x,allowed) for x in paths):
        ev(event="api_patch_scope_failure",job=job,paths=sorted(paths)); return False
    patch_file=BASE/(job+"-implement.patch"); patch_file.write_text(patch)
    check=subprocess.run(["git","apply","--check",str(patch_file)],cwd=ROOT,text=True,capture_output=True)
    if check.returncode:
        ev(event="api_patch_check_failure",job=job,stderr=check.stderr[-4000:])
        repair_prompt=("Return ONLY a corrected valid git unified diff beginning with 'diff --git'. No prose or fences. "
                       "Repair the candidate patch so git apply --check succeeds. Do not change its intended behavior. "
                       "Every path must remain inside ALLOWED_PATHS.\nALLOWED_PATHS:\n"+"\n".join(allowed)+
                       "\nGIT_APPLY_ERROR:\n"+check.stderr[-4000:]+"\nCANDIDATE_PATCH:\n"+patch)
        rp=subprocess.run([str(ADAPTER)],input=repair_prompt,text=True,capture_output=True,cwd=ROOT,timeout=int(m.get("api_timeout_seconds",240)))
        if rp.returncode!=0:
            ev(event="api_patch_repair_failure",job=job,returncode=rp.returncode); return False
        try: repaired=clean_patch(json.loads(rp.stdout)["text"])
        except Exception as e:
            ev(event="api_patch_repair_parse_failure",job=job,error=str(e)); return False
        repaired_paths=patch_paths(repaired)
        if not repaired_paths or any(not allowed_path(x,allowed) for x in repaired_paths):
            ev(event="api_patch_repair_scope_failure",job=job,paths=sorted(repaired_paths)); return False
        patch=repaired; patch_file.write_text(patch)
        check=subprocess.run(["git","apply","--check",str(patch_file)],cwd=ROOT,text=True,capture_output=True)
        if check.returncode:
            ev(event="api_patch_repair_check_failure",job=job,stderr=check.stderr[-4000:]); return False
        ev(event="api_patch_repaired",job=job,paths=sorted(repaired_paths))
    apply=subprocess.run(["git","apply",str(patch_file)],cwd=ROOT,text=True,capture_output=True)
    if apply.returncode:
        ev(event="api_patch_apply_failure",job=job,stderr=apply.stderr[-4000:]); return False
    j["api_patch_saved"]=True
    return True
def scoped_checkpoint(job,m,j):
    allowed=m.get("allowed_paths",[])
    if not allowed:
        ev(event="scope_error",job=job,error="allowed_paths missing"); return False
    baseline=j.get("baseline_status",{})
    bad=scope_ok(baseline,allowed)
    if bad:
        ev(event="scope_violation",job=job,paths=bad); return False
    p=subprocess.run(["git","diff","--check","--",*allowed],cwd=ROOT,text=True,capture_output=True)
    if p.returncode:
        ev(event="diff_check_failed",job=job,stderr=p.stderr[-4000:]); return False
    p=subprocess.run(["git","add","--",*allowed],cwd=ROOT,text=True,capture_output=True)
    if p.returncode: return False
    p=subprocess.run(["git","diff","--cached","--quiet","--",*allowed],cwd=ROOT)
    if p.returncode==1:
        msg=m.get("commit_message","chore(dev): checkpoint "+job)
        p=subprocess.run(["git","commit","-m",msg,"--",*allowed],cwd=ROOT,text=True,capture_output=True)
        if p.returncode:
            ev(event="commit_failed",job=job,stderr=p.stderr[-4000:]); return False
        if m.get("push",False):
            p=subprocess.run(["git","push","origin",subprocess.check_output(["git","branch","--show-current"],cwd=ROOT,text=True).strip()],cwd=ROOT,text=True,capture_output=True)
            if p.returncode:
                ev(event="push_failed",job=job,stderr=p.stderr[-4000:]); return False
    p=subprocess.run(["git","rev-parse","HEAD"],cwd=ROOT,text=True,capture_output=True)
    j["last_good_commit"]=p.stdout.strip()
    return True
def mark_retryable(s,j,job,phase,error):
    j["status"]="RETRYABLE"
    j["last_error"]=str(error)[:1000]
    j["retry_after"]=time.time()+min(300,5*(2**min(j["attempt"],6)))
    s["updated_at"]=now(); write_state(s)
    ev(event="phase_retryable",job=job,phase=phase,error=j["last_error"],retry_after=j["retry_after"])
def one(job):
    BASE.mkdir(parents=True,exist_ok=True)
    with LOCK.open("a+") as lk:
        try: fcntl.flock(lk,fcntl.LOCK_EX|fcntl.LOCK_NB)
        except BlockingIOError: return 73
        m=json.loads((JOBS/(job+".json")).read_text()); s=state()
        j=s["jobs"].setdefault(job,{"phase":"PLAN","status":"READY","attempt":0,"created_at":now(),"baseline_status":git_status()})
        if "baseline_status" not in j: j["baseline_status"]=git_status()
        if j["phase"]=="COMPLETE": return 0
        if j.get("retry_after",0)>time.time(): return 75
        phase=j["phase"]; j["status"]="RUNNING"; j["attempt"]+=1; j["started_at"]=now(); s["updated_at"]=now(); write_state(s); ev(event="phase_start",job=job,phase=phase)
        ok=True
        try:
            if phase=="PLAN":
                route_input=json.dumps({"goal":m["goal"],"context":m.get("context",""),"allowed_paths":m.get("allowed_paths",[]),"push":m.get("push",False)})
                rr=subprocess.run([str(ROUTER)],input=route_input,text=True,capture_output=True,cwd=ROOT,timeout=15)
                if rr.returncode!=0: raise RuntimeError("risk router failed")
                route=json.loads(rr.stdout); profile=route["profile"]
                j["risk_profile"]=profile; j["risk_score"]=route["score"]; j["risk_reasons"]=route["reasons"]
                j["rollback_commit"]=subprocess.check_output(["git","rev-parse","HEAD"],cwd=ROOT,text=True).strip()
                (BASE/(job+"-risk-route.json")).write_text(json.dumps(route,indent=2)+"\n")
                if profile=="critical":
                    gates=" ".join(m.get("commands",{}).get("test",[])+m.get("commands",{}).get("verify",[])).lower()
                    if "-race" not in gates and "fault" not in gates:
                        ok=False; ev(event="risk_gate_missing",job=job,profile=profile,required="race_or_fault")
                prompt="GOAL:\n"+m["goal"]+"\nCONTEXT:\n"+m.get("context","")+"\nALLOWED_PATHS:\n"+"\n".join(m.get("allowed_paths",[]))+"\nRISK_PROFILE:\n"+profile
                if ok and m.get("ai_council",True):
                    council_env=os.environ.copy(); council_env["AI_COUNCIL_PROFILE"]=profile
                    cp=subprocess.run([str(COUNCIL)],input=prompt,text=True,capture_output=True,cwd=ROOT,env=council_env,timeout=int(m.get("council_timeout_seconds",900)))
                    if cp.returncode in (0,2):
                        (BASE/(job+"-council.json")).write_text(cp.stdout)
                        council=json.loads(cp.stdout); verdict=council.get("verdict",{})
                        j["council_decision"]=verdict.get("decision","REVISE"); j["ai_council_used"]=True
                        (BASE/(job+"-adjudication.json")).write_text(json.dumps(verdict,indent=2)+"\n")
                        if j["council_decision"]!="PASS" and m.get("block_on_council",False):
                            ok=False; ev(event="council_blocked",job=job,findings=verdict.get("unresolved",[]))
                    else: ok=False; ev(event="council_failure",job=job,returncode=cp.returncode,stderr=cp.stderr[-2000:])
                if ok:
                    evidence=(BASE/(job+"-adjudication.json")).read_text(errors="replace") if (BASE/(job+"-adjudication.json")).exists() else "{}"
                    plan_prompt="Create a concise bounded implementation plan. Use the council verdict as constraints; do not execute commands.\n"+prompt+"\nCOUNCIL_VERDICT:\n"+evidence
                    p=subprocess.run([str(ADAPTER)],input=plan_prompt,text=True,capture_output=True,cwd=ROOT,timeout=int(m.get("api_timeout_seconds",240)))
                    if p.returncode==0:
                        (BASE/(job+"-plan.json")).write_text(p.stdout); j["api_plan_saved"]=True
                    else:
                        ok=False; ev(event="api_failure",job=job,returncode=p.returncode,stderr=p.stderr[-2000:])
            elif phase=="IMPLEMENT":
                if m.get("api_implement",False):
                    ok=api_implement(job,m,j)
                if ok:
                    for c in m.get("commands",{}).get("implement",[]):
                        if cmd(c,int(m.get("timeout_seconds",240)))!=0: ok=False; break
            elif phase in ("TEST","VERIFY"):
                for c in m.get("commands",{}).get(phase.lower(),[]):
                    if cmd(c,int(m.get("timeout_seconds",240)))!=0: ok=False; break
            elif phase=="CHECKPOINT":
                ok=scoped_checkpoint(job,m,j)
        except subprocess.TimeoutExpired as e:
            mark_retryable(s,j,job,phase,"timeout: "+str(e)); return 75
        except Exception as e:
            mark_retryable(s,j,job,phase,"exception: "+repr(e)); return 75
        if not ok:
            mark_retryable(s,j,job,phase,"phase returned failure"); return 75
        j["retry_after"]=0; j.pop("last_error",None); j["phase"]=PHASES[PHASES.index(phase)+1]
        j["status"]="COMPLETE" if j["phase"]=="COMPLETE" else "READY"; s["updated_at"]=now()
        write_state(s); ev(event="phase_done",job=job,phase=phase,next=j["phase"]); return 0
def main():
    job=os.environ.get("DEV_JOB","proxy-control-plane")
    while True:
        rc=one(job)
        s=state(); j=s["jobs"].get(job,{})
        if j.get("phase")=="COMPLETE": return
        time.sleep(5 if rc==0 else 15)
if __name__=="__main__": main()
