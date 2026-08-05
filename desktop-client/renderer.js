const serverInput = document.getElementById('server');
const tokenInput = document.getElementById('token');
const actionButton = document.getElementById('action');
const statusText = document.getElementById('statusText');
const dot = document.getElementById('dot');
const logView = document.getElementById('log');

let connected = false;
let busy = false;

serverInput.value = localStorage.getItem('server') || '';
tokenInput.value = localStorage.getItem('token') || '';

function render() {
  actionButton.textContent = connected ? 'Disconnect' : 'Connect';
  actionButton.disabled = busy;
  serverInput.disabled = connected || busy;
  tokenInput.disabled = connected || busy;
  dot.className = `dot ${connected ? 'on' : 'off'}`;
}

function setStatus(message) {
  statusText.textContent = message;
}

actionButton.addEventListener('click', async () => {
  busy = true;
  render();

  try {
    if (connected) {
      await window.vpn.disconnect();
      connected = false;
      setStatus('Not connected');
      logView.textContent = '';
    } else {
      const server = serverInput.value.trim();
      const token = tokenInput.value.trim();
      if (!server || !token) {
        setStatus('Enter the server address and your token');
        return;
      }

      localStorage.setItem('server', server);
      localStorage.setItem('token', token);

      setStatus('Connecting…');
      await window.vpn.connect(server, token);
      connected = true;
    }
  } catch (error) {
    connected = false;
    setStatus(error.message);
  } finally {
    busy = false;
    render();
  }
});

// The client reports progress through its log, so poll it while connected.
setInterval(async () => {
  const { running, log } = await window.vpn.status();
  logView.textContent = log.split('\n').slice(-8).join('\n');

  if (!running) {
    return;
  }
  if (log.includes('VPN active')) {
    const match = log.match(/assigned IP: (\S+)/);
    setStatus(match ? `Connected — ${match[1]}` : 'Connected');
  } else if (/failed|error/i.test(log)) {
    setStatus(log.trim().split('\n').pop());
  }
}, 1000);

render();
