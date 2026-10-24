/* Request-scoped progress for slow read-only provider validation. */
(function(root){
 'use strict';
 const type='application/x-ndjson';
 async function response(r,onProgress){
  if(!r.ok||!r.headers.get('content-type')?.includes(type))return r;
  const reader=r.body?.getReader();if(!reader)throw Error('Provider validation response is unavailable. Validate again.');
  const decoder=new TextDecoder();let pending='',received=0;
  try{
   for(;;){
    const {value,done}=await reader.read();
    received+=value?.byteLength||0;
    if(received>8*1024*1024)throw Error('Provider validation response is too large.');
    pending+=decoder.decode(value,{stream:!done});
    let end;
    while((end=pending.indexOf('\n'))>=0){
     const line=pending.slice(0,end);pending=pending.slice(end+1);if(!line.trim())continue;
     let event;try{event=JSON.parse(line)}catch{throw Error('Provider validation response was interrupted. Validate again.')}
     if(event.type==='progress'){
      if(typeof event.stage!=='string'||!Number.isFinite(event.elapsed_seconds))throw Error('Invalid provider validation progress.');
      onProgress(event);continue;
     }
     if(event.type==='result'&&Number.isInteger(event.status)&&event.status>=200&&event.status<=599&&event.body&&typeof event.body==='object'){
      return new Response(JSON.stringify(event.body),{status:event.status,headers:{'Content-Type':'application/json'}});
     }
     throw Error('Invalid provider validation response.');
    }
    if(done)throw Error('Provider validation connection ended before completion. Your entries are preserved; validate again.');
   }
  }finally{await reader.cancel().catch(()=>{});reader.releaseLock()}
 }
 function message(event){
  const stages={connection:'Checking the selected connection',discovery:'Verifying selected regions and networks',credential:'Checking provider credentials',account:'Verifying the provider account',capacity:'Checking provider capacity',catalog:'Loading available plans, Ubuntu images and prices'};
  return (stages[event.stage]||'Validating provider options')+' — '+Math.max(0,Math.floor(event.elapsed_seconds))+'s. Save becomes available after validation and network review.';
 }
 const api={type,response,message};
 if(typeof module!=='undefined'&&module.exports)module.exports=api;
 else root.PreviewStream=api;
})(globalThis);
