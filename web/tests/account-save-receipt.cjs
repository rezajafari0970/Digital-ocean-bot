const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const source=fs.readFileSync('web/static/app.js','utf8');
const start=source.indexOf("const btn=$('#modalSubmit');btn.disabled=true;btn.textContent='Validating...';const t=performance.now();let saved=false;");
const finish="console.info('account save ms',Math.round(performance.now()-t));";
const end=source.indexOf(finish,start)+finish.length;
assert(start>=0&&end>start);
const helpers=source.slice(source.indexOf('function accountRuleApplicationView('),source.indexOf('\nfunction bootRecoveryCard('));
(async()=>{
 for(const scenario of ['success','list_unavailable','list_throws','wrong_receipt','write_failed']){
  const requests=[],messages=[],button={},modal={close(){this.closed=true}},error={};
  const rule={apply_to_existing:false,shrink_pending:false,replacement_revision:null,waiting_droplet_id:null};
  const receipt={id:scenario==='wrong_receipt'?'other':'owned',desired_server_count:10,rule_application:rule};
  const ctx=vm.createContext({id:'owned',payload:{desired_server_count:10},cache:{accounts:[]},console:{info(){}},performance:{now:()=>0},esc:String,formatDate:String,
   $:s=>s==='#modalSubmit'?button:s==='#modal'?modal:error,
   toast:(text,kind)=>messages.push({text,kind}),
   api:async(path,opt)=>{requests.push({path,opt});if(scenario==='write_failed')throw Error('fixture write failed');return {ok:true,account:receipt}},
   load:async()=>{if(scenario==='list_throws')throw Error('fixture read failed');return scenario==='list_unavailable'?false:undefined}
  });
  vm.runInContext(helpers,ctx);
  await vm.runInContext('(async()=>{'+source.slice(start,end)+'})()',ctx);
  assert.equal(requests.length,1,'Save must never be replayed');assert.equal(requests[0].opt.method,'PUT');
  const text=messages.map(x=>x.text).join('\n');
  if(scenario==='write_failed'){assert.match(text,/Save failed/);assert.equal(modal.closed,undefined)}
  else if(scenario==='wrong_receipt')assert.match(text,/status is unavailable/);
  else {assert.match(text,/OFF · Future builds only/);assert.match(text,/Desired target: 10/);assert.doesNotMatch(text,/Replacement status.*unavailable|Save failed/);assert.equal(modal.closed,true)}
 }
 console.log('PASS: exact committed Save receipt survives failed list reads; one PUT, no optimistic policy state.');
})().catch(e=>{console.error(e);process.exitCode=1});
