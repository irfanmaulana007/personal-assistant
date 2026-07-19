import type { ReactNode } from 'react';
import { Routes, Route, Navigate, useLocation } from 'react-router-dom';
import { useAuth } from './hooks/useAuth';
import { useProjects } from './contexts/project';
import { Login } from './components/Login';
import { Layout } from './components/Layout';
import { Chat } from './components/Chat';
import { AdminSkills } from './components/AdminSkills';
import { Reminders } from './components/Reminders';
import { BucketList } from './components/BucketList';
import { Projects } from './components/Projects';
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
// always qualifies). Members are sent to the project's chat. Must render inside
// ProjectProvider and a project (/:slug) shell.
function ProjectAdminRoute({ children }: { children: ReactNode }) {
  const { canManageActive, loading, projectPath } = useProjects();
  if (loading) return null;
  return canManageActive ? <>{children}</> : <Navigate to={projectPath('chat')} replace />;
}

// SuperadminRoute guards the global platform pages. Non-superadmins are sent to
// the Projects picker, their only global surface.
function SuperadminRoute({ isAdmin, children }: { isAdmin: boolean; children: ReactNode }) {
  return isAdmin ? <>{children}</> : <Navigate to="/projects" replace />;
}

// RootRedirect decides the landing page: superadmins get the global overview,
// members get the Projects picker (their only top-level surface).
function RootRedirect({ isAdmin }: { isAdmin: boolean }) {
  return isAdmin ? <Navigate to="/overview" replace /> : <Navigate to="/projects" replace />;
}

// Old flat project paths that predate the /:slug prefix. A bookmark to one of
// these is re-pointed into the active project's shell.
const LEGACY_PROJECT_PREFIXES = new Set([
  'chat',
  'reminders',
  'bucket-list',
  'profile',
  'integrations',
  'logs',
  'dashboard',
  'workflow',
  'settings',
]);

// LegacyRedirect catches anything the two shells don't match: pre-slug bookmarks
// (/chat, /settings/project) get the active slug prefixed; an unknown path inside
// a real project shell falls back to its chat; everything else goes home.
function LegacyRedirect({ isAdmin }: { isAdmin: boolean }) {
  const { projects, activeProject, loading } = useProjects();
  const location = useLocation();
  if (loading) return null;
  const segs = location.pathname.split('/').filter(Boolean);
  if (segs.length > 0 && projects.some((p) => p.slug === segs[0])) {
    return <Navigate to={`/${segs[0]}/chat`} replace />;
  }
  if (activeProject && segs.length > 0 && LEGACY_PROJECT_PREFIXES.has(segs[0])) {
    return <Navigate to={`/${activeProject.slug}/${segs.join('/')}${location.search}`} replace />;
  }
  return <RootRedirect isAdmin={isAdmin} />;
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
          <Route path="/" element={<RootRedirect isAdmin={isAdmin} />} />

          {/* Global shell — platform-wide surfaces. The four items are superadmin
              only, except Projects which every member reaches as their picker. */}
          <Route element={<Layout mode="global" onLogout={logout} isAdmin={isAdmin} user={user} />}>
            <Route
              path="overview"
              element={
                <SuperadminRoute isAdmin={isAdmin}>
                  <ProjectsOverview />
                </SuperadminRoute>
              }
            />
            <Route
              path="account"
              element={
                <SuperadminRoute isAdmin={isAdmin}>
                  <Account />
                </SuperadminRoute>
              }
            />
            <Route path="projects" element={<Projects isAdmin={isAdmin} />} />
            <Route
              path="skills"
              element={
                <SuperadminRoute isAdmin={isAdmin}>
                  <AdminSkills />
                </SuperadminRoute>
              }
            />
            <Route path="settings" element={<Settings scope="system" isAdmin={isAdmin} />}>
              <Route index element={<Navigate to="pricing" replace />} />
              <Route
                path="pricing"
                element={
                  <SuperadminRoute isAdmin={isAdmin}>
                    <PricingSettings />
                  </SuperadminRoute>
                }
              />
            </Route>
          </Route>

          {/* Project shell — every project-scoped page, prefixed by /:slug. */}
          <Route
            path=":slug"
            element={<Layout mode="project" onLogout={logout} isAdmin={isAdmin} user={user} />}
          >
            <Route index element={<Navigate to="chat" replace />} />
            <Route path="chat" element={<Chat />} />
            <Route path="reminders" element={<Reminders isAdmin={isAdmin} />} />
            <Route path="bucket-list" element={<BucketList />} />
            {/* The project skills surface now lives under project settings; keep
                old /:slug/skills bookmarks working by redirecting there. */}
            <Route path="skills" element={<Navigate to="settings/project/skills" replace />} />
            <Route path="profile" element={<Profile />} />

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

            {/* Superadmin routines/workflow, kept project-scoped. */}
            {isAdmin && <Route path="workflow" element={<Workflow />} />}

            <Route path="settings" element={<Settings scope="project" isAdmin={isAdmin} />}>
              <Route index element={<AgentSettings />} />
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
              <Route
                path="model"
                element={
                  <ProjectAdminRoute>
                    <ModelSettings />
                  </ProjectAdminRoute>
                }
              />
              <Route path="display" element={<DisplaySettings />} />
            </Route>
          </Route>

          {/* Legacy bookmarks + anything unmatched. */}
          <Route path="*" element={<LegacyRedirect isAdmin={isAdmin} />} />
        </Routes>
      </ProjectProvider>
    </PreferencesProvider>
  );
}

export default App;
