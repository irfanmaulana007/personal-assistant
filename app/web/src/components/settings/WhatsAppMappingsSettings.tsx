import { useState, useEffect } from 'react';
import {
  listWhatsAppMappings,
  createWhatsAppMapping,
  updateWhatsAppMapping,
  deleteWhatsAppMapping,
  listProjects,
} from '../../api/client';
import { Modal } from '../ui/Modal';
import { SkeletonFormCard } from '../ui/Skeleton';
import type { WhatsAppMapping, WhatsAppMappingKind, Project } from '../../types';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30';

const selectClass = inputClass;

type Role = WhatsAppMapping['role'];

// The editable payload for the create/edit form — everything the API needs
// except the server-managed id / created_at.
type Draft = Omit<WhatsAppMapping, 'id' | 'created_at'>;

const EMPTY_DRAFT: Draft = {
  jid: '',
  kind: 'group',
  project_id: 0,
  role: 'member',
  user_id: 0,
  label: '',
};

// A group JID can never confer superadmin — a shared chat has no single owner.
// Personal 1:1 chats may map to superadmin.
function rolesFor(kind: WhatsAppMappingKind): Role[] {
  return kind === 'group' ? ['admin', 'member'] : ['superadmin', 'admin', 'member'];
}

function roleLabel(role: Role): string {
  return role.charAt(0).toUpperCase() + role.slice(1);
}

