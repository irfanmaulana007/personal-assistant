import { Routes, Route, Navigate } from 'react-router-dom';
import { useAuth } from './hooks/useAuth';
import { Login } from './components/Login';
import { Layout } from './components/Layout';
import { Chat } from './components/Chat';
import { Settings } from './components/Settings';
import { Dashboard } from './components/Dashboard';
import { Logs } from './components/Logs';
import { Account } from './components/Account';
import { Profile } from './components/Profile';
import { Integrations } from './components/Integrations';

function App() {
  const { authenticated, isAdmin, needsSetup, loading, submitting, error, login, setup, logout } =
    useAuth();

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50 text-sm text-gray-400">
        Loading…
      </div>
    );
  }

  if (needsSetup) {
    return <Login mode="setup" onSubmit={setup} error={error} loading={submitting} />;
  }

  if (!authenticated) {
    return <Login mode="login" onSubmit={login} error={error} loading={submitting} />;
  }

  return (
    <Routes>
      <Route element={<Layout onLogout={logout} isAdmin={isAdmin} />}>
        <Route index element={<Chat />} />
        <Route path="chat" element={<Chat />} />
        <Route path="profile" element={<Profile />} />
        {isAdmin && [
          <Route key="integrations" path="integrations" element={<Integrations />} />,
          <Route key="dashboard" path="dashboard" element={<Dashboard />} />,
          <Route key="logs" path="logs" element={<Logs />} />,
          <Route key="settings" path="settings" element={<Settings />} />,
          <Route key="account" path="account" element={<Account />} />,
        ]}
        <Route path="*" element={<Navigate to="/chat" replace />} />
      </Route>
    </Routes>
  );
}

export default App;
