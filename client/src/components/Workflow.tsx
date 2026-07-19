import { RoutinesSettings } from './settings/RoutinesSettings';

// Workflow is the home for recurring, automated work the assistant runs on a
// schedule. Today that's Daily skills (moved out of Settings); more workflow
// types can be added as further sections here.
export function Workflow() {
  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
        Workflow
      </h1>
      <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
        Automate recurring work your assistant runs for you.
      </p>

      <div className="mt-6">
        <RoutinesSettings />
      </div>
    </div>
  );
}
