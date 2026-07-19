import { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  updateProject,
  deleteProject,
  listProjectMembers,
  addProjectMember,
  createProjectMember,
  updateProjectMember,
  removeProjectMember,
  listProjectAudit,
  getProjectFeatures,
  setProjectFeature,
} from '../../api/client';
import { useProjects } from '../../contexts/project';
import type { Project, ProjectMember, ProjectRole, ProjectFeature, AuditEvent } from '../../types';
import { Skeleton, SkeletonCard, SkeletonListRow } from '../ui/Skeleton';
import { Modal } from '../ui/Modal';
import { Toggle } from '../ui/Toggle';
import { Skills } from '../Skills';

const inputClass =
  'rounded-xl border border-gray-200 px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30';

const selectClass =
  'rounded-lg border border-gray-200 px-2 py-1 text-sm text-gray-900 outline-none transition focus:border-indigo-500 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400';

// -------------------------------------------------------------------------
// Settings sections
//
// Each section manages the *active* project (chosen in the sidebar switcher).
// The routes are gated to project managers, and a project's effective role of
// 'superadmin' means the caller is a global superadmin. These thin wrappers
// resolve the active project from context, then render the shared cards below.
// -------------------------------------------------------------------------

function useActiveProject() {
  const { activeProject, loading } = useProjects();
  const isSuperadmin = activeProject?.role === 'superadmin';
  const canManage = activeProject?.role === 'admin' || isSuperadmin;
  return { project: activeProject, loading, canManage, isSuperadmin };
}

function LoadingCard() {
  return (
    <SkeletonCard>
      <Skeleton className="mb-4 h-3.5 w-20" />
      <div className="space-y-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="flex items-center justify-between gap-4">
            <Skeleton className="h-4 w-56 max-w-full" />
            <Skeleton className="h-7 w-24 rounded-lg" />
            <Skeleton className="h-6 w-16 rounded-lg" />
          </div>
        ))}
      </div>
    </SkeletonCard>
  );
}

function NoProject() {
  return <p className="text-sm text-gray-500 dark:text-gray-400">No active project selected.</p>;
}

export function ProjectOverviewSettings() {
  const { reload: reloadSwitcher } = useProjects();
  const navigate = useNavigate();
  const { project, loading, canManage } = useActiveProject();
  if (loading) return <LoadingCard />;
  if (!project) return <NoProject />;
  return (
    <OverviewTab
      project={project}
      canManage={canManage}
      onRenamed={reloadSwitcher}
      onDeleted={() => {
        // The active project is gone; refresh the project list and drop the user
        // back to the global Projects picker to choose another.
        reloadSwitcher();
        navigate('/projects');
      }}
    />
  );
}

export function ProjectMembersSettings() {
  const { project, loading, canManage, isSuperadmin } = useActiveProject();
  if (loading) return <LoadingCard />;
  if (!project) return <NoProject />;
  return <MembersTab projectId={project.id} canManage={canManage} isSuperadmin={isSuperadmin} />;
}

export function ProjectSkillsSettings() {
  const { project, loading, isSuperadmin } = useActiveProject();
  if (loading) return <LoadingCard />;
  if (!project) return <NoProject />;
  // The full project-admin skills surface: enable/disable the skills available
  // to this project and fork a global skill to customize its prompt here.
  return <Skills isAdmin={isSuperadmin} />;
}

export function ProjectFeaturesSettings() {
  const { project, loading, canManage } = useActiveProject();
  if (loading) return <LoadingCard />;
  if (!project) return <NoProject />;
  return <FeaturesTab projectId={project.id} canManage={canManage} />;
}

export function ProjectAuditSettings() {
  const { project, loading } = useActiveProject();
  if (loading) return <LoadingCard />;
  if (!project) return <NoProject />;
  return <AuditTab projectId={project.id} />;
}

// -------------------------------------------------------------------------
// Overview
// -------------------------------------------------------------------------

