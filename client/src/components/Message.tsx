import { usePreferences } from '../contexts/preferences';
import type { ChatMessage } from '../types';

export function Message({ message }: { message: ChatMessage }) {
  const { formatDate } = usePreferences();
  const isUser = message.direction === 'out';
  const name = isUser ? 'You' : 'Assistant';
  const time = message.timestamp ? formatDate(message.timestamp, { time: true }) : '';

  return (
    <div className={`mb-5 flex flex-col ${isUser ? 'items-end' : 'items-start'}`}>
      <div className={`mb-1 flex items-baseline gap-2 px-1 ${isUser ? 'flex-row-reverse' : ''}`}>
        <span className="text-sm font-semibold text-gray-700">{name}</span>
        {time && <span className="text-xs text-gray-400">{time}</span>}
      </div>
      <div
        className={`max-w-[80%] whitespace-pre-wrap break-words rounded-2xl px-4 py-2.5 text-sm leading-relaxed ${
          isUser
            ? 'rounded-tr-sm bg-indigo-100 text-gray-900'
            : 'rounded-tl-sm bg-gray-100 text-gray-900'
        }`}
      >
        {message.image && (
          <img src={message.image} alt="attachment" className="mb-2 max-h-48 w-auto rounded-lg" />
        )}
        {message.body}
      </div>
    </div>
  );
}
