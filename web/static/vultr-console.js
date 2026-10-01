import RFB from '/vultr-browser/core/rfb.js';

const status = document.getElementById('status');
const screen = document.getElementById('screen');
const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
const wsURL = scheme + '://' + location.host + '/websockify';

status.textContent = 'Connecting…';
const rfb = new RFB(screen, wsURL, { shared: true });
rfb.scaleViewport = true;
rfb.resizeSession = false;
rfb.clipViewport = false;
rfb.dragViewport = false;
rfb.focusOnClick = true;

function installMobileScrollBridge() {
  const canvas = screen.querySelector('canvas');
  if (!canvas || canvas.dataset.scrollBridge === '1') return;
  canvas.dataset.scrollBridge = '1';

  let startX = 0;
  let startY = 0;
  let lastY = 0;
  let pending = 0;
  let scrolling = false;
  let timer = null;

  const flush = (touch) => {
    if (!pending) return;
    const rect = canvas.getBoundingClientRect();
    const x = Math.max(0, Math.min(411, (touch.clientX - rect.left) * 412 / rect.width));
    const y = Math.max(0, Math.min(914, (touch.clientY - rect.top) * 915 / rect.height));
    const deltaY = Math.max(-900, Math.min(900, pending * 2.2));
    pending = 0;
    fetch('/vultr-browser/input/scroll', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({x, y, delta_y: deltaY}),
      keepalive: false,
    }).catch(() => {});
  };

  canvas.addEventListener('touchstart', (e) => {
    if (e.touches.length !== 1) return;
    const t = e.touches[0];
    startX = t.clientX;
    startY = lastY = t.clientY;
    pending = 0;
    scrolling = false;
  }, {capture: true, passive: false});

  canvas.addEventListener('touchmove', (e) => {
    if (e.touches.length !== 1) return;
    const t = e.touches[0];
    if (!scrolling && Math.hypot(t.clientX - startX, t.clientY - startY) < 16) return;
    scrolling = true;
    e.preventDefault();
    e.stopImmediatePropagation();
    pending += lastY - t.clientY;
    lastY = t.clientY;
    if (!timer) {
      const snapshot = {clientX: t.clientX, clientY: t.clientY};
      timer = setTimeout(() => {
        timer = null;
        flush(snapshot);
      }, 55);
    }
  }, {capture: true, passive: false});

  canvas.addEventListener('touchend', (e) => {
    if (!scrolling) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    const t = e.changedTouches[0];
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    flush(t);
    scrolling = false;
  }, {capture: true, passive: false});
}

rfb.addEventListener('connect', () => {
  status.textContent = 'Connected';
  installMobileScrollBridge();
  setTimeout(() => { status.hidden = true; }, 900);
});
rfb.addEventListener('disconnect', e => {
  status.hidden = false;
  status.textContent = e.detail.clean ? 'Disconnected' : 'Connection failed';
});
rfb.addEventListener('credentialsrequired', () => {
  status.hidden = false;
  status.textContent = 'VNC credentials required';
});
window.addEventListener('beforeunload', () => {
  try { rfb.disconnect(); } catch (_) {}
});
