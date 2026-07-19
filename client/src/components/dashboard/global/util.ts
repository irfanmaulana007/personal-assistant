import { useOutletContext } from 'react-router-dom';
import type { LineSeries } from '../../charts/MultiLineChart';
import { formatDayLabel } from '../util';
import type {
  Project,
  UsageStats,
  UsageDay,
  UsageModel,
  UsagePlatform,
  UsageUser,
  ToolCount,
} from '../../../types';

export interface PerProject {
  project: Project;
  stats: UsageStats;
}

export interface GlobalDashboardContext {
  // One entry per selected project, each carrying that project's usage stats.
  perProject: PerProject[];
  // Sum of every selected project's stats — powers the aggregate stat tiles and
  // the non-time-series distribution charts.
  aggregate: UsageStats;
}

export function useGlobalDashboard(): GlobalDashboardContext {
  return useOutletContext<GlobalDashboardContext>();
}

// Stable, DOM-safe series key for a project (its id can't collide with the
// 'label' row key used by the charts).
export function seriesKey(projectId: number): string {
  return `p${projectId}`;
}

// pivotByDay turns the per-project daily series into rows keyed by project, one
// row per date across the union of all projects' days, plus a matching series
// list (one line per project) coloured from the categorical palette. Feed both
// straight into <MultiLineChart>.
export function pivotByDay(
  perProject: PerProject[],
  pick: (d: UsageDay) => number,
  palette: string[],
): { rows: ({ label: string } & Record<string, number | string>)[]; series: LineSeries[] } {
  const dates = new Set<string>();
  perProject.forEach((pp) => pp.stats.by_day.forEach((d) => dates.add(d.date)));
  const ordered = [...dates].sort();

  const rows = ordered.map((date) => {
    const row: { label: string } & Record<string, number | string> = {
      label: formatDayLabel(date),
    };
    perProject.forEach((pp) => {
      const day = pp.stats.by_day.find((d) => d.date === date);
      row[seriesKey(pp.project.id)] = day ? pick(day) : 0;
    });
    return row;
  });

  const series: LineSeries[] = perProject.map((pp, i) => ({
    key: seriesKey(pp.project.id),
    name: pp.project.name,
    color: palette[i % palette.length],
  }));

  return { rows, series };
}

// emptyStats is the zero value used when no project is selected / no data.
export function emptyStats(from: string, to: string): UsageStats {
  return {
    from,
    to,
    platform: '',
    summary: {
      requests: 0,
      prompt_tokens: 0,
      completion_tokens: 0,
      total_tokens: 0,
      estimated_cost_usd: 0,
      avg_latency_ms: 0,
      latency_p50_ms: 0,
      latency_p95_ms: 0,
      latency_p99_ms: 0,
      tool_calls: 0,
      errors: 0,
      active_users: 0,
    },
    by_day: [],
    by_model: [],
    by_platform: [],
    top_tools: [],
    by_hour: Array(24).fill(0),
    by_weekday: Array(7).fill(0),
    by_user: [],
    cost_partial: false,
  };
}

// mergeStats sums a set of per-project usage stats into one aggregate. Additive
// metrics (requests, tokens, errors, tool calls) sum directly. Average latency
// is recomputed as a request-weighted mean. Latency percentiles cannot be
// merged from summaries, so they are left at zero (the global Performance tab
// does not show them). Active users are summed across projects and so may
// double-count a user who is active in several projects.
export function mergeStats(perProject: PerProject[], from: string, to: string): UsageStats {
  const out = emptyStats(from, to);
  if (perProject.length === 0) return out;

  const byDay = new Map<string, UsageDay>();
  const byModel = new Map<string, UsageModel>();
  const byPlatform = new Map<string, UsagePlatform>();
  const byTool = new Map<string, number>();
  const byUser = new Map<number, UsageUser>();
  // Weighted latency accumulators (weight = requests with non-zero latency is
  // unknown, so weight by total requests — a good-enough approximation).
  let latencyWeightedSum = 0;
  let latencyWeight = 0;

  for (const { stats } of perProject) {
    const s = stats.summary;
    out.summary.requests += s.requests;
    out.summary.prompt_tokens += s.prompt_tokens;
    out.summary.completion_tokens += s.completion_tokens;
    out.summary.total_tokens += s.total_tokens;
    out.summary.estimated_cost_usd += s.estimated_cost_usd;
    out.summary.tool_calls += s.tool_calls;
    out.summary.errors += s.errors;
    out.summary.active_users += s.active_users;
    out.cost_partial = out.cost_partial || stats.cost_partial;
    if (s.avg_latency_ms > 0 && s.requests > 0) {
      latencyWeightedSum += s.avg_latency_ms * s.requests;
      latencyWeight += s.requests;
    }

    stats.by_hour.forEach((v, i) => (out.by_hour[i] += v));
    stats.by_weekday.forEach((v, i) => (out.by_weekday[i] += v));

    for (const d of stats.by_day) {
      const cur = byDay.get(d.date);
      if (!cur) {
        byDay.set(d.date, { ...d });
      } else {
        // Weight avg latency by requests before overwriting.
        const totalReq = cur.requests + d.requests;
        const weighted =
          totalReq > 0
            ? (cur.avg_latency_ms * cur.requests + d.avg_latency_ms * d.requests) / totalReq
            : 0;
        cur.requests += d.requests;
        cur.errors += d.errors;
        cur.total_tokens += d.total_tokens;
        cur.estimated_cost_usd += d.estimated_cost_usd;
        cur.avg_latency_ms = Math.round(weighted);
      }
    }

    for (const m of stats.by_model) {
      const cur = byModel.get(m.model);
      if (!cur) {
        byModel.set(m.model, { ...m });
      } else {
        cur.requests += m.requests;
        cur.prompt_tokens += m.prompt_tokens;
        cur.completion_tokens += m.completion_tokens;
        cur.total_tokens += m.total_tokens;
        cur.estimated_cost_usd += m.estimated_cost_usd;
        cur.rate_known = cur.rate_known && m.rate_known;
      }
    }

    for (const p of stats.by_platform) {
      const cur = byPlatform.get(p.platform);
      if (!cur) byPlatform.set(p.platform, { ...p });
      else {
        cur.requests += p.requests;
        cur.total_tokens += p.total_tokens;
      }
    }

    for (const t of stats.top_tools) {
      byTool.set(t.tool, (byTool.get(t.tool) ?? 0) + t.count);
    }

    for (const u of stats.by_user) {
      const cur = byUser.get(u.user_id);
      if (!cur) {
        byUser.set(u.user_id, { ...u });
      } else {
        cur.requests += u.requests;
        cur.total_tokens += u.total_tokens;
        cur.errors += u.errors;
        cur.estimated_cost_usd += u.estimated_cost_usd;
      }
    }
  }

  out.summary.avg_latency_ms =
    latencyWeight > 0 ? Math.round(latencyWeightedSum / latencyWeight) : 0;
  out.by_day = [...byDay.values()].sort((a, b) => a.date.localeCompare(b.date));
  out.by_model = [...byModel.values()].sort((a, b) => b.total_tokens - a.total_tokens);
  out.by_platform = [...byPlatform.values()].sort((a, b) => b.requests - a.requests);
  out.top_tools = [...byTool.entries()]
    .map(([tool, count]): ToolCount => ({ tool, count }))
    .sort((a, b) => b.count - a.count);
  out.by_user = [...byUser.values()].sort((a, b) => b.requests - a.requests);

  return out;
}
