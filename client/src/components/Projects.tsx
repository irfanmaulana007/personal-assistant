import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { listProjects, createProject } from '../api/client';
import type { Project } from '../types';
import { useProjects } from '../contexts/project';
import { SkeletonListRow } from './ui/Skeleton';
import { Modal } from './ui/Modal';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:ring-indigo-500/30';

function formatDate(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
}

function RoleBadge({ role }: { role: Project['role'] }) {
  const accent = role === 'member';
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
        accent
          ? 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
          : 'bg-indigo-50 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300'
      }`}
    >
      {role}
    </span>
  );
}

export function Projects({ isSuperadmin }: { isSuperadmin: boolean }) {
  const navigate = useNavigate();
  const { activeProject, setActiveProject, reload: reloadContext } = useProjects();

  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busyId, setBusyId] = useState<number | null>(null);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    let active = true;
    listProjects()
      .then((ps) => {
        if (!active) return;
        setProjects(ps);
        setError('');
      })
      .catch((e) => active && setError(e instanceof Error ? e.message : 'Failed to load projects'))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const reload = async () => {
    setProjects(await listProjects());
  };

  const switchTo = async (id: number) => {
    setBusyId(id);
    setError('');
    try {
      await setActiveProject(id);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to switch project');
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
            Projects
          </h1>
          <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
            Switch between the projects you can access, or open one to manage its members and
            settings.
          </p>
        </div>
        {isSuperadmin && (
          <button
            type="button"
            onClick={() => setCreating(true)}
            className="shrink-0 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-500 dark:bg-indigo-500 dark:hover:bg-indigo-400"
          >
            + New project
          </button>
        )}
      </div>

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      <Modal
        open={creating}
        onClose={() => setCreating(false)}
        title="New project"
        description="Create a new project and optionally assign its first admin."
      >
        <NewProjectForm
          onCancel={() => setCreating(false)}
          onCreate={async (name, adminEmail) => {
            await createProject(name, adminEmail);
            await reload();
            await reloadContext();
            setCreating(false);
          }}
        />
      </Modal>

      {loading ? (
        <div className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <SkeletonListRow key={i} trailingWidth="w-20" />
          ))}
        </div>
      ) : projects.length === 0 ? (
        <div className="mt-6 rounded-2xl border border-dashed border-gray-300 bg-white p-10 text-center dark:border-gray-700 dark:bg-gray-800">
          <p className="text-sm font-medium text-gray-900 dark:text-gray-50">No projects yet</p>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {isSuperadmin
              ? 'Create your first project to get started.'
              : 'You don’t have access to any projects yet. Ask an admin to add you.'}
          </p>
        </div>
      ) : (
        <div className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {projects.map((p) => {
            const isActive = activeProject?.id === p.id;
            return (
              <div
                key={p.id}
                className="flex flex-col rounded-2xl border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <h2 className="truncate text-sm font-semibold text-gray-900 dark:text-gray-50">
                        {p.name}
                      </h2>
                      {isActive && (
                        <span className="inline-flex shrink-0 items-center rounded-full bg-green-50 px-2 py-0.5 text-xs font-medium text-green-700 dark:bg-green-500/15 dark:text-green-300">
                          Active
                        </span>
                      )}
                    </div>
                    <div className="mt-1.5 flex flex-wrap items-center gap-2">
                      <RoleBadge role={p.role} />
                      <span className="text-xs text-gray-500 dark:text-gray-400">
                        {p.member_count} member{p.member_count === 1 ? '' : 's'}
                      </span>
                    </div>
                  </div>
                </div>

                <p className="mt-3 text-xs text-gray-500 dark:text-gray-400">
                  Created {formatDate(p.created_at)}
                </p>

                <div className="mt-4 flex items-center gap-3 border-t border-gray-100 pt-3 dark:border-gray-800">
                  <button
                    type="button"
                    onClick={() => navigate(`/projects/${p.id}`)}
                    className="text-sm font-medium text-indigo-600 hover:text-indigo-700 dark:text-indigo-400 dark:hover:text-indigo-300"
                  >
                    Open
                  </button>
                  {!isActive && (
                    <button
                      type="button"
                      disabled={busyId === p.id}
                      onClick={() => switchTo(p.id)}
                      className="text-sm font-medium text-gray-600 hover:text-gray-900 disabled:opacity-50 dark:text-gray-300 dark:hover:text-gray-100"
                    >
                      {busyId === p.id ? 'Switching…' : 'Switch to'}
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function NewProjectForm({
  onCreate,
  onCancel,
}: {
  onCreate: (name: string, adminEmail?: string) => Promise<void>;
  onCancel: () => void;
}) {
  const [name, setName] = useState('');
  const [adminEmail, setAdminEmail] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setSaving(true);
    setError('');
    try {
      await onCreate(name.trim(), adminEmail.trim() || undefined);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create project');
    } finally {
      setSaving(false);
    }
  };

  return (
    <form onSubmit={submit}>
      <div className="space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Project name
          </label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Acme Team"
            autoFocus
            className={inputClass}
          />
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Admin email{' '}
            <span className="font-normal text-gray-400 dark:text-gray-500">(optional)</span>
          </label>
          <input
            type="email"
            value={adminEmail}
            onChange={(e) => setAdminEmail(e.target.value)}
            placeholder="admin@email.com"
            className={inputClass}
          />
          <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
            The initial project admin. Leave blank to make yourself admin.
          </p>
        </div>
      </div>

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      <div className="mt-5 flex items-center gap-3">
        <button
          type="submit"
          disabled={saving || !name.trim()}
          className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-400"
        >
          {saving ? 'Creating…' : 'Create project'}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
