const assert=require('node:assert/strict'),Preview=require('../static/preview-stream.js');
const stream=chunks=>new Response(new ReadableStream({start(c){for(const s of chunks)c.enqueue(new TextEncoder().encode(s));c.close()}}),{headers:{'content-type':Preview.type}});
(async()=>{
 const events=[];
 const res=await Preview.response(stream(['{"type":"pro','gress","stage":"catalog","elapsed_seconds":12}\n{"type":"result","status":403,"body":{"error":"provider_permission_denied","detail":"Denied"}}\n']),e=>events.push(e));
 assert.equal(res.status,403);assert.equal((await res.json()).error,'provider_permission_denied');assert.equal(events.length,1);assert.match(Preview.message(events[0]),/12s/);
 for(const chunks of [[],['{"type":"progress","stage":"catalog","elapsed_seconds":1}\n'],['broken\n'],['{"type":"result","status":999,"body":{}}\n']])await assert.rejects(()=>Preview.response(stream(chunks),()=>{}));
 const json=new Response('{"provider":"aws"}',{headers:{'content-type':'application/json'}});assert.equal(await Preview.response(json,()=>{}),json);
 const html=new Response('<html>502</html>',{status:502});assert.equal(await Preview.response(html,()=>{}),html);
 console.log('preview stream: PASS (split frames, terminal errors, truncation, JSON compatibility)');
})().catch(e=>{console.error(e);process.exitCode=1});
