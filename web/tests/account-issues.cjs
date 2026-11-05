const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict'),path=require('node:path');
const source=fs.readFileSync(path.join(__dirname,'../static/app.js'),'utf8');
const ctx=vm.createContext({Response,FormData,DOMException,token:'panel-session',headers:()=>({}),esc:x=>String(x).replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;').replaceAll('"','&quot;'),formatDate:x=>String(x),cache:{accounts:[]},current:'accounts',activeRoute:{},calls:[],route:()=>ctx.activeRoute,accountDash:async(id)=>ctx.calls.push('detail:'+id),load:async(page)=>ctx.calls.push('list:'+page),logout:()=>ctx.calls.push('logout')});
vm.runInContext(source.slice(source.indexOf('function accountIssuesCard('),source.indexOf('async function enterPanel(')),ctx);
(async()=>{
 const issues=[{kind:'provider',code:'LOCKED',detail:'Account locked by provider',observed_at:'2026-11-05T00:00:00Z'},{kind:'observation',code:'PROXY_ERROR',detail:'Route verification failed'},{kind:'creation',code:'CREATE_OUTCOME_UNKNOWN',detail:'3 requests need a verified provider result.'}];
 const html=ctx.accountIssuesCard({issues});
 for(const text of ['Account locked by provider','Route verification failed','3 requests','Recorded:','complete provider refresh'])assert.ok(html.includes(text));
 assert.equal(ctx.accountIssuesCard({issues:[]}), '');
 const unsafe=ctx.accountIssuesCard({issues:[{kind:'<img>',code:'<svg>',detail:'<script>secret</script>',observed_at:'<img>'}]});
 assert.doesNotMatch(unsafe,/<img>|<svg>|<script>/);
 assert.match(unsafe,/&lt;script&gt;/);
 const fail=async(status,body)=>{ctx.fetch=async()=>new Response(JSON.stringify(body),{status});try{await ctx.api('/api/v1/accounts/test/refresh');assert.fail('error response accepted')}catch(e){return e}};
 let error=await fail(502,{error:'provider_discovery_failed',state:'LOCKED',detail:'Provider locked this account (HTTP 422).'});
 assert.match(error.message,/Provider locked this account/);assert.equal(error.state,'LOCKED');
 error=await fail(409,{ready:false,status:'PREFLIGHT_FAILED',checks:{network:false,provider_identity:false,enabled:true},provider_error:'Credential rejected',create_error:'Trial firewall restriction'});
 for(const detail of ['Account preflight failed','Credential rejected','Trial firewall restriction','network, provider identity'])assert.ok(error.message.includes(detail));
 assert.equal(error.checks.network,false);assert.doesNotMatch(error.message,/\{"ready"/);
 error=await fail(401,{error:'provider_credential_invalid',detail:'Provider rejected the credential'});
 assert.match(error.message,/Provider rejected/);assert.deepEqual(ctx.calls,[]);
 await fail(401,{error:'unauthorized'});assert.deepEqual(ctx.calls,['logout']);ctx.calls=[];
 ctx.activeRoute={};await ctx.refreshAccountView('one');assert.deepEqual(ctx.calls,['list:accounts']);ctx.calls=[];
 ctx.activeRoute={accountId:'one'};await ctx.refreshAccountView('one');assert.deepEqual(ctx.calls,['detail:one']);ctx.calls=[];
 ctx.activeRoute={accountId:'two'};await ctx.refreshAccountView('one');assert.deepEqual(ctx.calls,[]);
 ctx.activeRoute={};ctx.current='configs';await ctx.refreshAccountView('one');assert.deepEqual(ctx.calls,[]);
 // Execute the real failed Refresh handler: persisted failures must be read
 // back from both list and detail, not merely disappear in a toast.
 ctx.window=ctx;ctx.document={querySelector:()=>null};ctx.toast=()=>{};ctx.api=async()=>{throw Error('persisted failure')};ctx.current='accounts';ctx.activeRoute={};
 vm.runInContext(source.slice(source.indexOf('window.accountRefresh='),source.indexOf('function bindUniqueAccountChoices')),ctx);
 await ctx.accountRefresh('one');assert.deepEqual(ctx.calls,['list:accounts']);ctx.calls=[];
 ctx.activeRoute={accountId:'one'};await ctx.accountPreflight('one');assert.deepEqual(ctx.calls,['detail:one']);
 console.log('PASS account issue visibility, escaping, detailed HTTP failures, panel authentication and post-failure readback/navigation fencing');
})().catch(e=>{console.error(e);process.exitCode=1});
