import { useNavigate } from 'react-router-dom';
import { usePreferences } from '../contexts/preferences';
import { useProjects } from '../contexts/project';
import { Markdown } from './Markdown';
import type { ChatMessage } from '../types';

// Split an assistant reply into an optional grammar correction (from the English
// Tutor skill, wrapped in [[grammar]]…[[/grammar]]) and the actual reply.
function splitGrammar(body: string): { grammar: string | null; reply: string } {
  const m = body.match(/\[\[grammar\]\]([\s\S]*?)\[\[\/grammar\]\]/i);
  if (!m) return { grammar: null, reply: body };
  const grammar = m[1].trim();
  const reply = body.replace(m[0], '').trim();
  return { grammar: grammar || null, reply };
}

export function Message({ message }: { message: ChatMessage }) {
  const { formatChatTime, assistantName } = usePreferences();
  const { canManageActive, projectPath } = useProjects();
  const navigate = useNavigate();
  const isUser = message.direction === 'out';
  const name = isUser ? 'You' : assistantName;
  const time = message.timestamp ? formatChatTime(message.timestamp) : '';
  const { grammar, reply } = isUser
    ? { grammar: null, reply: message.body }
    : splitGrammar(message.body);

  // The "Log" affordance links a reply bubble to its run detail. Only shown when
  // the run is known (assistant replies with a trace id) and the user may visit
  // the project's Logs page (same gate as the nav item / route guard).
  const showLog = canManageActive && message.runId != null;

  return (
    <div className={`group mb-5 flex flex-col ${isUser ? 'items-end' : 'items-start'}`}>
      <div className={`mb-1 flex items-baseline gap-2 px-1 ${isUser ? 'flex-row-reverse' : ''}`}>
        <span className="text-sm font-semibold text-gray-700 dark:text-gray-200">{name}</span>
        {time && <span className="text-xs text-gray-400 dark:text-gray-500">{time}</span>}
      </div>
      <div
        className={`max-w-[80%] break-words rounded-2xl px-4 py-2.5 text-sm leading-relaxed ${
          isUser
            ? 'whitespace-pre-wrap rounded-tr-sm bg-indigo-100 text-gray-900 dark:bg-indigo-500/15 dark:text-gray-50'
            : 'rounded-tl-sm bg-gray-100 text-gray-900 dark:bg-gray-800 dark:text-gray-50'
        }`}
      >
        {message.image && (
          <img src={message.image} alt="attachment" className="mb-2 max-h-48 w-auto rounded-lg" />
        )}
        {isUser ? (
          message.body
        ) : (
          <>
            {grammar && (
              <div className="mb-2 border-b border-gray-200 pb-2 dark:border-gray-700">
                <Markdown className="text-xs italic text-gray-400 dark:text-gray-500">
                  {grammar}
                </Markdown>
              </div>
            )}
            {reply && <Markdown>{reply}</Markdown>}
            {message.images && message.images.length > 0 && (
              <div className={`flex flex-col gap-2 ${reply ? 'mt-2' : ''}`}>
                {message.images.map((src, i) => (
                  <a key={i} href={src} target="_blank" rel="noreferrer">
                    <img
                      src={src}
                      alt="generated image"
                      className="max-h-80 w-auto rounded-lg border border-gray-200 dark:border-gray-700"
                    />
                  </a>
                ))}
              </div>
            )}
          </>
        )}
      </div>
      {showLog && (
        <button
          type="button"
          onClick={() => navigate(`${projectPath('logs')}?run=${message.runId}`)}
          title="Open this run's log detail"
          className="mt-1 flex items-center gap-1 rounded px-1 text-xs font-medium text-gray-400 opacity-0 transition hover:text-indigo-700 focus-visible:opacity-100 focus-visible:outline-none group-hover:opacity-100 dark:text-gray-500 dark:hover:text-indigo-400"
        >
          <svg className="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4"
            />
          </svg>
          Log
        </button>
      )}
    </div>
  );
}
