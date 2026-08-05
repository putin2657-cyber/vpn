const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('vpn', {
  connect: (server, token) => ipcRenderer.invoke('vpn:connect', { server, token }),
  disconnect: () => ipcRenderer.invoke('vpn:disconnect'),
  status: () => ipcRenderer.invoke('vpn:status'),
});
