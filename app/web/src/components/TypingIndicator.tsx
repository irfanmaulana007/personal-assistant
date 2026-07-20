import { useEffect, useState } from 'react';

interface TypingIndicatorProps {
  // The user's latest message, used to derive the contextual phrase.
  phrase: string;
}

/**
 * The agent "typing" bubble shown while a reply is being generated. Instead of
 * bare animated dots, it shows a contextual phrase derived from the user's
 * message (e.g. "Checking your calendar...") plus a live elapsed-seconds
 * counter so the user can see how long the agent has been working.
 *
 * The component is mounted fresh for each request (MessageList renders it only
 * while `loading` is true), so the counter naturally restarts every reply.
 */
export function TypingIndicator({ phrase }: TypingIndicatorProps) {
  const [seconds, setSeconds] = useState(0);

  useEffect(() => {
    const interval = setInterval(() => {
      setSeconds((s) => s + 1);
    }, 1000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="mb-3 flex justify-start">
      <div className="flex items-center gap-2 rounded-2xl rounded-tl-sm bg-gray-100 px-4 py-2.5 dark:bg-gray-800">
        <div className="flex gap-1" aria-hidden="true">
          <span
            className="w-2 h-2 bg-gray-400 rounded-full animate-bounce dark:bg-gray-600"
            style={{ animationDelay: '0ms' }}
          />
          <span
            className="w-2 h-2 bg-gray-400 rounded-full animate-bounce dark:bg-gray-600"
            style={{ animationDelay: '150ms' }}
          />
          <span
            className="w-2 h-2 bg-gray-400 rounded-full animate-bounce dark:bg-gray-600"
            style={{ animationDelay: '300ms' }}
          />
        </div>
        <span className="text-sm text-gray-500 dark:text-gray-400">{phrase}…</span>
        {seconds > 0 && (
          <span className="text-xs tabular-nums text-gray-400 dark:text-gray-500">{seconds}s</span>
        )}
      </div>
    </div>
  );
}
