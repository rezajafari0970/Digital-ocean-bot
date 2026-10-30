#!/usr/bin/env python3
"""Run the actual browser in disposable systemd namespaces on an isolated CI host."""
import os,pathlib,shutil,subprocess,tempfile,sys
ROOT=pathlib.Path(__file__).resolve().parents[2]
def run(*args,**kw):return subprocess.run(args,check=True,**kw)
def sudo(*args):return run('sudo',*args)
def main():
    if os.environ.get('CI')!='true':raise SystemExit('This native sandbox gate requires an isolated CI host.')
    if pathlib.Path('/etc/digital-ocean-bot/env').exists() or pathlib.Path('/etc/dob-production/env').exists():raise SystemExit('Refusing to install test fixtures on an application host.')
    with tempfile.TemporaryDirectory(prefix='dob-browser-test-') as temp:
        temp=pathlib.Path(temp)
        run('go','test','-c','-race','-o',str(temp/'worker'),'./internal/providerbrowser',cwd=ROOT)
        sudo('install','-d','-m','0755','/opt/digital-ocean-bot/bin','/usr/local/lib/datacenter-bot/browser')
        sudo('install','-m','0755',str(temp/'worker'),'/opt/digital-ocean-bot/bin/provider-browser-runner')
        source=pathlib.Path(os.environ['DOB_TEST_CHROMIUM']).parent
        sudo('cp','-a',str(source),'/usr/local/lib/datacenter-bot/browser/')
        wrapper=temp/'chrome';wrapper.write_text('#!/bin/sh\nexec /usr/local/lib/datacenter-bot/browser/chrome-headless-shell-linux64/chrome-headless-shell --ignore-certificate-errors-spki-list="$DOB_TEST_BROWSER_SPKI" "$@"\n')
        sudo('install','-m','0755',str(wrapper),'/usr/local/lib/datacenter-bot/browser-fixture-chrome')
        service=(ROOT/'deploy/datacenter-provider-browser@.service').read_text().replace('ExecStart=/opt/digital-ocean-bot/bin/provider-browser-runner','ExecStart=/opt/digital-ocean-bot/bin/provider-browser-runner -test.run=^TestProviderBrowserWorkerProcess$').replace('StandardError=null','StandardError=journal')
        service+='\nEnvironment=DOB_TEST_BROWSER_WORKER=1\nPassEnvironment=DOB_TEST_BROWSER_SPKI\n'
        socket=(ROOT/'deploy/datacenter-provider-browser.socket').read_text().replace('SocketUser=root','SocketUser='+os.environ['USER']).replace('SocketGroup=digitaloceanbot','SocketGroup='+os.environ['USER'])
        for name,text in [('datacenter-provider-browser@.service',service),('datacenter-provider-browser.socket',socket)]:
            p=temp/name;p.write_text(text);sudo('install','-m','0644',str(p),'/etc/systemd/system/'+name)
        for path in ['/etc/dob-browser-host-fixture','/var/lib/dob-browser-host-fixture','/run/dob-browser-host-fixture']:
            sudo('touch',path)
        sudo('systemctl','daemon-reload');sudo('systemctl','start','datacenter-provider-browser.socket')
        try:
            env=os.environ.copy();env['DOB_RUN_PROVIDER_BROWSER_SANDBOX']='1'
            run('go','test','-race','-v','-count=1','-timeout','3m','-run','^TestProviderBrowserNativeSandbox$','./internal/providerbrowser',cwd=ROOT,env=env)
        except BaseException:
            subprocess.run(['sudo','journalctl','--no-pager','-n','100','--unit','datacenter-provider-browser@*'])
            raise
        finally:
            subprocess.run(['sudo','systemctl','stop','datacenter-provider-browser.socket','datacenter-provider-browser@*'])
if __name__=='__main__':main()
