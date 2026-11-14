'use client';

import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react';
import { usePathname } from 'next/navigation';
import { stubTransport, type ChatContext, type ChatMessage, type ChatTransport } from '../../lib/ai-chat';

interface ChatState {
  open: boolean;
  messages: ChatMessage[];
  busy: boolean;
  /** service picked on the system map, sent with each question as context */
  service?: string;
  setService: (service?: string) => void;
  openChat: (draft?: string) => void;
  closeChat: () => void;
  toggleChat: () => void;
  /** text to pre-fill in the input (consumed once by the input box) */
  draft: string;
  clearDraft: () => void;
  send: (text: string) => Promise<void>;
  stop: () => void;
  reset: () => void;
}

const ChatContextValue = createContext<ChatState | null>(null);

const newId = () => (typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`);

export function ChatProvider({ children, transport = stubTransport }: { children: ReactNode; transport?: ChatTransport }) {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [busy, setBusy] = useState(false);
  const [service, setService] = useState<string | undefined>();
  const [draft, setDraft] = useState('');
  const abortRef = useRef<AbortController | null>(null);
  const messagesRef = useRef<ChatMessage[]>([]);
  messagesRef.current = messages;

  const stop = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const send = useCallback(
    async (text: string) => {
      const content = text.trim();
      if (!content || abortRef.current) return;
      const user: ChatMessage = { id: newId(), role: 'user', content, createdAt: Date.now() };
      const reply: ChatMessage = { id: newId(), role: 'assistant', content: '', createdAt: Date.now(), streaming: true };
      const history = [...messagesRef.current, user];
      setMessages([...history, reply]);
      setBusy(true);
      const controller = new AbortController();
      abortRef.current = controller;
      const context: ChatContext = { path: pathname, service };
      try {
        await transport.send(history, context, (delta) => {
          setMessages((current) => current.map((m) => (m.id === reply.id ? { ...m, content: m.content + delta } : m)));
        }, controller.signal);
      } catch (err) {
        const aborted = (err as Error).name === 'AbortError';
        setMessages((current) =>
          current.map((m) =>
            m.id === reply.id ? { ...m, content: m.content || (aborted ? '(đã dừng)' : 'Không gửi được câu hỏi, thử lại sau.') } : m
          )
        );
      } finally {
        setMessages((current) => current.map((m) => (m.id === reply.id ? { ...m, streaming: false } : m)));
        abortRef.current = null;
        setBusy(false);
      }
    },
    [pathname, service, transport]
  );

  const value = useMemo<ChatState>(
    () => ({
      open,
      messages,
      busy,
      service,
      setService,
      openChat: (initial) => {
        if (initial) setDraft(initial);
        setOpen(true);
      },
      closeChat: () => setOpen(false),
      toggleChat: () => setOpen((o) => !o),
      draft,
      clearDraft: () => setDraft(''),
      send,
      stop,
      reset: () => {
        abortRef.current?.abort();
        setMessages([]);
      }
    }),
    [open, messages, busy, service, draft, send, stop]
  );

  return <ChatContextValue.Provider value={value}>{children}</ChatContextValue.Provider>;
}

export function useChat(): ChatState {
  const value = useContext(ChatContextValue);
  if (!value) throw new Error('useChat must be used inside <ChatProvider>');
  return value;
}
