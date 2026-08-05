import React, { useState, useEffect } from 'react';
import './App.css';

function App() {
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [newUser, setNewUser] = useState({ username: '', email: '' });
  const [dbPath, setDbPath] = useState('./vpn.db');
  const [adminCmd, setAdminCmd] = useState('./vpn-admin');
  const [message, setMessage] = useState('');

  useEffect(() => {
    loadUsers();
  }, []);

  const loadUsers = async () => {
    setLoading(true);
    try {
      const result = await window.vpnAdmin.listUsers(dbPath, adminCmd);
      if (result.success) {
        // Parse the output to extract users
        const lines = result.output.split('\n').slice(2); // skip header
        const parsedUsers = lines
          .filter(line => line.trim() && !line.includes('─'))
          .map(line => {
            const parts = line.trim().split(/\s+/);
            return {
              id: parts[0],
              username: parts[1],
              token: parts[2],
              email: parts[3] || ''
            };
          });
        setUsers(parsedUsers);
        setMessage('');
      } else {
        setMessage(`Error: ${result.error}`);
      }
    } catch (error) {
      setMessage(`Failed to load users: ${error.message}`);
    }
    setLoading(false);
  };

  const handleAddUser = async (e) => {
    e.preventDefault();
    if (!newUser.username.trim()) {
      setMessage('Username is required');
      return;
    }

    setLoading(true);
    try {
      const result = await window.vpnAdmin.addUser(newUser.username, newUser.email, dbPath, adminCmd);
      if (result.success) {
        setMessage(`✅ User ${newUser.username} added successfully`);
        setNewUser({ username: '', email: '' });
        await loadUsers();
      } else {
        setMessage(`Error: ${result.error}`);
      }
    } catch (error) {
      setMessage(`Failed to add user: ${error.message}`);
    }
    setLoading(false);
  };

  const handleDeleteUser = async (id) => {
    if (!window.confirm('Are you sure you want to delete this user?')) {
      return;
    }

    setLoading(true);
    try {
      const result = await window.vpnAdmin.deleteUser(id, dbPath, adminCmd);
      if (result.success) {
        setMessage(`✅ User deleted successfully`);
        await loadUsers();
      } else {
        setMessage(`Error: ${result.error}`);
      }
    } catch (error) {
      setMessage(`Failed to delete user: ${error.message}`);
    }
    setLoading(false);
  };

  return (
    <div className="App">
      <header className="App-header">
        <h1>🔐 VPN Admin Panel</h1>
        <p>Manage your VPN users</p>
      </header>

      <main className="container">
        {message && (
          <div className={`message ${message.includes('Error') ? 'error' : 'success'}`}>
            {message}
          </div>
        )}

        {/* Add User Form */}
        <section className="section">
          <h2>Add New User</h2>
          <form onSubmit={handleAddUser} className="form">
            <div className="form-group">
              <label htmlFor="username">Username:</label>
              <input
                type="text"
                id="username"
                value={newUser.username}
                onChange={(e) => setNewUser({ ...newUser, username: e.target.value })}
                placeholder="Enter username"
                disabled={loading}
              />
            </div>
            <div className="form-group">
              <label htmlFor="email">Email:</label>
              <input
                type="email"
                id="email"
                value={newUser.email}
                onChange={(e) => setNewUser({ ...newUser, email: e.target.value })}
                placeholder="Enter email (optional)"
                disabled={loading}
              />
            </div>
            <button type="submit" disabled={loading} className="btn btn-primary">
              {loading ? 'Adding...' : 'Add User'}
            </button>
          </form>
        </section>

        {/* Users List */}
        <section className="section">
          <div className="section-header">
            <h2>Active Users</h2>
            <button onClick={loadUsers} disabled={loading} className="btn btn-secondary">
              {loading ? 'Refreshing...' : 'Refresh'}
            </button>
          </div>

          {users.length === 0 ? (
            <p className="empty">No users yet. Create one above!</p>
          ) : (
            <div className="table-container">
              <table className="users-table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Username</th>
                    <th>Token</th>
                    <th>Email</th>
                    <th>Action</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((user) => (
                    <tr key={user.id}>
                      <td>{user.id}</td>
                      <td className="username">{user.username}</td>
                      <td className="token" title={user.token}>
                        {user.token.substring(0, 12)}...
                      </td>
                      <td>{user.email}</td>
                      <td>
                        <button
                          onClick={() => handleDeleteUser(user.id)}
                          disabled={loading}
                          className="btn btn-danger btn-small"
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        {/* Settings */}
        <section className="section settings">
          <h2>Settings</h2>
          <div className="form-group">
            <label htmlFor="dbPath">Database Path:</label>
            <input
              type="text"
              id="dbPath"
              value={dbPath}
              onChange={(e) => setDbPath(e.target.value)}
              placeholder="./vpn.db"
            />
          </div>
          <div className="form-group">
            <label htmlFor="adminCmd">Admin Command:</label>
            <input
              type="text"
              id="adminCmd"
              value={adminCmd}
              onChange={(e) => setAdminCmd(e.target.value)}
              placeholder="./vpn-admin"
            />
          </div>
        </section>
      </main>
    </div>
  );
}

export default App;
