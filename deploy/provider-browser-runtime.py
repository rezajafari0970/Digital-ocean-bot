#!/usr/bin/env python3
"""Install the checksum-pinned remote browser before publishing application code.
The runtime is shared, immutable and contains no profile or credentials.
"""
import hashlib, io, os, pathlib, shutil, subprocess, tempfile, urllib.request, zipfile
VERSION='156.0.8078.47'
SHA256='0d5acc62c3e213f8c87d3c298d66ffcf9a236d12ea4b9f85318bf85dbc279daa'
ROOT=pathlib.Path('/usr/local/lib/datacenter-bot/browser')
def install():
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
if __name__=='__main__':install()
