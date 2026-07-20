import { Link } from 'react-router-dom';
import { WhatsAppConnectionCard } from './WhatsAppCard';
import { WhatsAppSettings } from './settings/WhatsAppSettings';
import { WhatsAppMappingsSettings } from './settings/WhatsAppMappingsSettings';

// WhatsApp integration detail page. Gathers everything WhatsApp in one place:
// the pairing/connection card plus, for superadmins, the agent-access allowlist
// and the identity mappings that used to live under Settings.
export function IntegrationsWhatsApp({ isAdmin }: { isAdmin: boolean }) {
  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 dark:bg-gray-900 p-6">
      <Link
        to="/integrations"
        className="inline-flex items-center gap-1 text-sm font-medium text-gray-500 transition hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100"
      >
        <svg viewBox="0 0 20 20" fill="currentColor" className="h-4 w-4" aria-hidden="true">
          <path
            fillRule="evenodd"
            d="M12.79 5.23a.75.75 0 0 1-.02 1.06L8.832 10l3.938 3.71a.75.75 0 1 1-1.04 1.08l-4.5-4.25a.75.75 0 0 1 0-1.08l4.5-4.25a.75.75 0 0 1 1.06.02z"
            clipRule="evenodd"
          />
        </svg>
        Integrations
      </Link>

      <div className="mt-2">
        <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
          WhatsApp
        </h1>
        <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
          Pair your WhatsApp account and control how the assistant answers.
        </p>
      </div>

      <div className="mt-6 space-y-6">
        <WhatsAppConnectionCard />
        {isAdmin && <WhatsAppSettings />}
        {isAdmin && <WhatsAppMappingsSettings />}
      </div>
    </div>
  );
}
