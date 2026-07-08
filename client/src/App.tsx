import { Routes, Route, Navigate } from 'react-router-dom';
import { useAuth } from './hooks/useAuth';
import { Login } from './components/Login';
import { Layout } from './components/Layout';
import { Chat } from './components/Chat';
import { Settings } from './components/Settings';
import { ComingSoon } from './components/ComingSoon';

function App() {
  const { authenticated, login, logout, error, loading } = useAuth();

  if (!authenticated) {
    return <Login onLogin={login} error={error} loading={loading} />;
  }

  return (
    <Routes>
      <Route element={<Layout onLogout={logout} />}>
        <Route index element={<Chat />} />
        <Route path="chat" element={<Chat />} />
        <Route path="settings" element={<Settings />} />
        <Route
          path="integrations"
          element={
            <ComingSoon
              title="Integrations"
              description="Connect Google Calendar, Gmail, and more via Composio."
            />
          }
        />
        <Route
          path="dashboard"
          element={
            <ComingSoon
              title="Dashboard"
              description="Monitor your token usage and estimated cost over time."
            />
          }
        />
        <Route
          path="account"
          element={
            <ComingSoon title="Account" description="Manage users and roles (admin & member)." />
          }
        />
        <Route path="*" element={<Navigate to="/chat" replace />} />
      </Route>
    </Routes>
  );
}

export default App;
