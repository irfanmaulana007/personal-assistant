// Helpers for multi-select filters that persist to the URL as a single
// comma-separated query param (e.g. `?channel=web,whatsapp`). An empty
// selection maps to an empty string so the param is dropped from the URL,
// which the pages read back as "all".

/**
 * Parse a comma-separated query param into a validated list of allowed values,
 * preserving `allowed` order and dropping unknown/duplicate entries.
 */
export function parseFilterList<T extends string>(raw: string | null, allowed: readonly T[]): T[] {
  if (!raw) return [];
  const wanted = new Set(raw.split(',').map((s) => s.trim()));
  return allowed.filter((v) => wanted.has(v));
}

/** Serialize a selection back to a comma-separated string ('' when empty). */
export function serializeFilterList(values: readonly string[]): string {
  return values.join(',');
}
