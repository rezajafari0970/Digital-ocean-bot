#!/usr/bin/env python3
import json, sys

HIGH_TERMS = {
    "migration":4, "schema":4, "sql":3, "database":3, "transaction":3,
    "concurrency":4, "race":4, "deadlock":4, "auth":4, "secret":4,
    "credential":4, "security":4, "proxy":3, "generation":3,
    "circuit":3, "half-open":4, "production":4, "deploy":3, "rollback":3
}
MED_TERMS = {
    "worker":2, "retry":2, "timeout":2, "cache":2, "api":2,
    "provider":2, "runtime":2, "state":2, "lifecycle":2, "ssh":2
}
HIGH_PATHS = (
    "migrations/", "internal/auth/", "internal/secrets/", "internal/proxycontrol/",
    "internal/providers/", "internal/droplets/", "internal/worker/", "cmd/worker/"
)
MED_PATHS = ("internal/app/", "internal/network/", "internal/adminapi/", "internal/panels/", "cmd/api/")

def classify(payload):
    text=(payload.get("goal","")+" "+payload.get("context","")).lower()
    paths=payload.get("allowed_paths",[])
    score=0
    reasons=[]
    for key,weight in HIGH_TERMS.items():
        if key in text:
            score+=weight
            reasons.append(key)
    for key,weight in MED_TERMS.items():
        if key in text:
            score+=weight
            reasons.append(key)
    for path in paths:
        if path.endswith(".sql") or path.startswith("migrations/"):
            score+=5
            reasons.append("migration_path")
        elif path.startswith(HIGH_PATHS):
            score+=4
            reasons.append("high_risk_path:"+path)
        elif path.startswith(MED_PATHS):
            score+=2
            reasons.append("medium_risk_path:"+path)

    if len(paths)>=8:
        score+=2
        reasons.append("broad_scope")
    if payload.get("push"):
        score+=1
        reasons.append("push")
    if score>=12:
        profile="critical"
    elif score>=7:
        profile="high"
    elif score>=3:
        profile="medium"
    else:
        profile="low"
    return {"schema":1,"profile":profile,"score":score,"reasons":sorted(set(reasons))}

def main():
    raw=sys.stdin.read()
    if not raw.strip():
        raise SystemExit(64)
    try:
        payload=json.loads(raw)
    except Exception:
        payload={"goal":raw,"context":"","allowed_paths":[]}
    print(json.dumps(classify(payload),ensure_ascii=False))

if __name__=="__main__":
    main()
