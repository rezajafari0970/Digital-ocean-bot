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

function installMobileGestures() {
  const canvas = screen.querySelector('canvas');
  if (!canvas || canvas.dataset.mobileGestures === '1') return;
  canvas.dataset.mobileGestures = '1';

  let startX = 0;
  let startY = 0;
  let lastY = 0;
  let swiping = false;
  const threshold = 14;

  canvas.addEventListener('touchstart', (e) => {
    if (e.touches.length !== 1) return;
    const t = e.touches[0];
    startX = t.clientX;
    startY = lastY = t.clientY;
    swiping = false;
  }, { capture: true, passive: false });

  canvas.addEventListener('touchmove', (e) => {
    if (e.touches.length !== 1) return;
    const t = e.touches[0];
    const dx = t.clientX - startX;
    const dy = t.clientY - startY;
    if (!swiping && Math.hypot(dx, dy) < threshold) return;
    swiping = true;
    e.preventDefault();
    e.stopImmediatePropagation();

    const delta = lastY - t.clientY;
    lastY = t.clientY;
    canvas.dispatchEvent(new WheelEvent('wheel', {
      deltaY: delta * 2.4,
      deltaMode: WheelEvent.DOM_DELTA_PIXEL,
      clientX: t.clientX,
      clientY: t.clientY,
      bubbles: true,
      cancelable: true,
    }));
  }, { capture: true, passive: false });

  canvas.addEventListener('touchend', (e) => {
    if (swiping) {
      e.preventDefault();
      e.stopImmediatePropagation();
    }
    swiping = false;
  }, { capture: true, passive: false });
}

rfb.addEventListener('connect', () => {
  status.textContent = 'Connected';
  installMobileGestures();
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