function OverviewTab({
  project,
  canManage,
  onRenamed,
  onDeleted,
}: {
  project: Project;
  canManage: boolean;
  onRenamed: () => void;
  onDeleted: () => void;
}) {
  const created = useMemo(() => {
    const d = new Date(project.created_at);
    return isNaN(d.getTime()) ? project.created_at : d.toLocaleDateString();
  }, [project.created_at]);

  return (
    <div className="space-y-6">
      <div className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-50">Project</h2>
          {canManage && <RenameButton project={project} onRenamed={onRenamed} />}
        </div>
        <dl className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <dt className="text-xs font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
              Name
            </dt>
            <dd className="mt-1 text-sm text-gray-800 dark:text-gray-100">{project.name}</dd>
          </div>
          <div>
            <dt className="text-xs font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
              Your role
            </dt>
            <dd className="mt-1 text-sm text-gray-800 dark:text-gray-100">{project.role}</dd>
          </div>
          <div>
            <dt className="text-xs font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
              Members
            </dt>
            <dd className="mt-1 text-sm text-gray-800 dark:text-gray-100">
              {project.member_count}
            </dd>
          </div>
          <div>
            <dt className="text-xs font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
              Created
            </dt>
            <dd className="mt-1 text-sm text-gray-800 dark:text-gray-100">{created}</dd>
          </div>
        </dl>
      </div>

      {canManage && <DangerZone project={project} onDeleted={onDeleted} />}
    </div>
  );
}

function RenameButton({ project, onRenamed }: { project: Project; onRenamed: () => void }) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(project.name);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const open = () => {
    setName(project.name);
    setErr('');
    setEditing(true);
  };

  const save = async () => {
    const trimmed = name.trim();
    if (!trimmed || trimmed === project.name) {
      setEditing(false);
      return;
    }
    setBusy(true);
    setErr('');
    try {
      await updateProject(project.id, trimmed);
      onRenamed();
      setEditing(false);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Rename failed');
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <button
        onClick={open}
        className="rounded-lg px-2.5 py-1 text-sm font-medium text-indigo-700 transition hover:bg-indigo-50 dark:text-indigo-400 dark:hover:bg-indigo-500/15"
      >
        Rename
      </button>

      <Modal
        open={editing}
        onClose={() => (busy ? undefined : setEditing(false))}
        title="Rename project"
        description="Give this project a new name."
      >
        <div className="space-y-4">
          <input
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') save();
            }}
            placeholder="Project name"
            className={`${inputClass} w-full`}
          />
          {err && <p className="text-sm text-red-600 dark:text-red-400">{err}</p>}
          <div className="flex justify-end gap-2">
            <button
              onClick={() => setEditing(false)}
              disabled={busy}
              className="rounded-xl px-4 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-100 disabled:opacity-50 dark:text-gray-300 dark:hover:bg-gray-700"
            >
              Cancel
            </button>
            <button
              onClick={save}
              disabled={busy || !name.trim()}
              className="rounded-xl bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-600"
            >
              Save
            </button>
          </div>
        </div>
      </Modal>
    </>
  );
}

function DangerZone({ project, onDeleted }: { project: Project; onDeleted: () => void }) {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const doDelete = async () => {
    setBusy(true);
    setErr('');
    try {
      await deleteProject(project.id);
      onDeleted();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Delete failed');
      setBusy(false);
    }
  };

  return (
    <div className="rounded-2xl border border-red-200 bg-white p-5 dark:border-red-500/30 dark:bg-gray-800">
      <h2 className="mb-1 text-sm font-semibold text-red-700 dark:text-red-400">Danger zone</h2>
      <p className="mb-4 text-sm text-gray-500 dark:text-gray-400">
        Deleting a project is permanent and removes its members, features, and data.
      </p>
      <button
        onClick={() => {
          setErr('');
          setConfirming(true);
        }}
        className="rounded-xl bg-red-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-red-500 dark:bg-red-500 dark:hover:bg-red-400"
      >
        Delete project
      </button>

      <Modal
        open={confirming}
        onClose={() => (busy ? undefined : setConfirming(false))}
        title="Delete project"
        description={`This permanently deletes "${project.name}". This cannot be undone.`}
      >
        <div className="space-y-4">
          {err && <p className="text-sm text-red-600 dark:text-red-400">{err}</p>}
          <div className="flex justify-end gap-2">
            <button
              onClick={() => setConfirming(false)}
              disabled={busy}
              className="rounded-xl px-4 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-100 disabled:opacity-50 dark:text-gray-300 dark:hover:bg-gray-700"
            >
              Cancel
            </button>
            <button
              onClick={doDelete}
              disabled={busy}
              className="rounded-xl bg-red-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-red-500 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-red-500 dark:hover:bg-red-400"
            >
              {busy ? 'Deleting…' : 'Delete project'}
            </button>
          </div>
        </div>
      </Modal>
    </div>
  );
}

// -------------------------------------------------------------------------
// Members
// -------------------------------------------------------------------------

