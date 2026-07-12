import { useEffect, useRef } from 'react';
import { Message } from './Message';
import { TypingIndicator } from './TypingIndicator';
import { loadingPhrase } from '../lib/loadingPhrase';
import type { ChatMessage } from '../types';

interface MessageListProps {
  messages: ChatMessage[];
  loading: boolean;
}

export function MessageList({ messages, loading }: MessageListProps) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const didInitialScroll = useRef(false);

  useEffect(() => {
    // Jump to the bottom instantly on first load (visiting the page); animate
    // only for messages that arrive afterwards.
    bottomRef.current?.scrollIntoView({
      behavior: didInitialScroll.current ? 'smooth' : 'auto',
    });
    if (messages.length > 0) didInitialScroll.current = true;
  }, [messages, loading]);

  if (messages.length === 0 && !loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-white p-8 dark:bg-gray-900">
        <div className="text-center text-gray-400 dark:text-gray-500">
          <svg
            className="w-12 h-12 mx-auto mb-3 opacity-50"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"
            />
          </svg>
          <p className="text-sm">Send a message to get started</p>
          <p className="text-xs mt-1">Try "help" to see what I can do</p>
        </div>
      </div>
    );
  }

  // While loading, the last message is always the user's just-sent message —
  // derive the contextual phrase from it (e.g. "Checking your calendar").
  const lastOutgoing = [...messages].reverse().find((m) => m.direction === 'out');
  const phrase = loadingPhrase(lastOutgoing?.body);

  return (
    <div className="flex-1 overflow-y-auto bg-white px-4 py-6 dark:bg-gray-900">
      {messages.map((msg) => (
        <Message key={msg.id} message={msg} />
      ))}
      {loading && <TypingIndicator phrase={phrase} />}
      <div ref={bottomRef} />
    </div>
  );
}
