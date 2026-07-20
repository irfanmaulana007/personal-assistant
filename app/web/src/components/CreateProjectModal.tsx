import { useState } from 'react';
import { createProject } from '../api/client';
import { useProjects } from '../contexts/project';
import { Modal } from './ui/Modal';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:ring-indigo-500/30';

// CreateProjectModal is the single entry point for creating a project. It lives
// on its own so any surface (today: the project switcher) can open it, rather
// than being buried in a now-removed Projects list page.
export function CreateProjectModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { reload } = useProjects();

  const [name, setName] = useState('');
  const [adminEmail, setAdminEmail] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const reset = () => {
    setName('');
    setAdminEmail('');
    setError('');
  };

  const close = () => {
    if (saving) return;
    reset();
    onClose();
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setSaving(true);
    setError('');
    try {
      await createProject(name.trim(), adminEmail.trim() || undefined);
      // Refresh the switcher so the new project shows up immediately.
      reload();
      reset();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create project');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={close}
      title="New project"
      description="Create a new project and optionally assign its first admin."
    >
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

        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={close}
            disabled={saving}
            className="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition hover:bg-gray-100 disabled:opacity-50 dark:text-gray-300 dark:hover:bg-gray-800"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={saving || !name.trim()}
            className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-400"
          >
            {saving ? 'Creating…' : 'Create project'}
          </button>
        </div>
      </form>
    </Modal>
  );
}
