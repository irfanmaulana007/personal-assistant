import { useState, useEffect, useMemo } from 'react';
import { Link, useParams, useNavigate } from 'react-router-dom';
import {
  listProjects,
  updateProject,
  deleteProject,
  listProjectMembers,
  addProjectMember,
  updateProjectMember,
  removeProjectMember,
  listProjectAudit,
} from '../api/client';
import { useProjects } from '../contexts/project';
import type { Project, ProjectMember, ProjectRole, AuditEvent } from '../types';
import { Skeleton, SkeletonCard } from './ui/Skeleton';
import { Modal } from './ui/Modal';

const inputClass =
  'rounded-xl border border-gray-200 px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30';

const selectClass =
  'rounded-lg border border-gray-200 px-2 py-1 text-sm text-gray-900 outline-none transition focus:border-indigo-500 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400';

export function ProjectDetail({ isSuperadmin }: { isSuperadmin: boolean }) {
  const { id } = useParams();
  const projectId = Number(id);
  const { reload: reloadSwitcher } = useProjects();
  const navigate = useNavigate();

  const [project, setProject] = useState<Project | null>(null);
  const [members, setMembers] = useState<ProjectMember[]>([]);
  const [audit, setAudit] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [refreshKey, setRefreshKey] = useState(0);

  const reload = () => setRefreshKey((k) => k + 1);

  // The caller's effective role in this project drives every RBAC gate below.
  const role = project?.role;
  const canManage = role === 'admin' || role === 'superadmin';

  useEffect(() => {
    let active = true;
    (async () => {
      if (!Number.isFinite(projectId)) {
        if (active) {
          setError('Invalid project id');
          setLoading(false);
        }
        return;
      }
      setLoading(true);
      try {
        const projects = await listProjects();
        const proj = projects.find((p) => p.id === projectId) ?? null;
        if (!active) return;
        if (!proj) {
          setError('Project not found');
          setProject(null);
          setLoading(false);
          return;
        }
        const canManageProj = proj.role === 'admin' || proj.role === 'superadmin';
        const [mem, aud] = await Promise.all([
          listProjectMembers(projectId),
          canManageProj ? listProjectAudit(projectId) : Promise.resolve([] as AuditEvent[]),
        ]);
        if (!active) return;
        setProject(proj);
        setMembers(mem);
        setAudit(aud);
        setError('');
      } catch (e) {
        if (active) setError(e instanceof Error ? e.message : 'Failed to load project');
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [projectId, refreshKey]);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <Link
        to="/projects"
        className="inline-flex items-center gap-1 text-sm font-medium text-indigo-700 transition hover:text-indigo-800 dark:text-indigo-400 dark:hover:text-indigo-300"
      >
        <svg
          className="h-4 w-4"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M15 18l-6-6 6-6" />
        </svg>
        Projects
      </Link>

      {loading ? (
        <div className="mt-4">
          <Skeleton className="h-7 w-56" />
          <SkeletonCard className="mt-6">
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
        </div>
      ) : !project ? (
        <p className="mt-6 text-sm text-red-600 dark:text-red-400">
          {error || 'Project not found'}
        </p>
      ) : (
        <>
          <ProjectHeader
            project={project}
            canManage={canManage}
            onChanged={reload}
            onRenamed={reloadSwitcher}
          />

          {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

          <MembersCard
            projectId={projectId}
            members={members}
            canManage={canManage}
            isSuperadmin={isSuperadmin}
            onChanged={setMembers}
          />

          {canManage && <AuditCard events={audit} />}

          {canManage && (
            <DangerZone
              project={project}
              onDeleted={() => {
                reloadSwitcher();
                navigate('/projects');
              }}
            />
          )}
        </>
      )}
    </div>
  );
}

function ProjectHeader({
  project,
  canManage,
  onChanged,
  onRenamed,
}: {
  project: Project;
  canManage: boolean;
  onChanged: () => void;
  onRenamed: () => void;
}) {
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
      onChanged();
      setEditing(false);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Rename failed');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mt-4 flex items-center gap-3">
      <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
        {project.name}
      </h1>
      {canManage && (
        <button
          onClick={open}
          className="rounded-lg px-2.5 py-1 text-sm font-medium text-indigo-700 transition hover:bg-indigo-50 dark:text-indigo-400 dark:hover:bg-indigo-500/15"
        >
          Rename
        </button>
      )}

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
    </div>
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
    <div className="mt-6 rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
      <h2 className="mb-4 text-sm font-semibold text-gray-900 dark:text-gray-50">Members</h2>

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
    </div>
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
    <div className="mt-6 rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
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
    <div className="mt-6 rounded-2xl border border-red-200 bg-white p-5 dark:border-red-500/30 dark:bg-gray-800">
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