function MembersTab({
  projectId,
  canManage,
  isSuperadmin,
}: {
  projectId: number;
  canManage: boolean;
  isSuperadmin: boolean;
}) {
  const [members, setMembers] = useState<ProjectMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    listProjectMembers(projectId)
      .then((m) => {
        if (active) {
          setMembers(m);
          setError('');
        }
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : 'Failed to load members');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [projectId]);

  if (loading) {
    return (
      <SkeletonCard>
        <Skeleton className="mb-4 h-3.5 w-20" />
        <div className="space-y-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="flex items-center justify-between gap-4">
              <Skeleton className="h-4 w-56 max-w-full" />
              <Skeleton className="h-7 w-24 rounded-lg" />
              <Skeleton className="h-6 w-16 rounded-lg" />
            </div>
          ))}
        </div>
      </SkeletonCard>
    );
  }

  if (error) {
    return <p className="text-sm text-red-600 dark:text-red-400">{error}</p>;
  }

  return (
    <MembersCard
      projectId={projectId}
      members={members}
      canManage={canManage}
      isSuperadmin={isSuperadmin}
      onChanged={setMembers}
    />
  );
}

function MembersCard({
  projectId,
  members,
  canManage,
  isSuperadmin,
  onChanged,
}: {
  projectId: number;
  members: ProjectMember[];
  canManage: boolean;
  isSuperadmin: boolean;
  onChanged: (m: ProjectMember[]) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [rowError, setRowError] = useState('');
  const [creating, setCreating] = useState(false);

  const adminCount = useMemo(() => members.filter((m) => m.role === 'admin').length, [members]);

  // Every mutation returns the refreshed member list; push it straight into state.
  const run = async (fn: () => Promise<ProjectMember[]>) => {
    setBusy(true);
    setRowError('');
    try {
      const next = await fn();
      onChanged(next);
    } catch (e) {
      setRowError(e instanceof Error ? e.message : 'Action failed');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
      <div className="mb-4 flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-50">Members</h2>
        {canManage && (
          <button
            onClick={() => setCreating(true)}
            className="rounded-xl border border-gray-200 px-3 py-1.5 text-sm font-medium text-indigo-700 transition hover:bg-indigo-50 dark:border-gray-700 dark:text-indigo-400 dark:hover:bg-indigo-500/10"
          >
            Create user
          </button>
        )}
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-gray-100 text-left text-xs font-medium uppercase tracking-wide text-gray-400 dark:border-gray-800 dark:text-gray-500">
              <th className="pb-2 font-medium">Member</th>
              <th className="pb-2 font-medium">Role</th>
              {canManage && <th className="pb-2 text-right font-medium">Actions</th>}
            </tr>
          </thead>
          <tbody>
            {members.map((m) => {
              const isLastAdmin = m.role === 'admin' && adminCount === 1;
              // Setting or clearing the admin role is superadmin-only, so a plain
              // project admin can't change roles at all (there are only two).
              const roleLocked = !isSuperadmin || isLastAdmin;
              // Removing an admin removes the admin role, so it's superadmin-gated;
              // the last admin can never be removed by anyone.
              const removeLocked = (m.role === 'admin' && !isSuperadmin) || isLastAdmin;
              return (
                <tr
                  key={m.user_id}
                  className="border-b border-gray-50 last:border-0 dark:border-gray-800"
                >
                  <td className="py-3">
                    <div className="text-gray-800 dark:text-gray-100">{m.name || m.email}</div>
                    {m.name && (
                      <div className="text-xs text-gray-500 dark:text-gray-400">{m.email}</div>
                    )}
                  </td>
                  <td className="py-3">
                    {canManage ? (
                      <div className="flex items-center gap-2">
                        <select
                          value={m.role}
                          disabled={busy || roleLocked}
                          onChange={(e) =>
                            run(() =>
                              updateProjectMember(
                                projectId,
                                m.user_id,
                                e.target.value as ProjectRole,
                              ),
                            )
                          }
                          className={selectClass}
                        >
                          {/* admin only assignable/removable by a superadmin */}
                          <option value="admin" disabled={!isSuperadmin}>
                            admin
                          </option>
                          <option value="member">member</option>
                        </select>
                        {!isSuperadmin && (
                          <span className="text-xs text-gray-400 dark:text-gray-500">
                            superadmin only
                          </span>
                        )}
                      </div>
                    ) : (
                      <span className="text-gray-800 dark:text-gray-100">{m.role}</span>
                    )}
                  </td>
                  {canManage && (
                    <td className="py-3 text-right">
                      <button
                        disabled={busy || removeLocked}
                        onClick={() => run(() => removeProjectMember(projectId, m.user_id))}
                        title={isLastAdmin ? 'A project must keep at least one admin' : undefined}
                        className="rounded-lg px-2.5 py-1 text-sm font-medium text-red-600 transition hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-red-400 dark:hover:bg-red-500/15"
                      >
                        Remove
                      </button>
                    </td>
                  )}
                </tr>
              );
            })}
            {members.length === 0 && (
              <tr>
                <td
                  colSpan={canManage ? 3 : 2}
                  className="py-6 text-center text-sm text-gray-500 dark:text-gray-400"
                >
                  No members yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {rowError && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{rowError}</p>}

      {canManage && (
        <AddMemberForm
          busy={busy}
          isSuperadmin={isSuperadmin}
          onAdd={(email, role) => run(() => addProjectMember(projectId, email, role))}
        />
      )}

      <CreateUserModal
        projectId={projectId}
        isSuperadmin={isSuperadmin}
        open={creating}
        onClose={() => setCreating(false)}
        onCreated={onChanged}
      />
    </div>
  );
}

// CreateUserModal creates a brand-new account and adds them to the project in
// one step, for onboarding someone who has no account yet. This complements
// AddMemberForm, which only attaches an existing user by email.
function CreateUserModal({
  projectId,
  isSuperadmin,
  open,
  onClose,
  onCreated,
}: {
  projectId: number;
  isSuperadmin: boolean;
  open: boolean;
  onClose: () => void;
  onCreated: (m: ProjectMember[]) => void;
}) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState<ProjectRole>('member');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const reset = () => {
    setEmail('');
    setPassword('');
    setRole('member');
    setErr('');
  };

  const close = () => {
    if (busy) return;
    reset();
    onClose();
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim() || !password) return;
    setBusy(true);
    setErr('');
    try {
      const next = await createProjectMember(projectId, email.trim(), password, role);
      onCreated(next);
      reset();
      onClose();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Failed to create user');
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={close}
      title="Create user"
      description="Create a new account and add them to this project."
    >
      <form onSubmit={submit} className="space-y-4">
        <div className="space-y-1">
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Email
          </label>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="member@email.com"
            autoFocus
            className={`${inputClass} w-full`}
          />
        </div>
        <div className="space-y-1">
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Password
          </label>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="At least 8 characters"
            className={`${inputClass} w-full`}
          />
        </div>
        <div className="space-y-1">
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Project role
          </label>
          <select
            value={role}
            onChange={(e) => setRole(e.target.value as ProjectRole)}
            className={`${inputClass} w-full`}
          >
            <option value="member">member</option>
            {/* Appointing a project admin is a superadmin-only action. */}
            {isSuperadmin && <option value="admin">admin</option>}
          </select>
        </div>
        {err && <p className="text-sm text-red-600 dark:text-red-400">{err}</p>}
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={close}
            disabled={busy}
            className="rounded-xl px-4 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-100 disabled:opacity-50 dark:text-gray-300 dark:hover:bg-gray-700"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={busy || !email.trim() || !password}
            className="rounded-xl bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-600"
          >
            {busy ? 'Creating…' : 'Create user'}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function AddMemberForm({
  busy,
  isSuperadmin,
  onAdd,
}: {
  busy: boolean;
  isSuperadmin: boolean;
  onAdd: (email: string, role: ProjectRole) => void;
}) {
  const [email, setEmail] = useState('');
  const [role, setRole] = useState<ProjectRole>('member');

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim()) return;
    onAdd(email.trim(), role);
    setEmail('');
    setRole('member');
  };

  return (
    <form
      onSubmit={submit}
      className="mt-5 flex flex-wrap items-center gap-2 border-t border-gray-100 pt-4 dark:border-gray-800"
    >
      <input
        type="email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        placeholder="member@email.com"
        className={`${inputClass} min-w-[180px] flex-1`}
      />
      <select
        value={role}
        onChange={(e) => setRole(e.target.value as ProjectRole)}
        className={`${inputClass}`}
      >
        <option value="member">member</option>
        {/* Appointing a project admin is a superadmin-only action. */}
        {isSuperadmin && <option value="admin">admin</option>}
      </select>
      <button
        type="submit"
        disabled={busy || !email.trim()}
        className="rounded-xl bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-600"
      >
        Add member
      </button>
    </form>
  );
}

// -------------------------------------------------------------------------
// Features
// -------------------------------------------------------------------------

function FeaturesTab({ projectId, canManage }: { projectId: number; canManage: boolean }) {
  const [features, setFeatures] = useState<ProjectFeature[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busyId, setBusyId] = useState<number | null>(null);

  useEffect(() => {
    let active = true;
    getProjectFeatures(projectId)
      .then((f) => {
        if (active) {
          setFeatures(f);
          setError('');
        }
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : 'Failed to load features');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [projectId]);

  const toggle = async (f: ProjectFeature) => {
    setBusyId(f.id);
    setError('');
    try {
      setFeatures(await setProjectFeature(projectId, f.id, !f.enabled));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to update feature');
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div>
      <p className="mb-4 text-sm text-gray-500 dark:text-gray-400">
        Disabling a feature hides its navigation and turns off all of its skills for this project.
      </p>

      {error && <p className="mb-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {loading ? (
        <div className="space-y-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <SkeletonListRow key={i} />
          ))}
        </div>
      ) : features.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">No features available.</p>
      ) : (
        <div className="divide-y divide-gray-100 overflow-hidden rounded-2xl border border-gray-200 bg-white dark:divide-gray-800 dark:border-gray-700 dark:bg-gray-800">
          {features.map((f) => (
            <div key={f.id} className="flex items-start gap-4 p-4">
              <div className="min-w-0 flex-1">
                <span className="text-sm font-semibold text-gray-900 dark:text-gray-50">
                  {f.name}
                </span>
                <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">{f.description}</p>
                {f.skill_keys.length > 0 && (
                  <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
                    Skills: {f.skill_keys.join(', ')}
                  </p>
                )}
              </div>
              <Toggle
                on={f.enabled}
                busy={busyId === f.id}
                disabled={!canManage}
                onClick={() => toggle(f)}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// -------------------------------------------------------------------------
// Audit
// -------------------------------------------------------------------------

function AuditTab({ projectId }: { projectId: number }) {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    listProjectAudit(projectId)
      .then((a) => {
        if (active) {
          setEvents(a);
          setError('');
        }
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : 'Failed to load audit log');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [projectId]);

  if (loading) {
    return (
      <SkeletonCard>
        <Skeleton className="mb-4 h-3.5 w-20" />
        <div className="space-y-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="flex items-center justify-between gap-4">
              <Skeleton className="h-4 w-56 max-w-full" />
              <Skeleton className="h-4 w-24" />
              <Skeleton className="h-4 w-28" />
            </div>
          ))}
        </div>
      </SkeletonCard>
    );
  }

  if (error) {
    return <p className="text-sm text-red-600 dark:text-red-400">{error}</p>;
  }

  return <AuditCard events={events} />;
}

function AuditCard({ events }: { events: AuditEvent[] }) {
  // Newest first.
  const sorted = useMemo(
    () => [...events].sort((a, b) => b.created_at.localeCompare(a.created_at)),
    [events],
  );

  const fmt = (ts: string) => {
    const d = new Date(ts);
    return isNaN(d.getTime()) ? ts : d.toLocaleString();
  };

  const compactMeta = (raw: string) => {
    if (!raw) return '';
    try {
      const obj = JSON.parse(raw);
      if (obj && typeof obj === 'object') {
        return Object.entries(obj)
          .map(([k, v]) => `${k}: ${typeof v === 'object' ? JSON.stringify(v) : String(v)}`)
          .join(', ');
      }
    } catch {
      /* not JSON — show as-is */
    }
    return raw;
  };

  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
      <h2 className="mb-4 text-sm font-semibold text-gray-900 dark:text-gray-50">Audit log</h2>

      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-gray-100 text-left text-xs font-medium uppercase tracking-wide text-gray-400 dark:border-gray-800 dark:text-gray-500">
              <th className="pb-2 font-medium">Action</th>
              <th className="pb-2 font-medium">Target</th>
              <th className="pb-2 font-medium">Actor</th>
              <th className="pb-2 text-right font-medium">Time</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((e) => {
              const meta = compactMeta(e.metadata);
              return (
                <tr
                  key={e.id}
                  className="border-b border-gray-50 last:border-0 dark:border-gray-800"
                >
                  <td className="py-3 align-top">
                    <div className="font-medium text-gray-800 dark:text-gray-100">{e.action}</div>
                    {meta && (
                      <div className="mt-0.5 max-w-md truncate font-mono text-xs text-gray-500 dark:text-gray-400">
                        {meta}
                      </div>
                    )}
                  </td>
                  <td className="py-3 align-top text-gray-800 dark:text-gray-100">{e.target}</td>
                  <td className="py-3 align-top text-gray-500 dark:text-gray-400">
                    {e.actor_email}
                  </td>
                  <td className="whitespace-nowrap py-3 text-right align-top text-gray-500 dark:text-gray-400">
                    {fmt(e.created_at)}
                  </td>
                </tr>
              );
            })}
            {sorted.length === 0 && (
              <tr>
                <td
                  colSpan={4}
                  className="py-6 text-center text-sm text-gray-500 dark:text-gray-400"
                >
                  No audit events yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
