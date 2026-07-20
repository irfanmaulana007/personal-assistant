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

    // In stream mode the assistant bubble is created on the first token and
    // updated in place; in block mode no delta fires and it's added on completion.
    const assistantId = nextId();
    let streaming = false;

    try {
      const res = await apiSendMessage(text, image, {
        onDelta: (full) => {
          setMessages((prev) => {
            if (!streaming) {
              streaming = true;
              return [
                ...prev,
                {
                  id: assistantId,
                  direction: 'in',
                  body: full,
                  timestamp: new Date().toISOString(),
                },
              ];
            }
            return prev.map((m) => (m.id === assistantId ? { ...m, body: full } : m));
          });
        },
      });
      // Reconcile to the authoritative final reply (and attach any images).
      const finalMsg: ChatMessage = {
        id: assistantId,
        direction: 'in',
        body: res.response,
        timestamp: new Date().toISOString(),
        images: res.images,
      };
      setMessages((prev) =>
        prev.some((m) => m.id === assistantId)
          ? prev.map((m) => (m.id === assistantId ? finalMsg : m))
          : [...prev, finalMsg],
      );
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
