import { useState } from 'react';
import { formatTokens } from '../../lib/format';
import { usePreferences } from '../../contexts/preferences';
import { UsageDualLineChart } from '../charts/UsageDualLineChart';
import { ModelBarChart } from '../charts/ModelBarChart';
import { StatTile, Card } from './parts';
import { useDashboard, formatDayLabel } from './util';
import { isImageModel } from '../../types';
import type { UsageModel } from '../../types';

type Lens = 'combined' | 'llm' | 'image';

const LENSES: { key: Lens; label: string }[] = [
  { key: 'combined', label: 'Combined' },
  { key: 'llm', label: 'LLM' },
  { key: 'image', label: 'Image' },
];

// lensTotals sums the per-model breakdown for the chosen lens: all models
// (combined), text LLMs only, or image models only. The server already prices
// each model row, so cost is just their sum.
function lensTotals(byModel: UsageModel[], lens: Lens) {
  const rows = byModel.filter((m) => {
    if (lens === 'llm') return !isImageModel(m.model);
    if (lens === 'image') return isImageModel(m.model);
    return true;
  });
  return rows.reduce(
    (acc, m) => ({
      promptTokens: acc.promptTokens + m.prompt_tokens,
      completionTokens: acc.completionTokens + m.completion_tokens,
      totalTokens: acc.totalTokens + m.total_tokens,
      cost: acc.cost + m.estimated_cost_usd,
    }),
    { promptTokens: 0, completionTokens: 0, totalTokens: 0, cost: 0 },
  );
}

export function Usage() {
  const { stats } = useDashboard();
  const { formatMoney } = usePreferences();
  const s = stats.summary;
  const [lens, setLens] = useState<Lens>('combined');

  const hasImage = stats.by_model.some((m) => isImageModel(m.model));
  const t = lensTotals(stats.by_model, lens);
  // Requests count whole runs; per-request figures always divide by that, so the
  // "cost / req" figure stays comparable across lenses.
  const reqs = s.requests;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-200">
          {lens === 'combined'
            ? 'LLM + image generation'
            : lens === 'llm'
              ? 'LLM only'
              : 'Image generation only'}
        </h2>
        {hasImage && <LensToggle value={lens} onChange={setLens} />}
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatTile
          label="Total tokens"
          value={formatTokens(t.totalTokens)}
          sub={`${formatTokens(t.promptTokens)} in · ${formatTokens(t.completionTokens)} out`}
        />
        <StatTile label="Prompt tokens" value={formatTokens(t.promptTokens)} />
        <StatTile label="Completion tokens" value={formatTokens(t.completionTokens)} />
        <StatTile
          label="Est. cost"
          value={formatMoney(t.cost)}
          sub={stats.cost_partial ? 'excludes unpriced models' : 'estimated'}
        />
        <StatTile
          label="Avg tokens / req"
          value={reqs > 0 ? formatTokens(Math.round(t.totalTokens / reqs)) : '0'}
        />
        <StatTile
          label="Cost / req"
          value={reqs > 0 ? formatMoney(t.cost / reqs) : formatMoney(0)}
        />
      </div>

      <Card title="Tokens & cost over time">
        <UsageDualLineChart
          formatMoney={formatMoney}
          data={stats.by_day.map((d) => ({
            label: formatDayLabel(d.date),
            tokens: d.total_tokens,
            cost: d.estimated_cost_usd,
          }))}
        />
      </Card>

      <Card title="Tokens by model">
        {stats.by_model.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500">No usage in this range yet.</p>
        ) : (
          <ModelBarChart
            formatMoney={formatMoney}
            data={stats.by_model.map((m) => ({
              model: m.model,
              tokens: m.total_tokens,
              cost: m.estimated_cost_usd,
              rateKnown: m.rate_known,
            }))}
          />
        )}
      </Card>
    </div>
  );
}

// LensToggle is a small segmented control switching the token/cost tiles between
// the combined view and the LLM-only / image-only splits.
function LensToggle({ value, onChange }: { value: Lens; onChange: (l: Lens) => void }) {
  return (
    <div className="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-gray-700 dark:bg-gray-900">
      {LENSES.map((l) => (
        <button
          key={l.key}
          onClick={() => onChange(l.key)}
          className={`rounded-md px-3 py-1 text-xs font-medium transition ${
            value === l.key
              ? 'bg-white text-indigo-700 shadow-sm dark:bg-gray-700 dark:text-indigo-300'
              : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'
          }`}
        >
          {l.label}
        </button>
      ))}
    </div>
  );
}
