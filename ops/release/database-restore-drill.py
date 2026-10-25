#!/usr/bin/env python3
"""Restore and interrupt synthetic fixtures in a disposable socket-only cluster."""
import datetime
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
from urllib.parse import urlencode

def main(repo):
    if os.geteuid() == 0:
        raise RuntimeError("unprivileged independent runner required")
    repo = Path(repo).resolve(strict=True)
    sys.path.insert(0, str(repo / "deploy"))
    from backup_store import seal, verified
    candidates = [p for p in Path("/usr/lib/postgresql").glob("*/bin")
                  if p.parent.name.isdigit() and all((p/n).is_file() for n in ("initdb","pg_ctl","pg_dump","pg_restore","pg_amcheck","psql","createdb"))]
    if not candidates:
        raise RuntimeError("PostgreSQL drill binaries unavailable")
    pg = max(candidates, key=lambda p: int(p.parent.name))
    work = Path(tempfile.mkdtemp(prefix="dob-restore-drill-"))
    data, socket = work/"data", work/"socket"
    socket.mkdir(mode=0o700)
    env = {k:v for k,v in os.environ.items()
           if not k.startswith("PG") and k not in ("DATABASE_URL","BULK_TEST_DATABASE_URL","DOB_E2E_DSN","RESTORE_DRILL_DSN","RESTORE_DRILL_KEY_FILE")}
    env.update(PGHOST=str(socket),PGPORT="55437",PGUSER="dob_drill",PGDATABASE="postgres",PGCONNECT_TIMEOUT="5",
               DOB_DISPOSABLE_RESTORE_DRILL="1")
    interrupted = None
    def run(args, **kw):
        return subprocess.run([str(x) for x in args],env=env,cwd=repo,check=True,timeout=180,capture_output=True,text=True,**kw)
    def sql(query, db="postgres"):
        return run([pg/"psql","-XqAt","-v","ON_ERROR_STOP=1","-d",db,"-c",query]).stdout.strip()
    def fixture(mode, db):
        env["RESTORE_DRILL_DSN"]="postgresql://dob_drill@/"+db+"?"+urlencode({"host":str(socket),"port":"55437","sslmode":"disable"})
        return json.loads(run(["go","run","./ops/release/restore-fixture.go",mode]).stdout)
    try:
        run([pg/"initdb","-D",data,"-U","dob_drill","--auth-local=trust","--auth-host=reject","--data-checksums","--no-instructions"])
        with (data/"postgresql.conf").open("a") as f:
            f.write("\nlisten_addresses=''\nunix_socket_directories='"+str(socket)+"'\nunix_socket_permissions=0700\nport=55437\nmax_connections=20\n")
        run([pg/"pg_ctl","-D",data,"-l",work/"postgres.log","-w","start"])
        if sql("SELECT current_setting('data_directory')||'|'||current_setting('listen_addresses')||'|'||current_setting('data_checksums')") != str(data)+"||on":
            raise RuntimeError("cluster identity or checksum isolation failed")
        sql("CREATE ROLE digitaloceanbot; CREATE ROLE digitaloceanbot_readonly;")
        for db in ("dob_restore_source","dob_restore_target","dob_restore_damaged"):
            run([pg/"createdb",db])
        configuration=work/"source-configuration";configuration.mkdir(mode=0o700)
        (configuration/"master.key").write_bytes(bytes([7])*32)
        (configuration/"master.key").chmod(0o600)
        env["RESTORE_DRILL_KEY_FILE"]=str(configuration/"master.key")
        before=fixture("seed","dob_restore_source")
        root=work/"backups";root.mkdir(mode=0o700)
        pending=root/".pending";pending.mkdir(mode=0o700)
        run([pg/"pg_dump","--format=custom","--no-owner","-d","dob_restore_source","-f",pending/"database.dump"])
        shutil.copytree(configuration,pending/"configuration")
        stamp=datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S.%fZ")
        backup=seal(pending,root/stamp)
        if not verified(backup):
            raise RuntimeError("sealed fixture rejected")
        run([pg/"pg_restore","--exit-on-error","--single-transaction","--no-owner","-d","dob_restore_target",backup/"database.dump"])
        restored_configuration=work/"restored-configuration"
        shutil.copytree(backup/"configuration",restored_configuration)
        env["RESTORE_DRILL_KEY_FILE"]=str(restored_configuration/"master.key")
        after=fixture("verify","dob_restore_target")
        if before["digest"]!=after["digest"] or not after["sequence_verified"]:
            raise RuntimeError("restored identity, data, schema or sequence mismatch")
        run([pg/"pg_amcheck","--install-missing","--jobs=1","-d","dob_restore_target"])
        damaged=root/"20000101T000000.000001Z"
        shutil.copytree(backup,damaged)
        blob=(damaged/"database.dump").read_bytes()
        (damaged/"database.dump").write_bytes(blob[:len(blob)//2])
        if verified(damaged):
            raise RuntimeError("corrupted backup passed sealing verification")
        bad=subprocess.run([str(pg/"pg_restore"),"--exit-on-error","--single-transaction","--no-owner","-d","dob_restore_damaged",str(damaged/"database.dump")],
                           env=env,capture_output=True,text=True,timeout=60)
        if bad.returncode==0 or sql("SELECT to_regclass('public.accounts') IS NULL","dob_restore_damaged")!="t":
            raise RuntimeError("damaged restore accepted or left partial schema")
        sql("CREATE TABLE drill_fault(id integer PRIMARY KEY)","dob_restore_target")
        fault_env=dict(env,PGAPPNAME="dob_restore_interrupt")
        interrupted=subprocess.Popen([str(pg/"psql"),"-XqAt","-v","ON_ERROR_STOP=1","-d","dob_restore_target","-c",
                                       "BEGIN; INSERT INTO drill_fault VALUES(1); SELECT pg_sleep(60); COMMIT;"],
                                      env=fault_env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
        backend=""
        deadline=time.monotonic()+10
        while time.monotonic()<deadline:
            backend=sql("SELECT pid FROM pg_stat_activity WHERE application_name='dob_restore_interrupt' AND datname='dob_restore_target' AND wait_event='PgSleep'")
            if backend:break
            if interrupted.poll() is not None:raise RuntimeError("fault transaction exited before interruption")
            time.sleep(0.05)
        if not backend.isdigit():raise RuntimeError("fault transaction was not observed")
        if sql("SELECT pg_terminate_backend("+backend+")")!="t":raise RuntimeError("fault backend not terminated")
        interrupted.communicate(timeout=10)
        if interrupted.returncode==0 or sql("SELECT count(*) FROM drill_fault","dob_restore_target")!="0":
            raise RuntimeError("interrupted transaction committed")
        final=fixture("verify","dob_restore_target")
        if final["digest"]!=before["digest"]:raise RuntimeError("interruption changed durable state")
        if not verified(backup):raise RuntimeError("good backup changed")
        print(json.dumps({"drill":"disposable_database_restore","passed":True,"database":"private_socket_only",
                          "migrations":after["migrations"],"schema":after["schema"],"identity_digest":after["digest"],
                          "encrypted_identity_verified":True,"configuration_key_restored":True,"unknown_intent_preserved":True,"sequence_verified":True,
                          "amcheck_passed":True,"damaged_archive_rejected":True,"failed_restore_atomic":True,
                          "interrupted_transaction_rolled_back":True,"live_database_accessed":False}),flush=True)
    finally:
        if interrupted is not None and interrupted.poll() is None:
            interrupted.kill();interrupted.communicate(timeout=10)
        if (data/"postmaster.pid").exists():
            run([pg/"pg_ctl","-D",data,"-m","fast","-w","stop"])
        shutil.rmtree(work)

if __name__=="__main__":
    if len(sys.argv)!=2:raise SystemExit("usage: database-restore-drill.py SOURCE")
    main(sys.argv[1])
