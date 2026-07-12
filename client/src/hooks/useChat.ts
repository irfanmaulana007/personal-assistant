import { useState, useCallback, useEffect } from 'react';
import { sendMessage as apiSendMessage, getChatHistory } from '../api/client';
import type { ChatMessage } from '../types';

let messageId = 0;
function nextId(): string {
  return String(++messageId);
}

export function useChat() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [loading, setLoading] = useState(false);

  // Load history on mount
  useEffect(() => {
    getChatHistory()
      .then((history) => {
        const msgs: ChatMessage[] = history.map((entry) => ({
          id: nextId(),
          direction: entry.direction === 'in' ? 'out' : 'in', // flip: server "in" = user sent, display as "out" (right side)
          body: entry.body,
          timestamp: entry.timestamp,
        }));
        setMessages(msgs);
      })
      .catch(() => {
        // Ignore history load failures
      });
  }, []);

  const sendMessage = useCallback(async (text: string, image?: string) => {
    const userMsg: ChatMessage = {
      id: nextId(),
      direction: 'out',
      body: text,
      timestamp: new Date().toISOString(),
      image,
    };
    setMessages((prev) => [...prev, userMsg]);
    setLoading(true);

    try {
      const res = await apiSendMessage(text, image);
      const assistantMsg: ChatMessage = {
        id: nextId(),
        direction: 'in',
        body: res.response,
        timestamp: new Date().toISOString(),
        images: res.images,
      };
      setMessages((prev) => [...prev, assistantMsg]);
    } catch {
      const errorMsg: ChatMessage = {
        id: nextId(),
        direction: 'in',
        body: 'Sorry, something went wrong. Please try again.',
        timestamp: new Date().toISOString(),
      };
      setMessages((prev) => [...prev, errorMsg]);
    } finally {
      setLoading(false);
    }
  }, []);

  return { messages, sendMessage, loading };
}
