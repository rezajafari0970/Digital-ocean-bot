(()=>{
const url=new URL(location.href);url.searchParams.delete('view');
const out=document.getElementById('configs');
let busy=false,timer;
function clear(){out.textContent=''}
async function refresh(){
 if(busy||document.hidden)return;busy=true;
 const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),5000);
 try{
  const response=await fetch(url,{cache:'no-store',signal:controller.signal});
  if(!response.ok)throw Error('Output unavailable');
  const text=await response.text(),lines=text.trim()?text.trim().split(/\r?\n/):[];
  if(lines.some(x=>!/^vless:\/\/[^\s]+$/.test(x)))throw Error('Invalid Output');
  // Never render status text, errors or HTML alongside subscription lines.
  // A page hidden during a request must not republish stale credentials.
  out.textContent=document.hidden?'':lines.length?lines.join('\n')+'\n':'';
 }catch{clear()}
 finally{clearTimeout(timeout);busy=false;timer=setTimeout(refresh,1000)}
}
document.addEventListener('visibilitychange',()=>{clearTimeout(timer);if(document.hidden)clear();else refresh()});
refresh();
})();
