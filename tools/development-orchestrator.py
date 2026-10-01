#!/usr/bin/env python3
import fcntl,json,os,pathlib,subprocess,tempfile,time
from datetime import datetime,timezone
ROOT=pathlib.Path(__file__).resolve().parents[1]
os.environ["PATH"] = "/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:" + os.environ.get("PATH", "")
os.environ.setdefault("HOME", "/root")
os.environ.setdefault("GOPATH", "/root/go")
os.environ.setdefault("GOMODCACHE", "/root/go/pkg/mod")
BASE=ROOT/".local"/"dev-orchestrator"; STATE=BASE/"state.json"; LOCK=BASE/"lock"; LOG=BASE/"events.jsonl"
JOBS=ROOT/"docs"/"development-jobs"; ADAPTER=ROOT/"tools"/"openai-development-adapter.py"
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
    for raw in p.stdout.split(b"\\0"):
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
        phase=j["phase"]; j["status"]="RUNNING"; j["attempt"]+=1; write_state(s); ev(event="phase_start",job=job,phase=phase)
        ok=True
        if phase=="PLAN":
            prompt="Create a concise bounded implementation plan. Do not execute commands.\nGOAL:\n"+m["goal"]+"\nCONTEXT:\n"+m.get("context","")
            p=subprocess.run([str(ADAPTER)],input=prompt,text=True,capture_output=True,cwd=ROOT,timeout=200)
            if p.returncode==0:
                (BASE/(job+"-plan.json")).write_text(p.stdout); j["api_plan_saved"]=True
            else: ok=False; ev(event="api_failure",job=job,returncode=p.returncode)
        elif phase in ("IMPLEMENT","TEST","VERIFY"):
            for c in m.get("commands",{}).get(phase.lower(),[]):
                if cmd(c,int(m.get("timeout_seconds",240)))!=0: ok=False; break
        elif phase=="CHECKPOINT":
            ok=scoped_checkpoint(job,m,j)
        if not ok:
            j["status"]="RETRYABLE"; j["retry_after"]=time.time()+min(300,5*(2**min(j["attempt"],6))); write_state(s); return 75
        j["retry_after"]=0; j["phase"]=PHASES[PHASES.index(phase)+1]; j["status"]="COMPLETE" if j["phase"]=="COMPLETE" else "READY"; s["updated_at"]=now(); write_state(s); ev(event="phase_done",job=job,phase=phase,next=j["phase"]); return 0
def main():
    job=os.environ.get("DEV_JOB","proxy-control-plane")
    while True:
        rc=one(job)
        s=state(); j=s["jobs"].get(job,{})
        if j.get("phase")=="COMPLETE": return
        time.sleep(5 if rc==0 else 15)
if __name__=="__main__": main()
