import { useChat } from '../hooks/useChat';
import { MessageList } from './MessageList';
import { MessageInput } from './MessageInput';

export function Chat() {
  const { messages, sendMessage, loading } = useChat();

  return (
    <>
      <MessageList messages={messages} loading={loading} />
      <MessageInput onSend={sendMessage} disabled={loading} />
    </>
  );
}
