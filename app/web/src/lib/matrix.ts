// Shared search filter for the projects-comparison matrix rows, so the /skills
// and /features pages don't reimplement it. Matches the row's name and subtitle
// against the query, case-insensitively.
export function matrixRowMatches(row: { name: string; subtitle?: string }, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return `${row.name} ${row.subtitle ?? ''}`.toLowerCase().includes(q);
}
