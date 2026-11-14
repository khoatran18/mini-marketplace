'use client';

import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react';
import { usePathname } from 'next/navigation';
import { useAuth } from '../auth/AuthProvider';
import { AI_UI_PREVIEW, confirmProposal, sseTransport, stubTransport, type ChatEvent, type ChatMessage, type ChatProposal, type ChatTransport } from '../../lib/ai-chat';

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
  /** user pressed Confirm / Cancel on a proposal card */
  confirm: (messageId: string, proposalId: string) => Promise<void>;
  dismiss: (messageId: string, proposalId: string) => void;
  stop: () => void;
  reset: () => void;
}

const ChatContextValue = createContext<ChatState | null>(null);

const newId = () => (typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`);

const ERROR_TEXT: Record<string, string> = {
  session_expired: 'Phiên làm việc của trợ lý đã hết hạn. Hãy hỏi lại.',
  busy: 'Trợ lý đang xử lý câu hỏi trước của bạn, đợi một chút.',
  timeout: 'Câu hỏi mất quá lâu nên đã dừng. Hãy thử hỏi gọn hơn.',
  budget: 'Câu hỏi này dùng quá nhiều tài nguyên nên đã dừng. Hãy hỏi cụ thể hơn.',
  unavailable: 'Trợ lý tạm thời không khả dụng.'
};

/** Applies one SSE event to the assistant message `id`. */
function applyEvent(m: ChatMessage, e: ChatEvent): ChatMessage {
  switch (e.event) {
    case 'token':
      return { ...m, content: m.content + e.data.text, status: undefined };
    case 'tool_call':
      return { ...m, status: e.data.label };
    case 'tool_result':
      return { ...m, status: undefined };
    case 'card':
      return { ...m, cards: [...(m.cards ?? []), { tool: e.data.tool, data: e.data.data }] };
    case 'citation':
      return (m.sources ?? []).includes(e.data.label) ? m : { ...m, sources: [...(m.sources ?? []), e.data.label] };
    case 'action_proposal': {
      const proposal: ChatProposal = {
        id: e.data.id,
        token: e.data.proposal,
        summary: e.data.summary,
        expiresAt: e.data.expires_at ? Date.parse(e.data.expires_at) : Date.now() + 5 * 60 * 1000,
        request: e.data.request,
        state: 'pending'
      };
      return { ...m, status: undefined, proposals: [...(m.proposals ?? []), proposal] };
    }
    case 'error':
      return { ...m, status: undefined, error: ERROR_TEXT[e.data.code] ?? e.data.message ?? 'Có lỗi khi trả lời.' };
    default:
      return m;
  }
}

export function ChatProvider({ children, transport }: { children: ReactNode; transport?: ChatTransport }) {
  const pathname = usePathname();
  const { getValidAccessToken } = useAuth();
  const conversationRef = useRef<string | undefined>(undefined);
  const activeTransport = useMemo<ChatTransport>(
    () => transport ?? (AI_UI_PREVIEW ? stubTransport : sseTransport(getValidAccessToken)),
    [transport, getValidAccessToken]
  );
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
      try {
        await activeTransport.send(
          { message: content, conversationId: conversationRef.current, clientMessageId: user.id, context: { path: pathname, service } },
          (event) => {
            if (event.event === 'start' || event.event === 'done') conversationRef.current = event.data.conversation_id ?? conversationRef.current;
            setMessages((current) => current.map((m) => (m.id === reply.id ? applyEvent(m, event) : m)));
          },
          controller.signal
        );
      } catch (err) {
        const aborted = (err as Error).name === 'AbortError';
        setMessages((current) =>
          current.map((m) =>
            m.id === reply.id ? { ...m, status: undefined, content: m.content || (aborted ? '(đã dừng)' : ''), error: aborted ? m.error : (err as Error).message || 'Không gửi được câu hỏi, thử lại sau.' } : m
          )
        );
      } finally {
        setMessages((current) => current.map((m) => (m.id === reply.id ? { ...m, streaming: false } : m)));
        abortRef.current = null;
        setBusy(false);
      }
    },
    [pathname, service, activeTransport]
  );

  const patchProposal = useCallback((messageId: string, proposalId: string, patch: Partial<ChatProposal>) => {
    setMessages((current) =>
      current.map((m) => (m.id === messageId ? { ...m, proposals: m.proposals?.map((p) => (p.id === proposalId ? { ...p, ...patch } : p)) } : m))
    );
  }, []);

  const confirm = useCallback(
    async (messageId: string, proposalId: string) => {
      const proposal = messagesRef.current.find((m) => m.id === messageId)?.proposals?.find((p) => p.id === proposalId);
      if (!proposal || proposal.state !== 'pending') return;
      if (Date.now() > proposal.expiresAt) {
        patchProposal(messageId, proposalId, { state: 'failed', error: 'Đề xuất đã hết hạn. Hãy yêu cầu lại.' });
        return;
      }
      patchProposal(messageId, proposalId, { state: 'confirming', error: undefined });
      try {
        await confirmProposal(proposal, await getValidAccessToken());
        patchProposal(messageId, proposalId, { state: 'done' });
      } catch (err) {
        patchProposal(messageId, proposalId, { state: 'failed', error: (err as Error).message || 'Không thực hiện được.' });
      }
    },
    [getValidAccessToken, patchProposal]
  );

  const dismiss = useCallback((messageId: string, proposalId: string) => patchProposal(messageId, proposalId, { state: 'dismissed' }), [patchProposal]);

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
      confirm,
      dismiss,
      stop,
      reset: () => {
        abortRef.current?.abort();
        conversationRef.current = undefined;
        setMessages([]);
      }
    }),
    [open, messages, busy, service, draft, send, confirm, dismiss, stop]
  );

  return <ChatContextValue.Provider value={value}>{children}</ChatContextValue.Provider>;
}

export function useChat(): ChatState {
  const value = useContext(ChatContextValue);
  if (!value) throw new Error('useChat must be used inside <ChatProvider>');
  return value;
}
