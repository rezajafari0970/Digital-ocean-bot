#!/usr/bin/env python3
import os,re,json,glob
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
ups=sorted(glob.glob('migrations/*.up.sql'))
tables={}; migration_ops=[]
def table(n):
 n=n.strip('"').lower(); return tables.setdefault(n,{'table':n,'created_by':None,'columns':{},'altered_by':[],'indexes':[],'references':set()})
for p in ups:
 txt=open(p,encoding='utf-8').read(); name=os.path.basename(p)
 ops=[]
 for m in re.finditer(r'CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?["`]?([A-Za-z_][\w]*)["`]?\s*\((.*?)\)\s*;',txt,re.I|re.S):
  t=table(m.group(1)); t['created_by']=t['created_by'] or name; body=m.group(2)
  for raw in re.split(r',\s*\n',body):
   x=raw.strip(); cm=re.match(r'["`]?([A-Za-z_][\w]*)["`]?\s+([^,]+)',x,re.S)
   if cm and cm.group(1).upper() not in {'PRIMARY','FOREIGN','UNIQUE','CHECK','CONSTRAINT'}: t['columns'].setdefault(cm.group(1),{'definition':' '.join(cm.group(2).split()),'added_by':name})
  ops.append({'op':'create_table','table':t['table']})
 for m in re.finditer(r'ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?["`]?([A-Za-z_][\w]*)["`]?(.*?);',txt,re.I|re.S):
  t=table(m.group(1)); clause=' '.join(m.group(2).split()); t['altered_by'].append({'migration':name,'clause':clause[:500]}); ops.append({'op':'alter_table','table':t['table'],'clause':clause[:500]})
  for a in re.finditer(r'ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?["`]?([A-Za-z_][\w]*)["`]?\s+(.+?)(?=\s+ADD\s+COLUMN|\s+DROP\s+COLUMN|$)',m.group(2),re.I|re.S): t['columns'].setdefault(a.group(1),{'definition':' '.join(a.group(2).split())[:500],'added_by':name})
 for m in re.finditer(r'CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?["`]?([A-Za-z_][\w]*)["`]?\s+ON\s+["`]?([A-Za-z_][\w]*)',txt,re.I): table(m.group(2))['indexes'].append({'name':m.group(1),'migration':name})
 for m in re.finditer(r'REFERENCES\s+["`]?([A-Za-z_][\w]*)',txt,re.I):
  # migration-level refs retained below; exact source table may be implicit in CREATE/ALTER body
  pass
 migration_ops.append({'migration':name,'operations':ops})
# Static SQL usage: map table names to Go files mentioning them in SQL-ish text.
go=glob.glob('cmd/**/*.go',recursive=True)+glob.glob('internal/**/*.go',recursive=True)
for t in tables.values():
 uses=[]; pat=re.compile(r'\b'+re.escape(t['table'])+r'\b',re.I)
 for p in go:
  try:s=open(p,encoding='utf-8').read()
  except:continue
  if pat.search(s): uses.append(p)
 t['code_usage']=sorted(uses)[:120]; t['references']=sorted(t['references'])
idx={'schema_version':1,'migration_count':len(ups),'table_count':len(tables),'tables':sorted(tables.values(),key=lambda x:x['table']),'migrations':migration_ops}
json.dump(idx,open('docs/SCHEMA_INDEX.json','w',encoding='utf-8'),ensure_ascii=False,indent=2); open('docs/SCHEMA_INDEX.json','a').write('\n')
out=['# Database / Schema Navigation Index','',f'Statically reconstructed from **{len(ups)} up migrations**: **{len(tables)} table names** observed. This is a source index, not proof of live DB migration level.','', 'For exact production state, query the live migration table/schema before mutation. Full machine-readable index: `docs/SCHEMA_INDEX.json`.','']
for t in sorted(tables.values(),key=lambda x:x['table']):
 out += [f'## `{t["table"]}`',f'- Created/first observed: `{t["created_by"] or "pre-existing/alter-only"}`',f'- Known columns from static migration parsing: {len(t["columns"])}; alter operations: {len(t["altered_by"])}; indexes: {len(t["indexes"])}']
 if t['columns']: out.append('- Columns: '+', '.join(f'`{k}`' for k in list(t['columns'])[:40]))
 if t['code_usage']: out.append('- Code usage: '+', '.join(f'`{p}`' for p in t['code_usage'][:15]))
 out.append('')
open('docs/SCHEMA_NAVIGATION.md','w',encoding='utf-8').write('\n'.join(out))
print('migrations',len(ups),'tables',len(tables),'with-code-usage',sum(bool(t['code_usage']) for t in tables.values()))
