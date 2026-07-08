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
    <div className="flex rounded-xl border border-gray-200 bg-white p-0.5">
      {options.map((o) => (
        <button
          key={o.value || 'all'}
          onClick={() => onChange(o.value)}
          className={`rounded-lg px-3 py-1.5 text-sm font-medium transition ${
            value === o.value
              ? 'bg-indigo-100 text-indigo-700'
              : 'text-gray-500 hover:text-gray-900'
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
