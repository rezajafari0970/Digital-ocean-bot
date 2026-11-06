#!/usr/bin/env python3
"""Install the checksum-pinned remote browser before publishing application code.
The runtime is shared, immutable and contains no profile or credentials.
"""
import hashlib, io, os, pathlib, shutil, subprocess, tempfile, urllib.request, zipfile
VERSION='156.0.8078.47'
SHA256='0d5acc62c3e213f8c87d3c298d66ffcf9a236d12ea4b9f85318bf85dbc279daa'
ROOT=pathlib.Path('/usr/local/lib/datacenter-bot/browser')
def install_headless():
    if os.uname().machine!='x86_64':
        print('Isolated provider browser unavailable on this architecture; no direct fallback.')
        return
    manifest=ROOT/'runtime.sha256'
    executable=ROOT/'chrome-headless-shell-linux64/chrome-headless-shell'
    if manifest.is_file() and executable.is_file() and manifest.read_text().strip()==hashlib.sha256(executable.read_bytes()).hexdigest():
        return
    payload=urllib.request.urlopen(f'https://storage.googleapis.com/chrome-for-testing-public/{VERSION}/linux64/chrome-headless-shell-linux64.zip',timeout=90).read()
    if hashlib.sha256(payload).hexdigest()!=SHA256:raise RuntimeError('browser archive checksum mismatch')
    ROOT.parent.mkdir(parents=True,exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='browser-stage-',dir=ROOT.parent) as folder:
        stage=pathlib.Path(folder)/'browser';stage.mkdir()
        with zipfile.ZipFile(io.BytesIO(payload)) as archive:
            for entry in archive.infolist():
                p=pathlib.PurePosixPath(entry.filename)
                if p.is_absolute() or '..' in p.parts or entry.external_attr>>16 & 0o170000 == 0o120000:raise RuntimeError('invalid browser archive entry')
            archive.extractall(stage)
        binary=stage/'chrome-headless-shell-linux64/chrome-headless-shell';binary.chmod(0o755)
        for p in stage.rglob('*'):
            if p.is_dir():p.chmod(0o755)
            elif p!=binary:p.chmod(0o644)
        stage.chmod(0o755)
        (stage/'runtime.sha256').write_text(hashlib.sha256(binary.read_bytes()).hexdigest()+'\n')
        # Verify shared-library availability before publication. No network used.
        subprocess.run([str(binary),'--version'],check=True,stdout=subprocess.DEVNULL)
        if ROOT.exists():raise RuntimeError('existing browser runtime failed integrity; preserve it for inspection')
        os.rename(stage,ROOT)
FULL_SHA256='e6c94490f0b6b337225aeb2d2a5034858516b7017356980ba94f5176d8774a50'
FULL_ROOT=ROOT.parent/'chrome'/VERSION
def install_full():
    if os.uname().machine!='x86_64':return
    manifest=FULL_ROOT/'runtime.json'
    if FULL_ROOT.exists():
        data=__import__('json').loads(manifest.read_text())
        if data.get('archive_sha256')!=FULL_SHA256:raise RuntimeError('full browser runtime identity mismatch')
        for name,want in data['files'].items():
            if hashlib.sha256((FULL_ROOT/name).read_bytes()).hexdigest()!=want:raise RuntimeError('full browser runtime integrity failure')
    else:
        FULL_ROOT.parent.mkdir(parents=True,exist_ok=True)
        with tempfile.TemporaryDirectory(prefix='chrome-stage-',dir=FULL_ROOT.parent) as folder:
            folder=pathlib.Path(folder);archive_path=folder/'chrome.zip';digest=hashlib.sha256()
            with urllib.request.urlopen(f'https://storage.googleapis.com/chrome-for-testing-public/{VERSION}/linux64/chrome-linux64.zip',timeout=90) as response,archive_path.open('wb') as out:
                while chunk:=response.read(1<<20):digest.update(chunk);out.write(chunk)
            if digest.hexdigest()!=FULL_SHA256:raise RuntimeError('full browser archive checksum mismatch')
            stage=folder/'runtime';stage.mkdir()
            with zipfile.ZipFile(archive_path) as archive:
                for entry in archive.infolist():
                    p=pathlib.PurePosixPath(entry.filename)
                    if p.is_absolute() or '..' in p.parts or entry.external_attr>>16 & 0o170000 == 0o120000:raise RuntimeError('invalid full browser archive entry')
                archive.extractall(stage)
            files={}
            for p in stage.rglob('*'):
                if p.is_dir():p.chmod(0o755)
                elif p.is_file():
                    p.chmod(0o755 if p.name in ['chrome','chrome_crashpad_handler','chrome_sandbox'] else 0o644)
                    files[str(p.relative_to(stage))]=hashlib.sha256(p.read_bytes()).hexdigest()
            subprocess.run([str(stage/'chrome-linux64/chrome'),'--version'],check=True,stdout=subprocess.DEVNULL)
            (stage/'runtime.json').write_text(__import__('json').dumps({'version':VERSION,'archive_sha256':FULL_SHA256,'files':files},sort_keys=True)+'\n')
            stage.chmod(0o755);os.rename(stage,FULL_ROOT)
    if not shutil.which('xvfb-run') or not shutil.which('xauth'):raise RuntimeError('isolated browser display dependencies unavailable')
    wrapper=ROOT.parent/'provider-browser-chrome'
    content='#!/bin/sh\nexec /usr/bin/xvfb-run -a -s "-screen 0 2048x4096x24 -nolisten tcp" '+str(FULL_ROOT/'chrome-linux64/chrome')+' "$@"\n'
    temporary=wrapper.with_suffix('.new');temporary.write_text(content);temporary.chmod(0o755);os.replace(temporary,wrapper)
def install():
    install_headless()
    install_full()
if __name__=='__main__':install()
