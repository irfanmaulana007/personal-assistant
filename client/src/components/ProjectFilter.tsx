import { MultiSelect, type MultiSelectOption } from './ui/MultiSelect';
import type { Project } from '../types';

const icon = (
  <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"
    />
  </svg>
);

/**
 * Multi-select filter over projects, used on the global dashboard to pick which
 * projects to compare. Values are project ids as strings; an empty selection
 * reads as "All projects". Options preserve the given project order.
 */
export function ProjectFilter({
  projects,
  value,
  onChange,
}: {
  projects: Project[];
  value: string[];
  onChange: (ids: string[]) => void;
}) {
  const options: MultiSelectOption<string>[] = projects.map((p) => ({
    value: String(p.id),
    label: p.name,
  }));
  return (
    <MultiSelect label="projects" icon={icon} options={options} value={value} onChange={onChange} />
  );
}
