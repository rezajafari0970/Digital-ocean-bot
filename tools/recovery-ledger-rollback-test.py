from pathlib import Path
import os,subprocess,urllib.parse,uuid,json
r=Path(__file__).resolve().parents[1]
u=urllib.parse.urlsplit(os.environ['DATABASE_URL'])._replace(path='/dob_bulk_test_20261003')
assert 'bulk_test' in u.path
dsn=urllib.parse.urlunsplit(u)
schema='rollback_gate_'+uuid.uuid4().hex
env={**os.environ,'DATABASE_URL':dsn,'PGOPTIONS':'-c search_path='+schema}
def sql(query):
 p=subprocess.run(['psql',dsn,'-XAt','-v','ON_ERROR_STOP=1','-c',query],env=env,text=True,capture_output=True)
 if p.returncode:raise RuntimeError(p.stderr)
 return p.stdout
sql('CREATE SCHEMA '+schema)
# The guard overwrites PGOPTIONS, so include search_path in server connection options.
q=urllib.parse.parse_qs(u.query);q['options']=['-c search_path='+schema]
env['DATABASE_URL']=urllib.parse.urlunsplit(u._replace(query=urllib.parse.urlencode(q,doseq=True,quote_via=urllib.parse.quote)))
facts=[]
try:
 sql((r/'migrations/000162_worker_recovery_checkpoints.up.sql').read_text())
 for stage,query,want in [
 ('empty','SELECT 1',0),
 ('pending',"INSERT INTO worker_recovery_checkpoints(kind,item_id,account_id) VALUES('lifecycle','pending','00000000-0000-0000-0000-000000000001')",1),
 ('unreadable','ALTER TABLE worker_recovery_checkpoints RENAME TO unavailable',1),
 ('settled','ALTER TABLE unavailable RENAME TO worker_recovery_checkpoints; DELETE FROM worker_recovery_checkpoints',0)]:
  sql(query)
  p=subprocess.run(['python3',str(r/'tools/recovery-ledger-rollback.py'),'--check-only'],env=env,text=True,capture_output=True)
  assert (p.returncode==0)==(want==0),(stage,p.returncode,p.stderr)
  facts.append({'stage':stage,'blocked':p.returncode!=0})
finally:sql('DROP SCHEMA '+schema+' CASCADE')
print('ROLLBACK_ENTRYPOINT_GATE_PASS')
