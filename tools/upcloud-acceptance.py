#!/usr/bin/env python3
"""Local readback for provider rollout; never creates cloud resources."""
import argparse, hashlib, json, os, pathlib, secrets, subprocess, urllib.request, uuid
def sql(query):
    p=subprocess.run(["psql",os.environ["DATABASE_URL"],"-XAt","-v","ON_ERROR_STOP=1","-c",query],capture_output=True,text=True)
    if p.returncode: raise RuntimeError("local acceptance SQL failed")
    return p.stdout.strip()
def rows(table,order):
    return json.loads(sql("SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY "+order+"),'[]'::jsonb) FROM "+table+" t"))
def snapshot():
    return {
      "profiles":rows("reality_config_profiles","route_class"),
      "gate":rows("client_mutation_execution_gate","singleton"),
      "routing_control":rows("residential_routing_control","singleton"),
      "publication":json.loads(sql("SELECT COALESCE(jsonb_agg(x ORDER BY id),'[]'::jsonb) FROM (SELECT id,spec,state,duration_mode,publish_scope,deadline FROM residential_performance_experiments WHERE state IN('RUNNING','KEPT','ROLLING_BACK'))x")),
      "endpoint_configuration_hash":sql("SELECT md5(COALESCE(jsonb_agg(jsonb_build_array(proxy_id,name,type,host,port,username,secret_ref,enabled,outbound_tag) ORDER BY proxy_id)::text,'')) FROM residential_proxies"),
    }
def read_api(outdir):
    user=str(uuid.uuid4()); token=secrets.token_hex(32); digest=hashlib.sha256(token.encode()).hexdigest()
    sql("BEGIN; INSERT INTO admin_users(id,username,password_hash,role) VALUES('"+user+"','upcloud-acceptance-"+user+"','!','admin'); INSERT INTO admin_sessions(id,user_id,token_hash,expires_at) VALUES(gen_random_uuid(),'"+user+"',decode('"+digest+"','hex'),now()+interval '2 minutes'); COMMIT;")
    try:
        def get(path):
            req=urllib.request.Request("http://127.0.0.1:18080"+path,headers={"Authorization":"Bearer "+token})
            with urllib.request.urlopen(req,timeout=15) as res:
                assert res.status==200
                return json.load(res)
        providers=get("/api/v1/providers")
        by={x["name"]:x for x in providers}
        assert all(by[x]["status"]=="ready" for x in ["digitalocean","vultr","upcloud"])
        assert "ucat_" in by["upcloud"]["credential_label"]
        result={"providers":providers,"api_checks":{}}
        for path in ["/api/v1/accounts","/api/v1/proxies","/api/v1/residential-proxies","/api/v1/residential-performance","/api/v1/resources","/api/v1/configs"]:
            val=get(path);result["api_checks"][path]={"status":200,"type":type(val).__name__}
        (outdir/"api-readback.json").write_text(json.dumps(result,indent=2)+"\n")
        script=os.environ.get("UPCLOUD_BROWSER_SCRIPT")
        if script:
            env=dict(os.environ,DOB_UI_BASE="http://127.0.0.1:18080",DOB_UI_TOKEN=token,DOB_UI_ARTIFACT_DIR=str(outdir))
            subprocess.run(["node",script],env=env,check=True)
    finally:
        sql("DELETE FROM admin_users WHERE id='"+user+"' AND username='upcloud-acceptance-"+user+"';")
p=argparse.ArgumentParser();p.add_argument("phase",choices=["before","after"]);p.add_argument("evidence_dir")
a=p.parse_args();out=pathlib.Path(a.evidence_dir);out.mkdir(parents=True,exist_ok=True)
value=snapshot();(out/(a.phase+"-configuration.json")).write_text(json.dumps(value,indent=2)+"\n")
if a.phase=="after":
    baseline=json.loads((out/"before-configuration.json").read_text())
    assert value==baseline,"configuration changed: review before publishing acceptance"
    read_api(out)
    assert sql("SELECT version::text||':'||dirty::text FROM schema_migrations")=="153:false"
    print("UPCLOUD_REGISTERED_ALL_EXISTING_PROVIDERS_CONFIGURATION_PRESERVED_MIGRATION153 PASS")
else:
    print("BASELINE_SAVED; active UpCloud accounts:",sql("SELECT count(*) FROM accounts WHERE provider='upcloud' AND deleted_at IS NULL"))
