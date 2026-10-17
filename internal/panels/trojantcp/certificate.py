"""Owned, bounded ACME IP certificate maintenance. Never stops any existing service."""
import base64, fcntl, hashlib, ipaddress, json, os, pathlib, shutil, socket, subprocess, sys, tempfile, urllib.request
ROOT = pathlib.Path('/opt/dob-trojan')
ACME_VERSION = '3.1.6'
ACME_SHA256 = 'c7d68b021cfd6380ea83a82962abde5b484779fee0b97d38681dfa1396bbc8d7'
DIRECTORY = 'https://acme-v02.api.letsencrypt.org/directory'
def run(args, timeout=30, ok=(0,)):
    p = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout, text=True)
    if p.returncode not in ok:
        raise RuntimeError('command_failed_' + pathlib.Path(args[0]).name)
    return p
def atomic(path, data, mode=0o600):
    if path.is_symlink(): raise RuntimeError('unsafe_symlink')
    fd, name = tempfile.mkstemp(prefix='.pending-', dir=str(path.parent))
    try:
        os.fchmod(fd, mode)
        with os.fdopen(fd, 'wb') as f: f.write(data); f.flush(); os.fsync(f.fileno())
        os.replace(name, path)
    finally:
        if os.path.exists(name): os.unlink(name)
def valid_cert(host, seconds):
    cert = ROOT / 'tls/fullchain.pem'
    key = ROOT / 'tls/privkey.pem'
    if not cert.is_file() or not key.is_file(): return False
    return (run(['openssl','x509','-in',str(cert),'-checkend',str(seconds),'-noout'],ok=(0,1)).returncode == 0
        and run(['openssl','x509','-in',str(cert),'-checkip',host,'-noout'],ok=(0,1)).returncode == 0)
def main(plan):
    os.umask(0o077)
    if os.geteuid() != 0: raise RuntimeError('root_required')
    ip = ipaddress.ip_address(plan['host'])
    if not ip.is_global or ip.version != 4: raise RuntimeError('public_ipv4_required')
    port, panel_port = int(plan['port']), int(plan['panel_port'])
    if not 1024 <= port <= 65535 or port in (panel_port, 18080): raise RuntimeError('reserved_port')
    if ROOT.is_symlink() or not ROOT.is_dir(): raise RuntimeError('unsafe_owner_directory')
    if json.loads((ROOT/'owner.json').read_text()) != {'panel':plan['panel'],'host':str(ip)}:
        raise RuntimeError('owner_mismatch')
    with open('/run/lock/dob-trojan-certificate.lock','w') as lock:
        try: fcntl.flock(lock, fcntl.LOCK_EX|fcntl.LOCK_NB)
        except BlockingIOError: raise RuntimeError('certificate_busy')
        for name in ('acme','tls'):
            path=ROOT/name
            if path.is_symlink(): raise RuntimeError('unsafe_symlink')
            path.mkdir(mode=0o700, exist_ok=True)
            os.chmod(path,0o700)
        acme=ROOT/'acme/acme.sh'
        if not acme.is_file() or hashlib.sha256(acme.read_bytes()).hexdigest()!=ACME_SHA256:
            request=urllib.request.Request('https://raw.githubusercontent.com/acmesh-official/acme.sh/'+ACME_VERSION+'/acme.sh',headers={'User-Agent':'dob-trojan-certificate'})
            with urllib.request.urlopen(request,timeout=20) as response: source=response.read(2000000)
            if hashlib.sha256(source).hexdigest()!=ACME_SHA256: raise RuntimeError('acme_digest_mismatch')
            atomic(acme,source,0o700)
        if shutil.which('firewall-cmd') and run(['firewall-cmd','--state'],ok=(0,1,252)).returncode==0:
            raise RuntimeError('unsupported_active_firewall')
        if not valid_cert(str(ip),172800):
            # Refuse port 80 conflicts. Never stop nginx, a relay, Reality, or a panel.
            with socket.socket() as probe:
                try: probe.bind(('0.0.0.0',80))
                except OSError: raise RuntimeError('acme_http_port_occupied')
            if shutil.which('ufw') and 'Status: active' in run(['ufw','status']).stdout:
                run(['ufw','allow','80/tcp','comment','dob-trojan-acme'])
            if not shutil.which('socat'):
                run(['apt-get','update','-qq'],timeout=90)
                run(['apt-get','install','-y','-qq','socat'],timeout=90)
            command=['/bin/sh',str(acme),'--home',str(ROOT/'acme'),'--config-home',str(ROOT/'acme'),
                '--issue','--standalone','--listen-v4','--server',DIRECTORY,
                '--certificate-profile','shortlived','--domain',str(ip),'--keylength','ec-256','--days','3',
                '--fullchain-file',str(ROOT/'tls/fullchain.pem'),'--key-file',str(ROOT/'tls/privkey.pem')]
            result=run(command,timeout=210,ok=(0,1,2))
            # Detailed ACME diagnostics stay private on the node.
            atomic(ROOT/'acme-last.log',(result.stdout+'\n'+result.stderr)[-32768:].encode())
            if not valid_cert(str(ip),86400): raise RuntimeError('acme_certificate_not_ready')
        if not valid_cert(str(ip),86400): raise RuntimeError('certificate_not_valid')
        for p in (ROOT/'tls/fullchain.pem',ROOT/'tls/privkey.pem'): os.chmod(p,0o600)
        atomic(ROOT/'plan.json',json.dumps(plan,separators=(',',':')).encode())
        service='''[Unit]
Description=DOB Trojan IP certificate renewal
After=network-online.target
[Service]
Type=oneshot
ExecStart=/usr/bin/python3 /opt/dob-trojan/renew.py
TimeoutStartSec=420
UMask=0077
NoNewPrivileges=true
'''
        timer='''[Unit]
Description=Renew DOB Trojan IP certificate
[Timer]
OnBootSec=10min
OnUnitActiveSec=1h
RandomizedDelaySec=5min
[Install]
WantedBy=timers.target
'''
        changed=False
        for name,value in [('dob-trojan-certificate.service',service),('dob-trojan-certificate.timer',timer)]:
            p=pathlib.Path('/etc/systemd/system')/name
            if p.is_symlink(): raise RuntimeError('unsafe_unit_symlink')
            if p.exists() and p.read_text()!=value and 'DOB Trojan IP certificate' not in p.read_text():
                raise RuntimeError('unit_owner_mismatch')
            if not p.exists() or p.read_text()!=value:
                atomic(p,value.encode(),0o644);changed=True
        if changed: run(['systemctl','daemon-reload'])
        run(['systemctl','enable','--now','dob-trojan-certificate.timer'])
        # Only the public certificate chain leaves the node; private keys never do.
        print(json.dumps({'certificate':(ROOT/'tls/fullchain.pem').read_text(),'renewal_timer':True}))
if __name__=='__main__':
    try: main(json.loads(base64.b64decode(sys.argv[1])) if len(sys.argv)>1 else json.loads((ROOT/'plan.json').read_text()))
    except Exception as e:
        code=str(e) if isinstance(e,RuntimeError) else type(e).__name__
        print(json.dumps({'error':code}));sys.exit(1)
