from pathlib import Path
import subprocess,json,hashlib,runpy,urllib.parse
identity=json.loads(Path('/etc/dob-production/instance.json').read_text())
assert hashlib.sha256(Path('/etc/dob-production/master.key').read_bytes()).hexdigest()==identity['master_key_sha256'],'master_key_identity_mismatch'
args=['/usr/sbin/runuser','-u','postgres','--','/usr/lib/postgresql/18/bin/psql','-h','/var/run/postgresql','-p',identity['port'],'-d',identity['database'],'-XAt','-v','ON_ERROR_STOP=1','-c',"SELECT json_build_object('system_identifier',(SELECT system_identifier::text FROM pg_control_system()),'database',current_database(),'database_oid',(SELECT oid::text FROM pg_database WHERE datname=current_database()),'port',current_setting('port'),'checksums',current_setting('data_checksums'),'fsync',current_setting('fsync'),'full_page_writes',current_setting('full_page_writes'),'synchronous_commit',current_setting('synchronous_commit'),'ignore_checksum_failure',current_setting('ignore_checksum_failure'),'zero_damaged_pages',current_setting('zero_damaged_pages'))"]
s=json.loads(subprocess.check_output(args,text=True,timeout=30))
assert all(s[k]==identity[k] for k in ['system_identifier','database','database_oid','port']),'wrong_database_instance'
assert all(s[k]=='on' for k in ['checksums','fsync','full_page_writes']) and s['synchronous_commit']!='off' and all(s[k]=='off' for k in ['ignore_checksum_failure','zero_damaged_pages']),'unsafe_database_settings'
print('Production database and key identity verified')

# Verify the application DSN independently of the privileged cluster probe.
helpers=runpy.run_path('/opt/dob-production/support/bootstrap.py')
application=helpers['read_environment'](Path('/etc/dob-production/env'))
u=urllib.parse.urlsplit(application['DATABASE_URL'])
assert u.hostname in ('127.0.0.1','localhost') and str(u.port)==identity['port'] and urllib.parse.unquote(u.path.lstrip('/'))==identity['database'],'application_database_target_mismatch'
env=helpers['database_environment'](application)
r=subprocess.run(['psql','-XqAt','-v','ON_ERROR_STOP=1','-c',"SELECT json_build_object('database',current_database(),'database_oid',(SELECT oid::text FROM pg_database WHERE datname=current_database()),'port',current_setting('port'))"],env=env,capture_output=True,text=True,timeout=30)
assert r.returncode==0,'application_database_unavailable'
actual=json.loads(r.stdout)
assert all(actual[k]==identity[k] for k in ['database','database_oid','port']),'application_database_identity_mismatch'
pending=Path('/etc/dob-production/deployment.pending')
if pending.exists():
 record=json.loads(pending.read_text())
 assert record.get('phase') in ('verifying','rolling_back'),'deployment_incomplete'
 expected=record['to_revision'] if record['phase']=='verifying' else record['from_revision']
 assert json.loads(Path('/opt/dob-production/build-manifest.json').read_text())['commit']==expected,'deployment_runtime_mismatch'
