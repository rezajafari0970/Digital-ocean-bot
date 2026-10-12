// Initial credentials require the per-install setup URL. Never persist the grant.
(() => {
 const fragment = new URLSearchParams(location.hash.slice(1));
 window.datacenterSetupGrant = fragment.get('setup') || '';
 if (window.datacenterSetupGrant) history.replaceState(null,'',location.pathname+location.search);
})();
