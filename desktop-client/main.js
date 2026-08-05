const { app, BrowserWindow, ipcMain } = require('electron');
const { execFile } = require('child_process');
const fs = require('fs');
const path = require('path');

const LOG_PATH = '/tmp/vpn-client-gui.log';

// The tunnel needs root, so the binary is launched through osascript, which
// shows the standard macOS authentication dialog.
let tunnelPid = null;

function clientBinary() {
  const packaged = path.join(process.resourcesPath || '', 'vpn-client');
  if (fs.existsSync(packaged)) {
    return packaged;
  }
  return path.resolve(__dirname, '..', 'vpn-client');
}

function shellQuote(value) {
  return `'${String(value).replace(/'/g, `'\\''`)}'`;
}

// osascript takes the script as an AppleScript string, so quotes are escaped
// twice: once for the shell command inside, once for AppleScript itself.
function appleScriptQuote(value) {
  return `"${String(value).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

function runElevated(command) {
  return new Promise((resolve, reject) => {
    const script = `do shell script ${appleScriptQuote(command)} with administrator privileges`;
    execFile('osascript', ['-e', script], (error, stdout, stderr) => {
      if (error) {
        reject(new Error(stderr.trim() || error.message));
        return;
      }
      resolve(stdout.trim());
    });
  });
}

ipcMain.handle('vpn:connect', async (_event, { server, token }) => {
  const binary = clientBinary();
  if (!fs.existsSync(binary)) {
    throw new Error(`vpn-client not found at ${binary}. Build it with ./test_vpn.sh first.`);
  }

  fs.writeFileSync(LOG_PATH, '');
  const command =
    `${shellQuote(binary)} -server ${shellQuote(server)} -token ${shellQuote(token)} ` +
    `> ${shellQuote(LOG_PATH)} 2>&1 & echo $!`;

  const pid = await runElevated(command);
  tunnelPid = parseInt(pid, 10);
  if (!Number.isInteger(tunnelPid)) {
    throw new Error('Could not determine the client process id');
  }
  return { pid: tunnelPid };
});

ipcMain.handle('vpn:disconnect', async () => {
  if (!tunnelPid) {
    return { stopped: false };
  }
  // SIGTERM lets the client restore routing and DNS before it exits.
  await runElevated(`kill -TERM ${tunnelPid}`);
  tunnelPid = null;
  return { stopped: true };
});

ipcMain.handle('vpn:status', async () => {
  let log = '';
  try {
    log = fs.readFileSync(LOG_PATH, 'utf8');
  } catch {
    log = '';
  }
  return { running: tunnelPid !== null, log };
});

function createWindow() {
  const window = new BrowserWindow({
    width: 420,
    height: 560,
    resizable: false,
    title: 'VPN Client',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });
  window.loadFile('index.html');
}

app.whenReady().then(createWindow);

app.on('window-all-closed', () => {
  app.quit();
});
