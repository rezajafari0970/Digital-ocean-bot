# Credentials never enter this program. Only public CSR/certificate data crosses SSH.
import base64, hashlib, json, os, pathlib, re, subprocess, sys
p=json.load(sys.stdin)
uid=re.compile(r'^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$')
assert all(uid.fullmatch(p[k]) for k in ('account','panel','generation'))
root=pathlib.Path('/opt/dob-cloudflare')/p['account']
root.mkdir(parents=True,exist_ok=True,mode=0o700)
assert not root.is_symlink()
owner=root/'owner.json'
identity={'panel':p['panel'],'account':p['account']}
if owner.exists():assert json.loads(owner.read_text())==identity
else:
 fd=os.open(owner,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,'w') as f:json.dump(identity,f)
gen=root/p['generation'];gen.mkdir(exist_ok=True,mode=0o700)
assert not gen.is_symlink()
key=gen/'privkey.pem';csr=gen/'request.pem';crt=gen/'fullchain.pem'
def run(args):return subprocess.check_output(args,stderr=subprocess.DEVNULL,timeout=20)
def atomic(path,data):
 tmp=path.with_suffix(path.suffix+'.tmp')
 fd=os.open(tmp,os.O_WRONLY|os.O_CREAT|os.O_TRUNC|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())
 os.replace(tmp,path)
if p['action']=='inspect':
 cert=(root/'current'/'fullchain.pem').read_bytes()
 certpub=run(['openssl','x509','-in',str(root/'current'/'fullchain.pem'),'-pubkey','-noout'])
 keypub=run(['openssl','pkey','-in',str(root/'current'/'privkey.pem'),'-pubout'])
 assert certpub==keypub
 print(json.dumps({'sha256':hashlib.sha256(cert).hexdigest()}))
elif p['action']=='prepare':
 assert 1<=len(p['names'])<=100
 assert all(re.fullmatch(r'\*\.[a-z0-9.-]+',n) and len(n)<194 for n in p['names'])
 if not key.exists():
  data=run(['openssl','genpkey','-algorithm','RSA','-pkeyopt','rsa_keygen_bits:2048'])
  atomic(key,data)
 if not csr.exists():
  data=run(['openssl','req','-new','-key',str(key),'-subj','/CN='+p['names'][0],'-addext','subjectAltName='+','.join('DNS:'+n for n in p['names'])])
  atomic(csr,data)
 print(json.dumps({'csr':csr.read_text()}))
elif p['action']=='install':
 assert key.is_file() and csr.is_file()
 cert=base64.b64decode(p['certificate'],validate=True);atomic(gen/'candidate.pem',cert)
 certpub=run(['openssl','x509','-in',str(gen/'candidate.pem'),'-pubkey','-noout'])
 keypub=run(['openssl','pkey','-in',str(key),'-pubout'])
 assert certpub==keypub
 run(['openssl','x509','-in',str(gen/'candidate.pem'),'-checkend','86400','-noout'])
 os.replace(gen/'candidate.pem',crt)
 link=root/'current.next'
 if link.is_symlink():link.unlink()
 os.symlink(gen.name,link);os.replace(link,root/'current')
 print(json.dumps({'installed':True,'sha256':hashlib.sha256(cert).hexdigest()}))
else:raise ValueError('invalid operation')
