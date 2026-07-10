import type { Channel } from '../types';

const options: { value: Channel; label: string }[] = [
  { value: '', label: 'All' },
  { value: 'web', label: 'Web' },
  { value: 'whatsapp', label: 'WhatsApp' },
];

export function ChannelFilter({
  value,
  onChange,
}: {
  value: Channel;
  onChange: (c: Channel) => void;
}) {
  return (
    <div className="flex rounded-xl border border-gray-200 bg-white p-0.5 dark:border-gray-700 dark:bg-gray-800">
      {options.map((o) => (
        <button
          key={o.value || 'all'}
          onClick={() => onChange(o.value)}
          className={`rounded-lg px-3 py-1.5 text-sm font-medium transition ${
            value === o.value
              ? 'bg-indigo-100 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300'
              : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-50'
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
