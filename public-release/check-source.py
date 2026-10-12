#!/usr/bin/env python3
import hashlib,json,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parents[1]
manifest=json.loads((root/'release/FILES.json').read_text())
actual=set(subprocess.check_output(['git','ls-files','-z'],cwd=root).decode().strip('\0').split('\0'))
expected=set(manifest['files'])|{'release/FILES.json'}
if actual!=expected:raise SystemExit('Export file list mismatch')
for name,want in manifest['files'].items():
 p=root/name
 if not p.is_file() or p.is_symlink() or hashlib.sha256(p.read_bytes()).hexdigest()!=want:
  raise SystemExit('Export content mismatch: '+name)
print('Exact exported source manifest verified')
