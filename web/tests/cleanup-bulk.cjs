const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const source=fs.readFileSync(require('node:path').join(__dirname,'../static/app.js'),'utf8');
const ctx={window:{},cache:{accounts:[]},accountCleanupBulkBusy:false,accountCleanupBulkResult:'',accountDeletionBusy:new Set(),refreshAccountsInPlace:async()=>true,accountIsDeleting:r=>!!r.deletion,confirm:()=>true,toast:()=>{},document:{querySelector:()=>null},calls:[]};
ctx.api=async(path,o)=>{ctx.calls.push({path,body:JSON.parse(o.body)});return{state:'PURGE_PENDING'}};
vm.createContext(ctx);vm.runInContext(source.slice(source.indexOf('window.purgeAllCleanupAccounts='),source.indexOf('window.editProxy=async')),ctx);
const cleanup=(id,allowed=true)=>({id,name:id,deletion:{local_purge_allowed:allowed,provider_state:'ACTIVE',remaining_servers:2,requested_at:'2026-11-06T00:00:00Z',resource_scope:'a'.repeat(64)}});
(async()=>{
 ctx.cache.accounts=[{id:'active'},cleanup('one'),cleanup('two')];ctx.confirm=()=>false;
 await ctx.window.purgeAllCleanupAccounts();assert.equal(ctx.calls.length,0);
 ctx.confirm=()=>{ctx.cache.accounts.push(cleanup('later'));return true};
 let held;ctx.api=async(path,o)=>{
  ctx.calls.push({path,body:JSON.parse(o.body)});
  if(path.includes('one')){await new Promise(resolve=>held=resolve);return{state:'PURGE_PENDING'}}
  throw Error('Resource scope changed');
 };
 const running=ctx.window.purgeAllCleanupAccounts();
 await new Promise(r=>setImmediate(r));
 await ctx.window.purgeAllCleanupAccounts();assert.equal(ctx.calls.length,1);
 held();await running;
 assert.equal(ctx.calls.length,2);assert.equal(ctx.calls[0].body.expected_resource_scope,'a'.repeat(64));
 assert.ok(ctx.calls.every(c=>!c.path.includes('active')&&!c.path.includes('later')));
 assert.match(ctx.accountCleanupBulkResult,/Queued: 1 · Needs review: 1/);
 assert.match(ctx.accountCleanupBulkResult,/Resource scope changed/);
 ctx.calls=[];ctx.confirm=()=>true;ctx.cache.accounts=[cleanup('wait',false)];
 await ctx.window.purgeAllCleanupAccounts();assert.equal(ctx.calls.length,0);assert.match(ctx.accountCleanupBulkResult,/settling/);
 console.log('PASS bulk cleanup consent, frozen scope, duplicate clicks, partial errors and safety wait');
})().catch(e=>{console.error(e);process.exitCode=1});