export function WhatsAppMappingsSettings() {
  const [mappings, setMappings] = useState<WhatsAppMapping[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  // The mapping being edited (existing) or created (null id via `formOpen`).
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<WhatsAppMapping | null>(null);
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState('');

  // Delete confirmation + per-row busy tracking.
  const [confirming, setConfirming] = useState<WhatsAppMapping | null>(null);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [rowError, setRowError] = useState<{ id: number; text: string } | null>(null);

  const load = async () => {
    const [ms, ps] = await Promise.all([listWhatsAppMappings(), listProjects()]);
    setMappings(ms);
    setProjects(ps);
  };

  useEffect(() => {
    let active = true;
    Promise.all([listWhatsAppMappings(), listProjects()])
      .then(([ms, ps]) => {
        if (!active) return;
        setMappings(ms);
        setProjects(ps);
      })
      .catch((e) => active && setError(e instanceof Error ? e.message : 'Failed to load'))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const projectName = (id: number): string =>
    projects.find((p) => p.id === id)?.name ?? `Project #${id}`;

  const openCreate = () => {
    setEditing(null);
    setDraft({ ...EMPTY_DRAFT, project_id: projects[0]?.id ?? 0 });
    setFormError('');
    setFormOpen(true);
  };

  const openEdit = (m: WhatsAppMapping) => {
    setEditing(m);
    setDraft({
      jid: m.jid,
      kind: m.kind,
      project_id: m.project_id,
      role: m.role,
      user_id: m.user_id,
      label: m.label,
    });
    setFormError('');
    setFormOpen(true);
  };

  // Keep the draft coherent when kind flips: a group can't keep a superadmin
  // role, and only personal chats carry a user_id.
  const setKind = (kind: WhatsAppMappingKind) => {
    setDraft((d) => ({
      ...d,
      kind,
      role: kind === 'group' && d.role === 'superadmin' ? 'member' : d.role,
      user_id: kind === 'group' ? 0 : d.user_id,
    }));
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError('');
    try {
      const payload: Draft = {
        ...draft,
        jid: draft.jid.trim(),
        label: draft.label.trim(),
        user_id: draft.kind === 'personal' ? draft.user_id : 0,
      };
      if (editing) {
        await updateWhatsAppMapping(editing.id, payload);
      } else {
        await createWhatsAppMapping(payload);
      }
      await load();
      setFormOpen(false);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const confirmDelete = async () => {
    if (!confirming) return;
    const id = confirming.id;
    setBusyId(id);
    setRowError(null);
    try {
      await deleteWhatsAppMapping(id);
      await load();
      setConfirming(null);
    } catch (err) {
      setRowError({ id, text: err instanceof Error ? err.message : 'Failed to delete' });
      setConfirming(null);
    } finally {
      setBusyId(null);
    }
  };

  if (loading) return <SkeletonFormCard fields={3} />;

  const kindBadge = (kind: WhatsAppMappingKind) =>
    kind === 'group'
      ? 'bg-blue-50 text-blue-700 ring-blue-200 dark:bg-blue-500/10 dark:text-blue-300 dark:ring-blue-500/30'
      : 'bg-purple-50 text-purple-700 ring-purple-200 dark:bg-purple-500/10 dark:text-purple-300 dark:ring-purple-500/30';

  const roleBadge = (role: Role) =>
    role === 'superadmin'
      ? 'bg-amber-50 text-amber-700 ring-amber-200 dark:bg-amber-500/10 dark:text-amber-300 dark:ring-amber-500/30'
      : role === 'admin'
        ? 'bg-indigo-50 text-indigo-700 ring-indigo-200 dark:bg-indigo-500/10 dark:text-indigo-300 dark:ring-indigo-500/30'
        : 'bg-gray-100 text-gray-600 ring-gray-200 dark:bg-gray-700 dark:text-gray-300 dark:ring-gray-600';

  const badgeBase =
    'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset';

  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h2 className="text-base font-semibold text-gray-900 dark:text-gray-50">
            WhatsApp identity mappings
          </h2>
          <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
            Decide which project and role the assistant acts as for an inbound WhatsApp message. A
            group chat maps to a project (admin or member); a personal number maps to a project and
            role, optionally attributed to a specific user.
          </p>
        </div>
        <button
          type="button"
          onClick={openCreate}
          className="shrink-0 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-600"
        >
          Add mapping
        </button>
      </div>

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {mappings.length === 0 ? (
        <div className="mt-6 rounded-xl border border-dashed border-gray-300 px-4 py-10 text-center dark:border-gray-600">
          <p className="text-sm font-medium text-gray-900 dark:text-gray-50">No mappings yet</p>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            Add a mapping to route a WhatsApp group or number to a project and role.
          </p>
        </div>
      ) : (
        <div className="mt-5 overflow-hidden rounded-xl border border-gray-200 dark:border-gray-700">
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead className="bg-gray-50 dark:bg-gray-900/50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                    Mapping
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                    Kind
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                    Project
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                    Role
                  </th>
                  <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {mappings.map((m) => (
                  <tr key={m.id} className="align-top">
                    <td className="px-4 py-3">
                      <div className="text-sm font-medium text-gray-900 dark:text-gray-50">
                        {m.label || <span className="text-gray-400 dark:text-gray-500">—</span>}
                      </div>
                      <div className="mt-0.5 break-all font-mono text-xs text-gray-500 dark:text-gray-400">
                        {m.jid}
                      </div>
                      {m.kind === 'personal' && m.user_id > 0 && (
                        <div className="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
                          user #{m.user_id}
                        </div>
                      )}
                      {rowError?.id === m.id && (
                        <p className="mt-1 text-xs text-red-600 dark:text-red-400">
                          {rowError.text}
                        </p>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <span className={`${badgeBase} ${kindBadge(m.kind)}`}>{m.kind}</span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">
                      {projectName(m.project_id)}
                    </td>
                    <td className="px-4 py-3">
                      <span className={`${badgeBase} ${roleBadge(m.role)}`}>
                        {roleLabel(m.role)}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-3">
                        <button
                          type="button"
                          onClick={() => openEdit(m)}
                          disabled={busyId === m.id}
                          className="text-sm font-medium text-indigo-600 hover:text-indigo-700 disabled:cursor-not-allowed disabled:opacity-50 dark:text-indigo-400 dark:hover:text-indigo-300"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            setRowError(null);
                            setConfirming(m);
                          }}
                          disabled={busyId === m.id}
                          className="text-sm font-medium text-red-600 hover:text-red-700 disabled:cursor-not-allowed disabled:opacity-50 dark:text-red-400 dark:hover:text-red-300"
                        >
                          {busyId === m.id ? 'Deleting…' : 'Delete'}
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Create / edit form */}
      <Modal
        open={formOpen}
        onClose={() => setFormOpen(false)}
        title={editing ? 'Edit mapping' : 'Add mapping'}
        description="Route an inbound WhatsApp identity to a project and role."
      >
        <form onSubmit={submit} className="space-y-4">
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              Kind
            </label>
            <select
              value={draft.kind}
              onChange={(e) => setKind(e.target.value as WhatsAppMappingKind)}
              className={selectClass}
            >
              <option value="group">Group</option>
              <option value="personal">Personal</option>
            </select>
            <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
              {draft.kind === 'group'
                ? 'A shared group chat. Can map to admin or member only — never superadmin.'
                : 'A one-to-one chat. May map to superadmin, admin, or member.'}
            </p>
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              JID
            </label>
            <input
              value={draft.jid}
              onChange={(e) => setDraft((d) => ({ ...d, jid: e.target.value }))}
              placeholder={draft.kind === 'group' ? 'group JID' : 'phone number'}
              required
              className={inputClass}
            />
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              Label
            </label>
            <input
              value={draft.label}
              onChange={(e) => setDraft((d) => ({ ...d, label: e.target.value }))}
              placeholder="Friendly name"
              className={inputClass}
            />
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              Project
            </label>
            <select
              value={draft.project_id}
              onChange={(e) => setDraft((d) => ({ ...d, project_id: Number(e.target.value) }))}
              required
              className={selectClass}
            >
              {projects.length === 0 && <option value={0}>No projects available</option>}
              {projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              Role
            </label>
            <select
              value={draft.role}
              onChange={(e) => setDraft((d) => ({ ...d, role: e.target.value as Role }))}
              className={selectClass}
            >
              {rolesFor(draft.kind).map((r) => (
                <option key={r} value={r}>
                  {roleLabel(r)}
                </option>
              ))}
            </select>
          </div>

          {draft.kind === 'personal' && (
            <div>
              <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
                Attribute to user id
              </label>
              <input
                type="number"
                min={0}
                value={draft.user_id}
                onChange={(e) => setDraft((d) => ({ ...d, user_id: Number(e.target.value) }))}
                className={inputClass}
              />
              <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
                Optional — attribute this chat to a user id. Leave 0 for none.
              </p>
            </div>
          )}

          {formError && <p className="text-sm text-red-600 dark:text-red-400">{formError}</p>}

          <div className="flex items-center justify-end gap-3 pt-1">
            <button
              type="button"
              onClick={() => setFormOpen(false)}
              className="rounded-xl border border-gray-200 px-4 py-2.5 text-sm font-medium text-gray-700 transition hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-700"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={saving}
              className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-600"
            >
              {saving ? 'Saving…' : editing ? 'Save changes' : 'Add mapping'}
            </button>
          </div>
        </form>
      </Modal>

      {/* Delete confirmation */}
      <Modal
        open={confirming !== null}
        onClose={() => setConfirming(null)}
        title="Delete mapping"
        description="This WhatsApp identity will no longer route to a project."
      >
        <p className="text-sm text-gray-700 dark:text-gray-200">
          Delete the mapping for{' '}
          <span className="font-medium text-gray-900 dark:text-gray-50">
            {confirming?.label || confirming?.jid}
          </span>
          ? This cannot be undone.
        </p>
        <div className="mt-5 flex items-center justify-end gap-3">
          <button
            type="button"
            onClick={() => setConfirming(null)}
            className="rounded-xl border border-gray-200 px-4 py-2.5 text-sm font-medium text-gray-700 transition hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-700"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={confirmDelete}
            disabled={busyId !== null}
            className="rounded-xl bg-red-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-red-700 disabled:opacity-50 dark:bg-red-500 dark:hover:bg-red-600"
          >
            {busyId !== null ? 'Deleting…' : 'Delete'}
          </button>
        </div>
      </Modal>
    </div>
  );
}
