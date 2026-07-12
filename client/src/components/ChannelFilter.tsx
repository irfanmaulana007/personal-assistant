import { MultiSelect, type MultiSelectOption } from './ui/MultiSelect';
import type { ChannelValue } from '../types';

const options: MultiSelectOption<ChannelValue>[] = [
  { value: 'web', label: 'Web' },
  { value: 'whatsapp', label: 'WhatsApp' },
];

const icon = (
  <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z"
    />
  </svg>
);

/** Multi-select filter over the message channel(s). Empty selection = all. */
export function ChannelFilter({
  value,
  onChange,
}: {
  value: ChannelValue[];
  onChange: (c: ChannelValue[]) => void;
}) {
  return (
    <MultiSelect label="Channels" icon={icon} options={options} value={value} onChange={onChange} />
  );
}
