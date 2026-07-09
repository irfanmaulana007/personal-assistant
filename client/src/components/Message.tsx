import { usePreferences } from '../contexts/preferences';
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
  const { formatChatTime } = usePreferences();
  const isUser = message.direction === 'out';
  const name = isUser ? 'You' : 'Assistant';
  const time = message.timestamp ? formatChatTime(message.timestamp) : '';
  const { grammar, reply } = isUser
    ? { grammar: null, reply: message.body }
    : splitGrammar(message.body);

  return (
    <div className={`mb-5 flex flex-col ${isUser ? 'items-end' : 'items-start'}`}>
      <div className={`mb-1 flex items-baseline gap-2 px-1 ${isUser ? 'flex-row-reverse' : ''}`}>
        <span className="text-sm font-semibold text-gray-700">{name}</span>
        {time && <span className="text-xs text-gray-400">{time}</span>}
      </div>
      <div
        className={`max-w-[80%] break-words rounded-2xl px-4 py-2.5 text-sm leading-relaxed ${
          isUser
            ? 'whitespace-pre-wrap rounded-tr-sm bg-indigo-100 text-gray-900'
            : 'rounded-tl-sm bg-gray-100 text-gray-900'
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
              <div className="mb-2 border-b border-gray-200 pb-2">
                <Markdown className="text-xs italic text-gray-400">{grammar}</Markdown>
              </div>
            )}
            <Markdown>{reply}</Markdown>
          </>
        )}
      </div>
    </div>
  );
}
