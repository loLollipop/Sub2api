// Runs before package scripts. Keep the v1 window API, but deliver responses only
// to this document's private port, never to a navigable WindowProxy.
(() => {
  const host = window.parent;
  const token = new URLSearchParams(window.location.hash.slice(1)).get('bridge_token');
  if (host === window || !token) return;

  const channel = new MessageChannel();
  const port = channel.port1;
  let active = true;
  let hostOrigin = '';
  port.onmessage = ({ data }) => {
    if (!active || !data || typeof data !== 'object') return;
    if (data.type === 'sub2api.plugin.connected') {
      hostOrigin = data.origin;
    } else if (data.type === 'sub2api.plugin.relay') {
      // Prove that the document which owns the port is still handling requests.
      // A document reached by navigation cannot echo on the previous port.
      port.postMessage({ type: 'sub2api.plugin.request', message: data.message });
    } else if (data.source === 'sub2api-plugin-host' && data.bridge_token === token) {
      window.dispatchEvent(new MessageEvent('message', {
        data,
        source: host,
        origin: hostOrigin,
      }));
    }
  };
  port.start();
  window.addEventListener('pagehide', () => {
    active = false;
    port.postMessage({ type: 'sub2api.plugin.disconnect' });
    port.close();
  }, { once: true });
  host.postMessage({
    source: 'sub2api-plugin-ui',
    bridge_token: token,
    type: 'sub2api.plugin.connect',
  }, '*', [channel.port2]);
})();
