const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('vpnAdmin', {
  addUser: (username, email, dbPath, adminCmd) => 
    ipcRenderer.invoke('admin:add-user', { username, email, dbPath, adminCmd }),
  listUsers: (dbPath, adminCmd) => 
    ipcRenderer.invoke('admin:list-users', { dbPath, adminCmd }),
  deleteUser: (id, dbPath, adminCmd) => 
    ipcRenderer.invoke('admin:delete-user', { id, dbPath, adminCmd }),
  openFile: (options) => ipcRenderer.invoke('dialog:open-file', options)
});
