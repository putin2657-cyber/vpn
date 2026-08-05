const { app, BrowserWindow, ipcMain, dialog } = require('electron');
const path = require('path');
const isDev = require('electron-is-dev');
const { execSync } = require('child_process');

let mainWindow;

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1200,
    height: 800,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      enableRemoteModule: false
    }
  });

  const startUrl = isDev
    ? 'http://localhost:3000'
    : `file://${path.join(__dirname, '../build/index.html')}`;

  mainWindow.loadURL(startUrl);

  if (isDev) {
    mainWindow.webContents.openDevTools();
  }

  mainWindow.on('closed', () => {
    mainWindow = null;
  });
}

app.on('ready', createWindow);

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('activate', () => {
  if (mainWindow === null) {
    createWindow();
  }
});

// IPC Handlers for admin operations
ipcMain.handle('admin:add-user', async (event, { username, email, dbPath, adminCmd }) => {
  try {
    const cmd = `${adminCmd} -action add -user ${username} -email ${email} -db ${dbPath}`;
    const output = execSync(cmd, { encoding: 'utf8' });
    return { success: true, output };
  } catch (error) {
    return { success: false, error: error.message };
  }
});

ipcMain.handle('admin:list-users', async (event, { dbPath, adminCmd }) => {
  try {
    const cmd = `${adminCmd} -action list -db ${dbPath}`;
    const output = execSync(cmd, { encoding: 'utf8' });
    return { success: true, output };
  } catch (error) {
    return { success: false, error: error.message };
  }
});

ipcMain.handle('admin:delete-user', async (event, { id, dbPath, adminCmd }) => {
  try {
    const cmd = `${adminCmd} -action delete -id ${id} -db ${dbPath}`;
    const output = execSync(cmd, { encoding: 'utf8' });
    return { success: true, output };
  } catch (error) {
    return { success: false, error: error.message };
  }
});

ipcMain.handle('dialog:open-file', async (event, options) => {
  return dialog.showOpenDialog(mainWindow, options);
});

