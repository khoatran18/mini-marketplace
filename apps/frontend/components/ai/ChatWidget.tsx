'use client';

import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react';
import { useAuth } from '../auth/AuthProvider';
import { audienceOf, suggestionsFor, AI_BACKEND_READY } from '../../lib/ai-chat';
import { useFeatureFlags } from '../../lib/features';
import { Icon } from '../layout/Icon';
import { useChat } from './ChatProvider';

/** True when the signed-in user may see the assistant (flag on for their audience). The gateway enforces access anyway. */
export function useAssistantEnabled(): boolean {
  const { accessToken, role } = useAuth();
  const flags = useFeatureFlags();
  const audience = audienceOf(role);
  if (!accessToken || !audience) return false;
  if (!AI_BACKEND_READY) return true; // UI only for now: the stub transport answers with the "coming soon" notice
  return audience === 'ops' ? flags.ai_ops : flags.ai_chat;
}

function SparkIcon({ className = 'h-5 w-5' }: { className?: string }) {
  return <Icon name="spark" className={className} />;
}

/** Button for the console top bar: opens/closes the assistant drawer. */
export function AssistantButton() {
  const enabled = useAssistantEnabled();
  const { open, toggleChat } = useChat();
  if (!enabled) return null;
  return (
    <button
      type="button"
      onClick={toggleChat}
      aria-pressed={open}
      aria-label="Trợ lý AI"
      data-chat-ui
      title="Trợ lý AI (Ctrl+K)"
      className={`inline-flex h-9 items-center gap-2 rounded-lg border px-3 text-sm font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand ${
        open ? 'border-brand/40 bg-brand-soft text-brand' : 'border-line bg-surface text-text hover:bg-surface2'
      }`}
    >
      <SparkIcon className="h-4 w-4" />
      <span className="hidden sm:inline">Trợ lý AI</span>
    </button>
  );
}

