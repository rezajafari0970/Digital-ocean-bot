import RFB from '/vultr-browser/core/rfb.js';

const status = document.getElementById('status');
const screen = document.getElementById('screen');
const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
const wsURL = scheme + '://' + location.host + '/websockify';

status.textContent = 'Connecting…';
const rfb = new RFB(screen, wsURL, { shared: true });
rfb.scaleViewport = true;
rfb.resizeSession = false;
rfb.addEventListener('connect', () => { status.textContent = 'Connected'; });
rfb.addEventListener('disconnect', e => {
  status.textContent = e.detail.clean ? 'Disconnected' : 'Connection failed';
});
rfb.addEventListener('credentialsrequired', () => {
  status.textContent = 'VNC credentials required';
});
window.addEventListener('beforeunload', () => {
  try { rfb.disconnect(); } catch (_) {}
});
