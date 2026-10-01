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

function point(canvas, touch) {
  const rect = canvas.getBoundingClientRect();
  return {
    x: Math.max(0, Math.min(411, (touch.clientX - rect.left) * 412 / rect.width)),
    y: Math.max(0, Math.min(914, (touch.clientY - rect.top) * 915 / rect.height)),
  };
}

function send(path, payload) {
  fetch('/vultr-browser/input/' + path, {
    method: 'POST',
    credentials: 'same-origin',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(payload),
  }).catch(() => {});
}

function installMobileInputBridge() {
  const canvas = screen.querySelector('canvas');
  if (!canvas || canvas.dataset.inputBridge === '1') return;
  canvas.dataset.inputBridge = '1';

  let start = null;
  let last = null;
  let scrolling = false;
  let pending = 0;
  let timer = null;
  const threshold = 14;

  const block = (e) => {
    e.preventDefault();
    e.stopImmediatePropagation();
  };

  canvas.addEventListener('touchstart', (e) => {
    if (e.touches.length !== 1) { block(e); return; }
    block(e);
    const t = e.touches[0];
    start = {clientX: t.clientX, clientY: t.clientY, at: performance.now()};
    last = {clientX: t.clientX, clientY: t.clientY};
    scrolling = false;
    pending = 0;
  }, {capture: true, passive: false});

  canvas.addEventListener('touchmove', (e) => {
    block(e);
    if (!start || e.touches.length !== 1) return;
    const t = e.touches[0];
    const distance = Math.hypot(t.clientX - start.clientX, t.clientY - start.clientY);
    if (!scrolling && distance < threshold) return;
    scrolling = true;
    pending += last.clientY - t.clientY;
    last = {clientX: t.clientX, clientY: t.clientY};
    if (!timer) {
      timer = setTimeout(() => {
        timer = null;
        if (!pending) return;
        const p = point(canvas, last);
        const deltaY = Math.max(-900, Math.min(900, pending * 2.2));
        pending = 0;
        send('scroll', {...p, delta_y: deltaY});
      }, 45);
    }
  }, {capture: true, passive: false});

  canvas.addEventListener('touchend', (e) => {
    block(e);
    if (!start) return;
    const t = e.changedTouches[0];
    if (timer) { clearTimeout(timer); timer = null; }
    if (scrolling) {
      if (pending) {
        const p = point(canvas, t);
        send('scroll', {...p, delta_y: Math.max(-900, Math.min(900, pending * 2.2))});
      }
    } else if (performance.now() - start.at < 550) {
      send('tap', point(canvas, t));
    }
    start = null;
    last = null;
    scrolling = false;
    pending = 0;
  }, {capture: true, passive: false});

  canvas.addEventListener('touchcancel', (e) => {
    block(e);
    if (timer) { clearTimeout(timer); timer = null; }
    start = last = null;
    scrolling = false;
    pending = 0;
  }, {capture: true, passive: false});

  canvas.addEventListener('contextmenu', (e) => e.preventDefault(), {capture: true});
}

rfb.addEventListener('connect', () => {
  status.textContent = 'Connected';
  installMobileInputBridge();
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
