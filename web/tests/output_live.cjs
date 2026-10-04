const test=require('node:test'),assert=require('node:assert/strict'),vm=require('node:vm'),fs=require('node:fs'),path=require('node:path');
const source=fs.readFileSync(path.join(__dirname,'../static/output-live.js'),'utf8');
function harness(replies){
 const out={textContent:''},events={},timers=new Map(),calls=[];
 let seq=0;
 const document={hidden:false,getElementById:id=>{assert.equal(id,'configs');return out},addEventListener:(name,fn)=>events[name]=fn};
 const context={URL,AbortController,document,location:{href:'https://output.test/share/output/token?view=1&created_within_minutes=1'},
  setTimeout:(fn,ms)=>{timers.set(++seq,{fn,ms});return seq},clearTimeout:id=>timers.delete(id),
  fetch:async(url,options)=>{calls.push({url:String(url),options});const next=replies.shift();if(next instanceof Error)throw next;return typeof next==='function'?next():{ok:true,text:async()=>next}}};
 vm.runInNewContext(source,context);
 const settle=()=>new Promise(r=>setImmediate(r));
 return {out,document,events,calls,settle,refresh:async()=>{const entry=[...timers].find(([,x])=>x.ms===1000);assert.ok(entry,'next one-second refresh missing');timers.delete(entry[0]);entry[1].fn();await settle()}};
}
test('prints only complete URI lines and preserves the share filter',async()=>{
 const h=harness(['vless://a@host:443?x=y\r\nvless://b@host:443\r\n','']);
 await h.settle();assert.equal(h.out.textContent,'vless://a@host:443?x=y\nvless://b@host:443\n');
 assert.equal(h.calls[0].url,'https://output.test/share/output/token?created_within_minutes=1');
 assert.equal(h.calls[0].options.cache,'no-store');
 await h.refresh();assert.equal(h.out.textContent,'');
});
test('faults and malformed responses clear old credentials without extra text',async()=>{
 for(const failure of [Error('offline'),'<html>error</html>','vless://a@host\nnot a URI',()=>({ok:false})]){
  const h=harness(['vless://a@host\n',failure,'vless://b@host\n']);
  await h.settle();await h.refresh();assert.equal(h.out.textContent,'');
  await h.refresh();assert.equal(h.out.textContent,'vless://b@host\n');
 }
});
test('an in-flight request cannot republish configs after the page is hidden',async()=>{
 let finish;
 const h=harness([()=>new Promise(r=>{finish=r})]);
 await h.settle();h.document.hidden=true;h.events.visibilitychange();
 finish({ok:true,text:async()=>'vless://a@host\n'});await h.settle();
 assert.equal(h.out.textContent,'');
});
