// Shared on/off switch used across Skills, Reminders, etc.
export function Toggle({
  on,
  busy,
  disabled,
  onClick,
}: {
  on: boolean;
  busy?: boolean;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      disabled={busy || disabled}
      onClick={onClick}
      className={`relative mt-0.5 inline-flex h-6 w-11 shrink-0 items-center rounded-full transition disabled:opacity-50 ${
        on ? 'bg-indigo-600' : 'bg-gray-200 dark:bg-gray-600'
      }`}
    >
      <span
        className={`inline-block h-5 w-5 transform rounded-full bg-white shadow transition ${
          on ? 'translate-x-5' : 'translate-x-0.5'
        }`}
      />
    </button>
  );
}
