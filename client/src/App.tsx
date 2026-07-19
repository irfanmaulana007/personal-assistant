import type { ReactNode } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { useAuth } from './hooks/useAuth';
import { useProjects } from './contexts/project';
import { Login } from './components/Login';
import { Layout } from './components/Layout';
import { Chat } from './components/Chat';
import { Skills } from './components/Skills';
import { Reminders } from './components/Reminders';
import { BucketList } from './components/BucketList';
import { Settings } from './components/Settings';
import { AgentSettings } from './components/settings/AgentSettings';
import { ModelSettings } from './components/settings/ModelSettings';
import { Workflow } from './components/Workflow';
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
import { IntegrationsWhatsApp } from './components/IntegrationsWhatsApp';
import { IntegrationsTrello } from './components/IntegrationsTrello';
import { PreferencesProvider } from './contexts/PreferencesContext';
import { ProjectProvider } from './contexts/ProjectContext';
import { ProjectsOverview } from './components/dashboard/ProjectsOverview';
import {
  ProjectOverviewSettings,
  ProjectMembersSettings,
  ProjectSkillsSettings,
  ProjectFeaturesSettings,
  ProjectAuditSettings,
} from './components/settings/ProjectSettings';

// ProjectAdminRoute guards a route to admins of the active project (superadmin
// always qualifies). Members are redirected to Chat. Must render inside
// ProjectProvider.
function ProjectAdminRoute({ children }: { children: ReactNode }) {
  const { canManageActive, loading } = useProjects();
  if (loading) return null;
  return canManageActive ? <>{children}</> : <Navigate to="/chat" replace />;
}

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
      <ProjectProvider>
        <Routes>
          <Route element={<Layout onLogout={logout} isAdmin={isAdmin} user={user} />}>
            <Route index element={<Chat />} />
            <Route path="chat" element={<Chat />} />
            <Route path="reminders" element={<Reminders isAdmin={isAdmin} />} />
            <Route path="bucket-list" element={<BucketList />} />
            <Route
              path="skills"
              element={
                <ProjectAdminRoute>
                  <Skills isAdmin={isAdmin} />
                </ProjectAdminRoute>
              }
            />
            <Route path="profile" element={<Profile />} />

            {/* Project-admin surfaces, scoped to the active project. */}
            <Route
              path="integrations"
              element={
                <ProjectAdminRoute>
                  <Integrations />
                </ProjectAdminRoute>
              }
            />
            <Route
              path="integrations/whatsapp"
              element={
                <ProjectAdminRoute>
                  <IntegrationsWhatsApp isAdmin={isAdmin} />
                </ProjectAdminRoute>
              }
            />
            <Route
              path="integrations/trello"
              element={
                <ProjectAdminRoute>
                  <IntegrationsTrello />
                </ProjectAdminRoute>
              }
            />
            <Route
              path="logs"
              element={
                <ProjectAdminRoute>
                  <Logs />
                </ProjectAdminRoute>
              }
            />
            {/* Global superadmin dashboard: platform-wide metrics across every
                project. Not scoped to the active project, so it lives at its own
                top-level route above the project-scoped dashboard tabs. */}
            <Route
              path="overview"
              element={isAdmin ? <ProjectsOverview /> : <Navigate to="/chat" replace />}
            />
            <Route
              path="dashboard"
              element={
                <ProjectAdminRoute>
                  <Dashboard />
                </ProjectAdminRoute>
              }
            >
              <Route index element={<Overview />} />
              <Route path="usage" element={<Usage />} />
              <Route path="activity" element={<Activity />} />
              <Route path="performance" element={<Performance />} />
              <Route path="users" element={<Users />} />
            </Route>

            {/* Superadmin per-project dashboard: the full dashboard tabs scoped
                to a specific project by URL, without switching the active
                project. Reached by drilling in from the All Projects overview. */}
            <Route
              path="dashboard/projects/:id"
              element={isAdmin ? <Dashboard /> : <Navigate to="/dashboard" replace />}
            >
              <Route index element={<Overview />} />
              <Route path="usage" element={<Usage />} />
              <Route path="activity" element={<Activity />} />
              <Route path="performance" element={<Performance />} />
              <Route path="users" element={<Users />} />
            </Route>

            <Route path="settings" element={<Settings isAdmin={isAdmin} />}>
              <Route index element={<AgentSettings />} />
              {/* Active-project management, gated to that project's admins. */}
              <Route
                path="project"
                element={
                  <ProjectAdminRoute>
                    <ProjectOverviewSettings />
                  </ProjectAdminRoute>
                }
              />
              <Route
                path="project/members"
                element={
                  <ProjectAdminRoute>
                    <ProjectMembersSettings />
                  </ProjectAdminRoute>
                }
              />
              <Route
                path="project/skills"
                element={
                  <ProjectAdminRoute>
                    <ProjectSkillsSettings />
                  </ProjectAdminRoute>
                }
              />
              <Route
                path="project/features"
                element={
                  <ProjectAdminRoute>
                    <ProjectFeaturesSettings />
                  </ProjectAdminRoute>
                }
              />
              <Route
                path="project/audit"
                element={
                  <ProjectAdminRoute>
                    <ProjectAuditSettings />
                  </ProjectAdminRoute>
                }
              />
              {/* The model + skill API keys are per-project, so this is gated to
                  project admins (like the other Project settings) rather than to
                  global superadmins. */}
              <Route
                path="model"
                element={
                  <ProjectAdminRoute>
                    <ModelSettings />
                  </ProjectAdminRoute>
                }
              />
              {/* API keys moved into the Model page; redirect the old path. */}
              <Route path="api-keys" element={<Navigate to="/settings/model" replace />} />
              <Route path="display" element={<DisplaySettings />} />
              {isAdmin && <Route path="pricing" element={<PricingSettings />} />}
            </Route>

            {/* Superadmin-only */}
            {isAdmin && <Route path="workflow" element={<Workflow />} />}
            {isAdmin && <Route path="account" element={<Account />} />}

            <Route path="*" element={<Navigate to="/chat" replace />} />
          </Route>
        </Routes>
      </ProjectProvider>
    </PreferencesProvider>
  );
}

export default App;
