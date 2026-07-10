import { formatTokens } from '../../lib/format';
import { usePreferences } from '../../contexts/preferences';
import { StatTile, Card } from './parts';
import { useDashboard } from './util';

export function Users() {
  const { stats } = useDashboard();
  const { formatMoney } = usePreferences();
  const users = stats.by_user;

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatTile label="Active users" value={stats.summary.active_users.toLocaleString()} />
        <StatTile label="Requests" value={stats.summary.requests.toLocaleString()} />
        <StatTile label="Total tokens" value={formatTokens(stats.summary.total_tokens)} />
        <StatTile label="Est. cost" value={formatMoney(stats.summary.estimated_cost_usd)} />
      </div>

      <Card title="Top users">
        {users.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500">No usage in this range yet.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-100 text-left text-xs font-medium uppercase tracking-wide text-gray-400 dark:border-gray-800 dark:text-gray-500">
                  <th className="pb-2 font-medium">User</th>
                  <th className="pb-2 text-right font-medium">Requests</th>
                  <th className="pb-2 text-right font-medium">Tokens</th>
                  <th className="pb-2 text-right font-medium">Errors</th>
                  <th className="pb-2 text-right font-medium">Est. cost</th>
                </tr>
              </thead>
              <tbody>
                {users.map((u) => (
                  <tr
                    key={u.user_id}
                    className="border-b border-gray-50 last:border-0 dark:border-gray-800"
                  >
                    <td className="py-2.5 text-gray-800 dark:text-gray-100">
                      {u.name?.trim() || u.email || `User #${u.user_id}`}
                      {u.name?.trim() && u.email && (
                        <span className="ml-2 text-xs text-gray-400 dark:text-gray-500">
                          {u.email}
                        </span>
                      )}
                    </td>
                    <td className="py-2.5 text-right tabular-nums text-gray-700 dark:text-gray-200">
                      {u.requests.toLocaleString()}
                    </td>
                    <td className="py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">
                      {formatTokens(u.total_tokens)}
                    </td>
                    <td className="py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">
                      {u.errors.toLocaleString()}
                    </td>
                    <td className="py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">
                      {formatMoney(u.estimated_cost_usd)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <p className="mt-3 text-xs text-gray-400 dark:text-gray-500">
          WhatsApp runs are attributed to the owner account, so per-user figures are most accurate
          for the web channel.
        </p>
      </Card>
    </div>
  );
}
