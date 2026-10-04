(()=>{
const url=new URL(location.href);url.searchParams.delete('view');
const status=document.getElementById('status'),out=document.getElementById('configs'),copy=document.getElementById('copy');
let busy=false,timer;
function clear(message){out.textContent='';copy.disabled=true;status.textContent=message}
async function refresh(){
 if(busy||document.hidden)return;busy=true;const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),5000);
 try{
  const response=await fetch(url,{cache:'no-store',signal:controller.signal});
  if(!response.ok)throw Error('Output request failed ('+response.status+').');
  const text=await response.text(),lines=text.trim()?text.trim().split(/\r?\n/):[];
  if(lines.some(x=>!x.startsWith('vless://')))throw Error('Unexpected Output response.');
  out.textContent=text;copy.disabled=!lines.length;
  status.textContent=lines.length?lines.length+' verified configs · updates every second':'No verified active configs for this link yet. Creation may be paused, routing unavailable, or no configs match its time filter. This page retries automatically.';
 }catch(error){clear(error.name==='AbortError'?'Output check timed out. Retrying…':error.message)}
 finally{clearTimeout(timeout);busy=false;timer=setTimeout(refresh,1000)}
}
copy.addEventListener('click',async()=>{try{await navigator.clipboard.writeText(out.textContent);status.textContent='Configs copied'}catch{const selection=getSelection(),range=document.createRange();range.selectNodeContents(out);selection.removeAllRanges();selection.addRange(range);status.textContent='Configs selected. Use Copy.'}});
document.addEventListener('visibilitychange',()=>{clearTimeout(timer);if(document.hidden)clear('Paused while this page is hidden.');else refresh()});
refresh();
})();
