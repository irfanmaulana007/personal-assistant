import { Routes, Route, Navigate } from 'react-router-dom';
import { useAuth } from './hooks/useAuth';
import { Login } from './components/Login';
import { Layout } from './components/Layout';
import { Chat } from './components/Chat';
import { Skills } from './components/Skills';
import { Settings } from './components/Settings';
import { ModelSettings } from './components/settings/ModelSettings';
import { DisplaySettings } from './components/settings/DisplaySettings';
import { Dashboard } from './components/Dashboard';
import { Logs } from './components/Logs';
import { Account } from './components/Account';
import { Profile } from './components/Profile';
import { Integrations } from './components/Integrations';
import { PreferencesProvider } from './contexts/PreferencesContext';

function App() {
  const {
    user,
    authenticated,
    isAdmin,
    needsSetup,
    loading,
    submitting,
    error,
    login,
    setup,
    logout,
  } = useAuth();

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-100 text-sm text-gray-400">
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
    <PreferencesProvider>
      <Routes>
        <Route element={<Layout onLogout={logout} isAdmin={isAdmin} user={user} />}>
          <Route index element={<Chat />} />
          <Route path="chat" element={<Chat />} />
          <Route path="skills" element={<Skills />} />
          <Route path="profile" element={<Profile />} />
          {isAdmin && [
            <Route key="integrations" path="integrations" element={<Integrations />} />,
            <Route key="dashboard" path="dashboard" element={<Dashboard />} />,
            <Route key="logs" path="logs" element={<Logs />} />,
            <Route key="settings" path="settings" element={<Settings />}>
              <Route index element={<ModelSettings />} />
              <Route path="display" element={<DisplaySettings />} />
            </Route>,
            <Route key="account" path="account" element={<Account />} />,
          ]}
          <Route path="*" element={<Navigate to="/chat" replace />} />
        </Route>
      </Routes>
    </PreferencesProvider>
  );
}

export default App;
