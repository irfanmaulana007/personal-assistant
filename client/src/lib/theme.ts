// Theme handling: light / dark / auto (follow system), persisted to localStorage.
//
// The class `dark` is toggled on <html>; Tailwind's `dark:` variant (see
// index.css) keys off it. A tiny inline script in index.html applies the same
// class before first paint to avoid a flash — keep STORAGE_KEY in sync with it.

export type ThemeChoice = 'light' | 'dark' | 'auto';

export const STORAGE_KEY = 'pa-theme';

const DARK_QUERY = '(prefers-color-scheme: dark)';

export function getStoredTheme(): ThemeChoice {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === 'light' || v === 'dark' || v === 'auto') return v;
  } catch {
    // localStorage unavailable (private mode, etc.) — fall through to default.
  }
  return 'auto';
}

function systemPrefersDark(): boolean {
  return window.matchMedia(DARK_QUERY).matches;
}

/** The concrete appearance a choice resolves to right now. */
export function resolveTheme(choice: ThemeChoice): 'light' | 'dark' {
  if (choice === 'auto') return systemPrefersDark() ? 'dark' : 'light';
  return choice;
}

function apply(choice: ThemeChoice): void {
  const dark = resolveTheme(choice) === 'dark';
  document.documentElement.classList.toggle('dark', dark);
}

/** Persist and immediately apply a theme choice. */
export function setTheme(choice: ThemeChoice): void {
  try {
    localStorage.setItem(STORAGE_KEY, choice);
  } catch {
    // Ignore write failures; the choice still applies for this session.
  }
  apply(choice);
}

/**
 * Apply the stored theme and keep it in sync with the OS when set to `auto`.
 * Call once at app startup.
 */
export function initTheme(): void {
  apply(getStoredTheme());
  window.matchMedia(DARK_QUERY).addEventListener('change', () => {
    if (getStoredTheme() === 'auto') apply('auto');
  });
}
