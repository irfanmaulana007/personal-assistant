import { MultiSelect, type MultiSelectOption } from './ui/MultiSelect';
import type { ScoreValue } from '../types';

const options: MultiSelectOption<ScoreValue>[] = [
  { value: 'low', label: 'Low' },
  { value: 'scored', label: 'Scored' },
  { value: 'unscored', label: 'Unscored' },
];

const icon = (
  <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.196-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.783-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z"
    />
  </svg>
);

/** Multi-select filter over the LLM-judge verdict. Empty selection = all. */
export function ScoreFilter({
  value,
  onChange,
}: {
  value: ScoreValue[];
  onChange: (s: ScoreValue[]) => void;
}) {
  return (
    <MultiSelect label="Quality" icon={icon} options={options} value={value} onChange={onChange} />
  );
}
