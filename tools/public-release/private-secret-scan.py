#!/usr/bin/env python3
"""Compare export with local live secrets in memory. Never output secret values."""
import argparse,base64,json,os,pathlib,runpy,subprocess,re,hashlib
from urllib.parse import urlsplit,unquote
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
def scan(candidate,bootstrap,configuration):
 values=runpy.run_path(str(bootstrap))['read_environment'](configuration/'env')
 encoded=pathlib.Path(values['MASTER_KEY_FILE']).read_bytes().strip()
 aes=AESGCM(base64.b64decode(encoded,validate=True));needles=set()
 def add(raw):
  if isinstance(raw,str):raw=raw.encode()
  if len(raw)>=8:needles.add(raw)
  try:
   d=json.loads(raw)
   def visit(x,field=""):
    sensitive=bool(re.search(r"password|secret|token|private|username|credential|authorization|api.?key|ssh.?key",field,re.I))
    if isinstance(x,dict):
     for k,v in x.items():visit(v,k)
    elif isinstance(x,list):
     for v in x:visit(v,field)
    elif isinstance(x,str) and len(x)>=8 and sensitive:needles.add(x.encode())
   visit(d)
  except (ValueError,UnicodeError):pass
  if b'PRIVATE KEY' in raw:
   for line in raw.splitlines():
    if len(line)>=20 and not line.startswith(b'-----'):needles.add(line.strip())
 def query(sql):
  q=subprocess.run(['psql','-XAt','-v','ON_ERROR_STOP=1'],input=sql,env=runpy.run_path(str(bootstrap))['database_environment'](values),capture_output=True,text=True,timeout=40)
  if q.returncode:raise RuntimeError('Private database scan failed')
  return json.loads(q.stdout)
 add(encoded);add(values['DATABASE_URL']);add(unquote(urlsplit(values['DATABASE_URL']).password))
 for p in configuration.glob('*.env'):
  for line in p.read_text().splitlines():
   if '=' in line and not line.lstrip().startswith('#') and re.search(r'KEY|TOKEN|PASSWORD|SECRET',line.split('=',1)[0]):add(line.split('=',1)[1].strip().strip("\"'"))
 for p in pathlib.Path('/root/.ssh').glob('*'):
  if p.is_file() and p.stat().st_size<50000 and b'PRIVATE KEY' in p.read_bytes():add(p.read_bytes())
 rows=query("SELECT COALESCE(json_agg(row_to_json(t)),'[]') FROM (SELECT id,account_id,proxy_id,probe_id,kind,encode(ciphertext,'hex') AS ciphertext,encode(nonce,'hex') AS nonce FROM secrets)t;")
 for row in rows:
  if row['account_id']:prefix=row['account_id']
  elif row['proxy_id']:prefix='proxy\0'+row['proxy_id']
  elif row['probe_id']:prefix='probe\0'+row['probe_id']
  else:raise RuntimeError('Unknown encrypted-secret owner')
  aad=(prefix+'\0'+row['id']+'\0'+row['kind']).encode()
  add(aes.decrypt(bytes.fromhex(row['nonce']),bytes.fromhex(row['ciphertext']),aad))
 rows=query("SELECT COALESCE(json_agg(row_to_json(t)),'[]') FROM (SELECT residential_id,id,kind,aad_scope,encode(ciphertext,'hex') AS ciphertext,encode(nonce,'hex') AS nonce FROM residential_proxy_secrets)t;")
 for row in rows:
  aad=(row['aad_scope']+'\0'+row['residential_id']+'\0'+row['id']+'\0'+row['kind']).encode()
  add(aes.decrypt(bytes.fromhex(row['nonce']),bytes.fromhex(row['ciphertext']),aad))
 for table,column in [('output_share_tokens','token'),('account_output_share_tokens','token'),('output_config_snapshots','uri')]:
  for row in query("SELECT COALESCE(json_agg(row_to_json(t)),'[]') FROM (SELECT "+column+" FROM "+table+")t;"):
   if row.get(column):add(row[column])
 findings=[]; match_meta={}
 for p in candidate.rglob('*'):
  if not p.is_file() or '.git' in p.relative_to(candidate).parts:continue
  raw=p.read_bytes()
  matched=[secret for secret in needles if secret in raw]
  if matched:
   findings.append(str(p.relative_to(candidate)))
   for secret in matched:match_meta[hashlib.sha256(secret).hexdigest()[:12]]={'length':len(secret),'ipv4':bool(re.fullmatch(rb'[0-9.]+',secret)),'private_key_line':len(secret)>60}
 if findings:print(json.dumps({'match_metadata_no_values':match_meta}))
 if findings:raise RuntimeError('Live secret match in paths: '+','.join(findings))
 print(json.dumps({'known_live_secret_scan':'PASS','private_values_compared':len(needles),'values_logged':False}))
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('candidate',type=pathlib.Path);p.add_argument('bootstrap',type=pathlib.Path);p.add_argument('--configuration',type=pathlib.Path,default=pathlib.Path('/etc/digital-ocean-bot'));a=p.parse_args()
 try:scan(a.candidate,a.bootstrap,a.configuration)
 except Exception as e:raise SystemExit(str(e) if isinstance(e,RuntimeError) else 'Private secret scan failed: '+type(e).__name__)
