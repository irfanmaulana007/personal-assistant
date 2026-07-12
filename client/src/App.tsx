import { Routes, Route, Navigate } from 'react-router-dom';
import { useAuth } from './hooks/useAuth';
import { Login } from './components/Login';
import { Layout } from './components/Layout';
import { Chat } from './components/Chat';
import { Skills } from './components/Skills';
import { Reminders } from './components/Reminders';
import { BucketList } from './components/BucketList';
import { Settings } from './components/Settings';
import { AgentSettings } from './components/settings/AgentSettings';
import { ModelSettings } from './components/settings/ModelSettings';
import { WhatsAppSettings } from './components/settings/WhatsAppSettings';
import { RoutinesSettings } from './components/settings/RoutinesSettings';
import { DisplaySettings } from './components/settings/DisplaySettings';
import { PricingSettings } from './components/settings/PricingSettings';
import { Dashboard } from './components/Dashboard';
import { Overview } from './components/dashboard/Overview';
import { Usage } from './components/dashboard/Usage';
import { Activity } from './components/dashboard/Activity';
import { Performance } from './components/dashboard/Performance';
import { Users } from './components/dashboard/Users';
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
      <div className="min-h-screen flex items-center justify-center bg-gray-100 text-sm text-gray-400 dark:bg-gray-900 dark:text-gray-500">
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
          <Route path="reminders" element={<Reminders isAdmin={isAdmin} />} />
          <Route path="bucket-list" element={<BucketList />} />
          <Route path="profile" element={<Profile />} />
          <Route path="settings" element={<Settings isAdmin={isAdmin} />}>
            <Route index element={<AgentSettings />} />
            {isAdmin && <Route path="model" element={<ModelSettings />} />}
            {isAdmin && <Route path="whatsapp" element={<WhatsAppSettings />} />}
            {isAdmin && <Route path="daily-skills" element={<RoutinesSettings />} />}
            <Route path="display" element={<DisplaySettings />} />
            {isAdmin && <Route path="pricing" element={<PricingSettings />} />}
          </Route>
          {isAdmin && [
            <Route key="integrations" path="integrations" element={<Integrations />} />,
            <Route key="dashboard" path="dashboard" element={<Dashboard />}>
              <Route index element={<Overview />} />
              <Route path="usage" element={<Usage />} />
              <Route path="activity" element={<Activity />} />
              <Route path="performance" element={<Performance />} />
              <Route path="users" element={<Users />} />
            </Route>,
            <Route key="logs" path="logs" element={<Logs />} />,
            <Route key="account" path="account" element={<Account />} />,
          ]}
          <Route path="*" element={<Navigate to="/chat" replace />} />
        </Route>
      </Routes>
    </PreferencesProvider>
  );
}

export default App;
