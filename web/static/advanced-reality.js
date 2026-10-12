// Observation only. Uses the host panel's auth, escaping and page lifecycle.
function renderAdvancedReality(d) {
  if (!d || !Array.isArray(d.panels) || !Array.isArray(d.capabilities) || !Array.isArray(d.unmeasured)) throw Error('Invalid engine status');
  const panels = d.panels;
  const fresh = panels.filter(p => p.route_fresh).length;
  const osFresh = panels.filter(p => p.os_metrics_fresh).length;
  const when = v => v ? esc(v) : 'Unknown';
  const metric = (p, key) => p.os_metrics_fresh && Number.isFinite(p.os_last_observation?.[key]) ? esc(p.os_last_observation[key]) : 'Unknown';
  return '<div class="card section"><h3>Advanced Reality Engine</h3><p><strong>' + esc(d.mode) + '</strong> · ' + esc(d.version) + '</p>' +
    '<p>Experimental observation and offline validation. Traffic changes are unavailable until server and official-client tests pass.</p>' +
    '<p>Snapshot: ' + when(d.observed_at) + ' · Route evidence ≤60s: ' + fresh + '/' + panels.length + ' · OS samples ≤60s: ' + osFresh + '/' + panels.length + '</p>' +
    (!d.enabled ? '<p class="muted">Observation is disabled. No fleet collection is started by this feature.</p>' : '') +
    '<button type="button" id="advancedRealityRefresh" class="ghost">Refresh status</button></div>' +
    '<div class="card section"><h3>Server observations</h3><div class="tableWrap"><table class="table"><thead><tr><th>Panel</th><th>Server / route</th><th>Core</th><th>CPU %</th><th>Available RAM MB</th><th>Observation</th><th>Inbounds</th></tr></thead><tbody>' +
    panels.map(p => '<tr><td>' + esc(p.id) + '<br>Panel: ' + esc(p.panel_version || 'Unknown') + '</td><td>' + esc(p.server_state) + ' / ' + esc(p.route_state) + '<br>' + (p.route_fresh ? 'Fresh route proof' : 'Missing or stale route proof') + '</td><td>' + esc(p.core_version || 'Unverified') + '</td><td>' + metric(p, 'cpu_percent') + '</td><td>' + metric(p, 'mem_available_mb') + '</td><td>' + when(p.os_observed_at) + '</td><td>' +
      (p.inbounds || []).map(i => esc(i.id) + ': ' + esc(i.protocol) + '/' + esc(i.transport) + '/' + esc(i.security) + ' · ' + esc(i.port) + ' · ' + (i.fresh ? 'Fresh' : 'Stale') + '<br>').join('') + '</td></tr>').join('') +
    '</tbody></table></div></div><div class="card section"><h3>Capability evidence</h3><div class="tableWrap"><table class="table"><thead><tr><th>Capability</th><th>Layer</th><th>Status</th><th>Scope</th></tr></thead><tbody>' +
    d.capabilities.map(x => '<tr><td>' + esc(x.id) + '</td><td>' + esc(x.layer) + '</td><td>' + esc(x.status) + '</td><td>' + esc(x.note) + '</td></tr>').join('') +
    '</tbody></table></div></div><div class="card section"><h3>Measurements not yet available</h3><p>' + d.unmeasured.map(esc).join(', ') + '</p><p>Interface traffic is not tunnel goodput. A fresh route proof does not establish connectivity from a user network or resistance to DPI.</p></div>';
}

async function advancedRealityPage(c, view) {
  const d = await viewRead('/api/v1/advanced-reality', view);
  if (!currentPageView(view)) return;
  c.innerHTML = renderAdvancedReality(d);
  document.getElementById('advancedRealityRefresh').onclick = () => load('advanced-reality');
}
