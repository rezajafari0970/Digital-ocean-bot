const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
const source=fs.readFileSync(path.join(__dirname,'../static/app.js'),'utf8');
function functionSource(name,next){return source.slice(source.indexOf('function '+name+'('),source.indexOf(next,source.indexOf('function '+name+'(')))}
const context={};
vm.runInNewContext(functionSource('pageReadFailureMessage','function showPageReadFailure'),context);
vm.runInNewContext(functionSource('healthLabel','let healthBusy='),context);
test('health cannot show ready for a failed or malformed database report',()=>{
 assert.equal(context.healthLabel({status:'not_ready',checks:[{name:'database',ok:false,message:'database_recovery_required'}]},false),'database needs attention');
 assert.equal(context.healthLabel({status:'not_ready',checks:[]},true),'not ready');
 assert.equal(context.healthLabel({},true),'not ready');
 assert.equal(context.healthLabel({status:'ready',checks:[{name:'database',ok:true}]},true),'ready');
});
test('recovery notice explains cached data and does not expose database internals',()=>{
 const text=context.pageReadFailureMessage({code:'database_recovery_required',message:'private SQL'},true);
 assert.match(text,/verified repair/);assert.match(text,/previously loaded/);assert.doesNotMatch(text,/private SQL/);
 assert.match(context.pageReadFailureMessage(Error('offline')),/Retrying automatically/);
});
test('API preserves machine-readable recovery code without retrying a mutation',async()=>{
 const api=source.slice(source.indexOf('async function api('),source.indexOf('async function enterPanel('));
 let calls=0;
 const ctx={FormData,DOMException,token:'fixture',headers:()=>({}),fetch:async()=>{calls++;return {ok:false,status:503,text:async()=>JSON.stringify({error:'database_recovery_required'})}}};
 vm.runInNewContext(api,ctx);
 await assert.rejects(ctx.api('/api/v1/accounts',{method:'POST'}),e=>e.code==='database_recovery_required'&&e.status===503);
 assert.equal(calls,1);
});
