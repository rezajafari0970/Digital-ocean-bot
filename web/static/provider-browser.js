/* Opens native-client setup in its own tab; remote rendering remains explicit. */
(() => {
 'use strict';
 function open({target,provider,mode='native',accountID='',authorizationID='',proxyIDs=[],countries=['AUTO'],authToken='',returnPath='/'}){
  const tab=target||window.open('about:blank','_blank');
  if(!tab||tab.closed)throw Error('Allow a new tab for this panel, then try again.');
  try{
   if(tab.location.href!=='about:blank')throw Error('The browser tab changed. Open it again.');
   const credential=authToken||localStorage.getItem('token')||sessionStorage.getItem('token');
   if(!credential)throw Error('Sign in to the panel before opening the provider.');
   const launch={provider,accountID,authorizationID,proxyIDs:proxyIDs.slice(0,3),countries:countries.slice(0,3),returnPath:typeof returnPath==='string'&&returnPath.startsWith('/')&&!returnPath.startsWith('//')?returnPath:'/'};
   tab.sessionStorage.setItem('providerBrowser.token',credential);
   tab.sessionStorage.setItem('providerBrowser.launch',JSON.stringify(launch));
   tab.sessionStorage.removeItem('providerBrowser.session');
   tab.sessionStorage.removeItem('providerBrowser.native');
   tab.opener=null;
   tab.location.replace(mode==='remote'?'/static/provider-browser.html':'/static/provider-native.html');
   return tab;
  }catch(e){try{tab.close()}catch{}throw e}
 }
 window.ProviderBrowser={open};
})();
