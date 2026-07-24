import type { ReactNode } from 'react';
import type { Hike } from '../../types';
import { Modal } from '../ui/Modal';

function formatDate(iso: string): string {
  // An empty hiked_on means the hike was logged without a date.
  if (!iso) return 'No date';
  const d = new Date(iso + 'T00:00:00');
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });
}

// One labelled row in the detail sheet; hidden by the caller when it has nothing
// to show so the modal never renders an empty field.
function DetailRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex gap-4 py-2.5">
      <div className="w-28 shrink-0 text-xs font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
        {label}
      </div>
      <div className="min-w-0 flex-1 text-sm text-gray-900 dark:text-gray-100">{children}</div>
    </div>
  );
}

export function HikeDetailModal({
  hike,
  onClose,
  onEdit,
  onDelete,
  deleting = false,
}: {
  hike: Hike | null;
  onClose: () => void;
  onEdit: (h: Hike) => void;
  onDelete: (h: Hike) => void;
  deleting?: boolean;
}) {
  return (
    <Modal
      open={hike !== null}
      onClose={onClose}
      title={hike?.mountain ?? 'Hike'}
      description={hike ? formatDate(hike.hiked_on) : undefined}
    >
      {hike && (
        <div>
          <div className="divide-y divide-gray-100 dark:divide-gray-700/60">
            <DetailRow label="Trails">
              <div className="space-y-0.5">
                <div>
                  <span className="text-gray-400 dark:text-gray-500">↑ Up</span>{' '}
                  {hike.up_track || <span className="text-gray-400 dark:text-gray-500">—</span>}
                </div>
                <div>
                  <span className="text-gray-400 dark:text-gray-500">↓ Down</span>{' '}
                  {hike.down_track || <span className="text-gray-400 dark:text-gray-500">—</span>}
                </div>
              </div>
            </DetailRow>

            <DetailRow label="Duration">
              {hike.days} day{hike.days === 1 ? '' : 's'}
              {hike.nights > 0 && ` · ${hike.nights} night${hike.nights === 1 ? '' : 's'}`}
            </DetailRow>

            <DetailRow label="Camping">
              {hike.camped ? (
                <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-2.5 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300">
                  ⛺ Camped overnight
                </span>
              ) : (
                <span className="text-gray-500 dark:text-gray-400">Day trip — no camp</span>
              )}
            </DetailRow>

            <DetailRow label="Companions">
              {hike.participants.length > 0 ? (
                <div className="flex flex-wrap gap-1.5">
                  {hike.participants.map((p) => (
                    <span
                      key={p}
                      className="rounded-full bg-indigo-50 px-2.5 py-0.5 text-xs font-medium text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300"
                    >
                      {p}
                    </span>
                  ))}
                </div>
              ) : (
                <span className="text-gray-500 dark:text-gray-400">Solo trip</span>
              )}
            </DetailRow>
          </div>

          <div className="mt-5 flex items-center gap-3 border-t border-gray-200 pt-4 dark:border-gray-700">
            <button
              type="button"
              onClick={() => onEdit(hike)}
              className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600"
            >
              Edit
            </button>
            <button
              type="button"
              disabled={deleting}
              onClick={() => onDelete(hike)}
              className="rounded-xl px-4 py-2.5 text-sm font-medium text-red-600 transition hover:bg-red-50 disabled:opacity-50 dark:text-red-400 dark:hover:bg-red-500/10"
            >
              {deleting ? 'Deleting…' : 'Delete'}
            </button>
            <button
              type="button"
              onClick={onClose}
              className="ml-auto rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800"
            >
              Close
            </button>
          </div>
        </div>
      )}
    </Modal>
  );
}
