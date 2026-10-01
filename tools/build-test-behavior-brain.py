#!/usr/bin/env python3
import json,glob,re,os,subprocess,collections
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT)
head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(); files=sorted(glob.glob('internal/**/*_test.go',recursive=True)); items=[]
for p in files:
 lines=open(p,encoding='utf-8').read().splitlines(); pkg='';
 for l in lines:
  m=re.match(r'\s*package\s+(\w+)',l)
  if m:pkg=m.group(1);break
 starts=[]
 for i,l in enumerate(lines,1):
  m=re.match(r'\s*func\s+(Test\w+)\s*\(',l)
  if m:starts.append((i,m.group(1)))
 for j,(start,name) in enumerate(starts):
  end=starts[j+1][0]-1 if j+1<len(starts) else len(lines); body='\n'.join(lines[start-1:end])
  calls=sorted(set(re.findall(r'\b([A-Za-z_]\w*)\s*\(',body)) - {name,'if','for','switch'})
  subtests=sorted(set(re.findall(r'\.Run\(["`]([^"`]+)',body)))
  endpoints=sorted(set(re.findall(r'/(?:api|healthz|readyz|version)[A-Za-z0-9_/{}/.:-]*',body)))
  tables=sorted(set(re.findall(r'(?i)\b(?:FROM|JOIN|INTO|UPDATE|TABLE)\s+([a-z_][a-z0-9_]*)',body)))
  assertions=[]
  for ln in body.splitlines():
   if re.search(r'\bt\.(?:Fatal|Fatalf|Error|Errorf)\s*\(',ln):assertions.append(ln.strip()[:350])
  fixtures=[]
  for token in ['httptest.NewServer','httptest.NewRecorder','sqlmock','testharness','context.Background','t.TempDir','os.Setenv','t.Setenv']:
   if token in body:fixtures.append(token)
  behavior=[]
  lname=name.lower()
  for key,label in [('error','error/failure behavior'),('retry','retry behavior'),('capacity','capacity behavior'),('pagination','pagination behavior'),('proxy','proxy/network behavior'),('output','output behavior'),('reality','Reality behavior'),('lifecycle','lifecycle behavior'),('create','creation behavior'),('delete','deletion behavior'),('update','update behavior'),('auth','authentication behavior')]:
   if key in lname:behavior.append(label)
  items.append({'test':name,'package':pkg,'file':p,'start_line':start,'end_line':end,'subtests':subtests,'candidate_calls':calls[:120],'endpoint_literals':endpoints,'sql_tables':tables,'fixture_evidence':fixtures,'assertion_evidence':assertions[:30],'name_inferred_behavior':behavior})
out={'schema_version':1,'source_commit':head,'test_file_count':len(files),'test_function_count':len(items),'methodology':'commit-pinned static test behavior evidence; name-inferred labels are hints and assertions/source remain authoritative','tests':items}
json.dump(out,open('docs/TEST_BEHAVIOR_BRAIN.json','w'),ensure_ascii=False,separators=(',',':'));open('docs/TEST_BEHAVIOR_BRAIN.json','a').write('\n')
with_assert=sum(bool(x['assertion_evidence']) for x in items);with_calls=sum(bool(x['candidate_calls']) for x in items);with_sub=sum(bool(x['subtests']) for x in items)
md=['# Test Behavior Brain','',f'Commit `{head}` — **{len(items)} test functions across {len(files)} test files**.','',f'- Tests with assertion evidence: **{with_assert}**',f'- Tests with candidate call evidence: **{with_calls}**',f'- Tests with named subtests: **{with_sub}**','', 'Each test records exact source range, subtests, candidate calls, endpoint/table literals, fixture evidence and assertion/failure evidence. Name-inferred behavior labels are navigation hints only; exact test source is authoritative.']
open('docs/TEST_BEHAVIOR_BRAIN.md','w').write('\n'.join(md)+'\n');print('test behavior',len(items),'files',len(files),'assert',with_assert,'calls',with_calls,'subtests',with_sub,'commit',head[:12])