function Panel({ mode }: { mode: 'drawer' | 'floating' }) {
  const { role } = useAuth();
  const { messages, busy, send, stop, reset, closeChat, draft, clearDraft, service } = useChat();
  const [text, setText] = useState('');
  const endRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: 'end' });
  }, [messages]);

  useEffect(() => {
    if (draft) {
      setText(draft);
      clearDraft();
    }
    inputRef.current?.focus();
  }, [draft, clearDraft]);

  const submit = (event?: FormEvent) => {
    event?.preventDefault();
    if (!text.trim() || busy) return;
    void send(text);
    setText('');
  };

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault();
      submit();
    }
  };

  const suggestions = suggestionsFor(role);
  const frame =
    mode === 'drawer'
      ? 'fixed bottom-0 right-0 top-14 z-40 w-full border-l sm:w-[400px]'
      : 'fixed bottom-20 right-4 z-40 h-[560px] max-h-[calc(100vh-7rem)] w-[calc(100vw-2rem)] rounded-2xl border shadow-2xl sm:w-[380px]';

  return (
    <section aria-label="Trợ lý AI" data-chat-ui className={`${frame} flex flex-col border-line bg-surface text-text`}>
      <header className="flex items-center justify-between gap-2 border-b border-line px-4 py-3">
        <div className="flex items-center gap-2">
          <span className="grid h-8 w-8 place-items-center rounded-lg bg-brand-soft text-brand">
            <SparkIcon className="h-4 w-4" />
          </span>
          <div className="leading-tight">
            <p className="text-sm font-bold">Trợ lý AI</p>
            <p className="text-xs text-muted">{service ? `Đang xem: ${service}` : 'Sắp ra mắt'}</p>
          </div>
        </div>
        <div className="flex items-center gap-1">
          {messages.length > 0 ? (
            <button type="button" onClick={reset} className="rounded-md px-2 py-1 text-xs font-semibold text-muted hover:bg-surface2">
              Cuộc mới
            </button>
          ) : null}
          <button type="button" onClick={closeChat} aria-label="Đóng trợ lý" className="rounded-md px-2 py-1 text-lg leading-none text-muted hover:bg-surface2">
            ×
          </button>
        </div>
      </header>

      <div className="flex-1 space-y-3 overflow-y-auto px-4 py-4" aria-live="polite">
        {messages.length === 0 ? (
          <div className="grid gap-3">
            <p className="text-sm text-muted">Xin chào! Hỏi mình bất cứ điều gì. Phần trả lời thật đang được xây dựng, nên hiện tại bạn chỉ thấy thông báo &quot;sắp ra mắt&quot;.</p>
            {suggestions.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {suggestions.map((s) => (
                  <button key={s} type="button" onClick={() => void send(s)} className="rounded-full border border-line bg-surface2 px-3 py-1.5 text-left text-xs font-medium text-text transition hover:border-brand/50 hover:bg-brand-soft">
                    {s}
                  </button>
                ))}
              </div>
            ) : null}
          </div>
        ) : (
          messages.map((m) => (
            <div key={m.id} className={`flex ${m.role === 'user' ? 'justify-end' : 'justify-start'}`}>
              <p
                className={`max-w-[85%] whitespace-pre-wrap rounded-2xl px-3.5 py-2 text-sm leading-relaxed ${
                  m.role === 'user' ? 'rounded-br-md bg-brand-solid text-white' : 'rounded-bl-md bg-surface2 text-text'
                }`}
              >
                {m.content || (m.streaming ? <span className="inline-flex gap-1" aria-label="Đang trả lời"><i className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted" /><i className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted [animation-delay:120ms]" /><i className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted [animation-delay:240ms]" /></span> : null)}
              </p>
            </div>
          ))
        )}
        <div ref={endRef} />
      </div>

      <form onSubmit={submit} className="grid gap-2 border-t border-line p-3">
        <div className="flex items-end gap-2">
          <textarea
            ref={inputRef}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={onKeyDown}
            rows={1}
            maxLength={2000}
            placeholder="Nhập câu hỏi…"
            aria-label="Câu hỏi cho trợ lý"
            className="max-h-32 min-h-[40px] resize-none rounded-lg py-2 text-sm"
          />
          {busy ? (
            <button type="button" onClick={stop} className="btn h-10 shrink-0 px-3">
              Dừng
            </button>
          ) : (
            <button type="submit" disabled={!text.trim()} className="btn-primary h-10 shrink-0 px-3">
              Gửi
            </button>
          )}
        </div>
        <p className="text-[11px] text-muted">Enter để gửi, Shift+Enter xuống dòng. Trợ lý có thể sai; hãy kiểm tra số liệu quan trọng.</p>
      </form>
    </section>
  );
}

/** Mounts the assistant: `drawer` docks to the right under the console top bar, `floating` is the storefront bubble. */
export function ChatWidget({ mode }: { mode: 'drawer' | 'floating' }) {
  const enabled = useAssistantEnabled();
  const { open, toggleChat, closeChat } = useChat();

  // Ctrl/Cmd+K toggles, Esc closes
  useEffect(() => {
    if (!enabled) return;
    const onKey = (event: globalThis.KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        toggleChat();
      } else if (event.key === 'Escape') {
        closeChat();
      }
    };
    // Storefront bubble: a click anywhere outside the panel closes it. The console drawer sits beside the page (it
    // pushes the content), so there a click on the page must keep working and the drawer stays until closed.
    const onPointerDown = (event: PointerEvent) => {
      if (mode === 'floating' && open && event.target instanceof Element && !event.target.closest('[data-chat-ui]')) closeChat();
    };
    window.addEventListener('keydown', onKey);
    window.addEventListener('pointerdown', onPointerDown);
    return () => {
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('pointerdown', onPointerDown);
    };
  }, [enabled, mode, open, toggleChat, closeChat]);

  if (!enabled) return null;
  return (
    <>
      {open ? <Panel mode={mode} /> : null}
      {mode === 'floating' ? (
        <button
          type="button"
          onClick={toggleChat}
          aria-label={open ? 'Đóng trợ lý AI' : 'Mở trợ lý AI'}
          aria-expanded={open}
          data-chat-ui
          className="fixed bottom-4 right-4 z-40 grid h-12 w-12 place-items-center rounded-full bg-brand-solid text-white shadow-lg transition hover:scale-105 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand"
        >
          <SparkIcon className="h-6 w-6" />
        </button>
      ) : null}
    </>
  );
}
