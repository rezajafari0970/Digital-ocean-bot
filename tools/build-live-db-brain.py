#!/usr/bin/env python3
import os,subprocess,json,hashlib
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
# Source env without printing it; only psql consumes DATABASE_URL.
cmd=r'''set -a; . /etc/digital-ocean-bot/env; set +a; psql "$DATABASE_URL" -X -A -t -F $'\t' -c "SELECT table_name,column_name,data_type,is_nullable,COALESCE(column_default,'') FROM information_schema.columns WHERE table_schema='public' ORDER BY table_name,ordinal_position"'''
p=subprocess.run(['bash','-lc',cmd],text=True,capture_output=True,timeout=20)
if p.returncode: raise SystemExit(p.stderr.strip())
tables={}
for line in p.stdout.splitlines():
 a=line.split('\t');
 if len(a)>=5: tables.setdefault(a[0],[]).append({'name':a[1],'type':a[2],'nullable':a[3]=='YES','default':a[4]})
cmd2=r'''set -a; . /etc/digital-ocean-bot/env; set +a; psql "$DATABASE_URL" -X -A -t -F $'\t' -c "SELECT version,COALESCE(checksum,''),applied_at::text FROM schema_migrations ORDER BY version"'''
p2=subprocess.run(['bash','-lc',cmd2],text=True,capture_output=True,timeout=20)
migs=[]
if p2.returncode==0:
 for l in p2.stdout.splitlines():
  a=l.split('\t');
  if len(a)>=3: migs.append({'version':a[0],'checksum':a[1],'applied_at':a[2]})
# indexes/constraints, no row/user data.
cmd3=r'''set -a; . /etc/digital-ocean-bot/env; set +a; psql "$DATABASE_URL" -X -A -t -F $'\t' -c "SELECT tablename,indexname,indexdef FROM pg_indexes WHERE schemaname='public' ORDER BY tablename,indexname"'''
p3=subprocess.run(['bash','-lc',cmd3],text=True,capture_output=True,timeout=20); indexes=[]
for l in p3.stdout.splitlines():
 a=l.split('\t',2)
 if len(a)==3:indexes.append({'table':a[0],'name':a[1],'definition':a[2]})
out={'schema_version':1,'scope':'production public schema metadata only; no application rows/secrets','table_count':len(tables),'tables':[{'name':k,'columns':v} for k,v in sorted(tables.items())],'migration_count':len(migs),'migrations':migs,'indexes':indexes}
json.dump(out,open('docs/LIVE_DB_BRAIN.json','w'),indent=2); open('docs/LIVE_DB_BRAIN.json','a').write('\n')
print('live-db tables',len(tables),'migrations',len(migs),'indexes',len(indexes),'latest',migs[-1]['version'] if migs else 'unknown')
