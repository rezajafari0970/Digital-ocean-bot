from pathlib import Path
import os,subprocess,tempfile,shutil,re,time
assert os.geteuid()!=0
r=Path.cwd();p=max(Path('/usr/lib/postgresql').glob('*/bin'),key=lambda x:int(x.parent.name))
w=Path(tempfile.mkdtemp(prefix='dob-retention-diagnostic-'));data=w/'data';sock=w/'socket';sock.mkdir(mode=0o700)
env={k:v for k,v in os.environ.items() if not k.startswith('PG') and k not in ['DATABASE_URL','BULK_TEST_DATABASE_URL']};env.update(PGHOST=str(sock),PGPORT='55439',PGUSER='retention_diag',PGDATABASE='postgres',PGCONNECT_TIMEOUT='5',PGOPTIONS='-c statement_timeout=15000')
def run(args,**kw):return subprocess.run([str(x) for x in args],env=env,check=True,timeout=60,**kw)
def sql(s):return run([p/'psql','-XqAt','-v','ON_ERROR_STOP=1','-c',s],capture_output=True,text=True).stdout
try:
 run([p/'initdb','-D',data,'-U','retention_diag','--auth-local=trust','--auth-host=reject','--no-instructions'],stdout=subprocess.DEVNULL)
 with(data/'postgresql.conf').open('a') as f:f.write("\nlisten_addresses=''\nunix_socket_directories='"+str(sock)+"'\nunix_socket_permissions=0700\nport=55439\n")
 run([p/'pg_ctl','-D',data,'-l',w/'postgres.log','-w','start'],stdout=subprocess.DEVNULL)
 assert sql("SELECT current_setting('data_directory')||'|'||current_setting('listen_addresses')").strip()==str(data)+'|'
 sql('CREATE ROLE digitaloceanbot; CREATE ROLE digitaloceanbot_readonly;')
 # Apply the same SQL migrations to a new, disposable public schema.
 all_sql='\n'.join('BEGIN;\n'+f.read_text()+'\nCOMMIT;' for f in sorted((r/'migrations').glob('*.up.sql')))
 run([p/'psql','-Xq','-v','ON_ERROR_STOP=1'],input=all_sql,text=True,stdout=subprocess.DEVNULL,stderr=subprocess.PIPE)
 sql("""INSERT INTO accounts(id,provider,name,secret_ref) VALUES('00000000-0000-4000-8000-000000000001','vultr','diagnostic','unused');
 INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES('00000000-0000-4000-8000-000000000002','00000000-0000-4000-8000-000000000001','fixture','PROVISIONING');
 INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref) VALUES('00000000-0000-4000-8000-000000000003','00000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000002','sanaei-3x-ui','http://fixture','unused');
 INSERT INTO panel_inventory_syncs(panel_id,state,finished_at) SELECT '00000000-0000-4000-8000-000000000003','COMPLETED',now()-interval '10 days' FROM generate_series(1,12500);
 INSERT INTO panel_inventory_syncs(panel_id,state,finished_at) VALUES('00000000-0000-4000-8000-000000000003','COMPLETED',now());
 INSERT INTO provider_snapshots(id,account_id,provider,data,canonical,created_at) SELECT gen_random_uuid(),'00000000-0000-4000-8000-000000000001','vultr','{}','{\"Account\":{\"Status\":\"active\"}}',now()-interval '10 days' FROM generate_series(1,2500);
 INSERT INTO provider_snapshots(id,account_id,provider,data,canonical,created_at) VALUES(gen_random_uuid(),'00000000-0000-4000-8000-000000000001','vultr','{}','{\"Account\":{\"Status\":\"active\"}}',now());""")
 print('INDEXES',sql("SELECT indexdef FROM pg_indexes WHERE tablename IN('panel_inventory_syncs','provider_snapshots')"),flush=True)
 queries=re.findall(chr(96)+'(.*?)'+chr(96),(r/'internal/app/observation_retention.go').read_text(),re.S);assert len(queries)==2
 for i,q in enumerate(queries):
  print('PLAN_BEFORE',i,sql('EXPLAIN '+q),flush=True)
  try:print('ANALYZE_BEFORE',i,sql('EXPLAIN (ANALYZE, BUFFERS) '+q),flush=True)
  except subprocess.CalledProcessError as e:print('FAILED_BEFORE',i,e.stderr,flush=True)
 sql('ANALYZE panel_inventory_syncs; ANALYZE provider_snapshots; ANALYZE account_capacity_events;')
 for i,q in enumerate(queries):
  print('PLAN_AFTER_STATISTICS',i,sql('EXPLAIN '+q),flush=True)
  try:print('ANALYZE_AFTER_STATISTICS',i,sql('EXPLAIN (ANALYZE, BUFFERS) '+q),flush=True)
  except subprocess.CalledProcessError as e:print('FAILED_AFTER',i,e.stderr,flush=True)
finally:
 if(data/'postmaster.pid').exists():run([p/'pg_ctl','-D',data,'-m','fast','-w','stop'],stdout=subprocess.DEVNULL)
 shutil.rmtree(w)
