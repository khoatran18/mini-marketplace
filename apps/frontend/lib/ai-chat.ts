// Chat UI contract. Backend: POST /ai/chat (SSE) on the gateway; the stub transport is only for NEXT_PUBLIC_AI_UI_PREVIEW=true.
// The UI never builds prompts or calls an LLM itself; the gateway adds identity from the JWT.

import { getBaseUrl } from './api';
import type { Role } from './types';

/** A data card the agent produced from a read-only tool (full data goes to the UI only, never through the model). */
export interface ChatCard {
  tool: string;
  data: unknown;
}

/** A write the agent proposed. Nothing has happened yet: the user confirms, then the UI replays `request` with their own token. */
export interface ChatProposal {
  id: string;
  /** signed proposal (sent back as X-Agent-Proposal) */
  token: string;
  summary: string;
  /** epoch ms; the gateway refuses it after this (5 minutes) */
  expiresAt: number;
  request: { method: string; path: string; body: Record<string, unknown> | null };
  state: 'pending' | 'confirming' | 'done' | 'failed' | 'dismissed';
  error?: string;
}

export interface ChatMessage {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  createdAt: number;
  /** true while the assistant text is still streaming in */
  streaming?: boolean;
  /** friendly label while a tool runs ("Đang tra cứu đơn hàng…") */
  status?: string;
  cards?: ChatCard[];
  proposals?: ChatProposal[];
  /** friendly source names ("Tìm kiếm sản phẩm"), shown small under the answer; never API routes */
  sources?: string[];
  error?: string;
}

/** What the page tells the assistant about where the user is (never an authority: the gateway decides access). */
export interface ChatContext {
  path: string;
  /** e.g. the service picked on the system map */
  service?: string;
}

/** Events of the SSE contract (docs/ai/08_AGENT_BUILD_ROADMAP.md). */
export type ChatEvent =
  | { event: 'start'; data: { run_id: string; conversation_id: string } }
  | { event: 'token'; data: { text: string } }
  | { event: 'tool_call'; data: { tool: string; label: string } }
  | { event: 'tool_result'; data: { tool: string; ok: boolean } }
  | { event: 'card'; data: { tool: string; kind: string; data: unknown } }
  | { event: 'citation'; data: { tool: string; label: string } }
  | {
      event: 'action_proposal';
      data: { proposal: string; id: string; expires_at?: string; summary: string; request: { method: string; path: string; body: Record<string, unknown> | null } };
    }
  | { event: 'error'; data: { code: string; message: string } }
  | { event: 'done'; data: { conversation_id: string } };

export interface ChatRequest {
  message: string;
  conversationId?: string;
  clientMessageId: string;
  context: ChatContext;
}

export interface ChatTransport {
  /** Calls `onEvent` for every event; resolves when the stream ends. Must stop when `signal` aborts. */
  send(request: ChatRequest, onEvent: (event: ChatEvent) => void, signal: AbortSignal): Promise<void>;
}

/** Build-time switch: show the assistant with the fixed "coming soon" reply, without the backend. */
export const AI_UI_PREVIEW = process.env.NEXT_PUBLIC_AI_UI_PREVIEW === 'true';

/** The assistant is shown only when the gateway's feature flags say so (preview mode aside). */
export const AI_BACKEND_READY = !AI_UI_PREVIEW;

export const COMING_SOON = 'Tính năng sẽ hoàn thiện sớm. Trợ lý AI đang được xây dựng; bạn sẽ hỏi được ngay tại đây khi hoàn tất.';

export class ChatHttpError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = 'ChatHttpError';
    this.status = status;
  }
}

/** Splits an SSE byte stream into events. Exported for tests. Handles chunks that cut an event in the middle. */
export async function* parseSse(body: ReadableStream<Uint8Array>): AsyncGenerator<{ event: string; data: string }> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, '\n');
      let cut: number;
      while ((cut = buffer.indexOf('\n\n')) >= 0) {
        const block = buffer.slice(0, cut);
        buffer = buffer.slice(cut + 2);
        let event = 'message';
        const data: string[] = [];
        for (const line of block.split('\n')) {
          if (line.startsWith('event:')) event = line.slice(6).trim();
          else if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''));
        }
        if (data.length) yield { event, data: data.join('\n') };
      }
    }
  } finally {
    reader.releaseLock();
  }
}

const HTTP_MESSAGES: Record<number, string> = {
  401: 'Phiên đăng nhập đã hết hạn, hãy đăng nhập lại.',
  404: 'Trợ lý AI đang tắt.',
  429: 'Bạn hỏi hơi nhanh, đợi một chút rồi thử lại.',
  503: 'Trợ lý tạm thời không khả dụng.'
};

/** Replays the confirmed write with the USER's token + the signed proposal. The gateway checks user, method, path, body and single use. */
export async function confirmProposal(proposal: ChatProposal, token: string | null): Promise<void> {
  const { method, path, body } = proposal.request;
  const response = await fetch(`${getBaseUrl()}${path}`, {
    method,
    cache: 'no-store',
    headers: { 'Content-Type': 'application/json', 'X-Agent-Proposal': proposal.token, ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    ...(body ? { body: JSON.stringify(body) } : {})
  });
  if (response.ok) return;
  let detail = '';
  try {
    const payload = (await response.json()) as { error?: string; message?: string };
    detail = payload.error ?? payload.message ?? '';
  } catch {
    /* not JSON */
  }
  const known: Record<number, string> = {
    401: 'Đề xuất đã hết hạn hoặc phiên đăng nhập hết hạn. Hãy yêu cầu lại.',
    403: 'Đề xuất không khớp hoặc đã được dùng. Hãy yêu cầu lại.',
    503: 'Chưa xác nhận được lúc này, thử lại sau.'
  };
  throw new ChatHttpError(known[response.status] ?? (detail || 'Không thực hiện được.'), response.status);
}

/** Real transport: POST /ai/chat with the user's JWT, reads the SSE stream. `getToken` should refresh an expired token. */
export function sseTransport(getToken: () => Promise<string | null>): ChatTransport {
  return {
    async send(request, onEvent, signal) {
      const token = await getToken();
      const response = await fetch(`${getBaseUrl()}/ai/chat`, {
        method: 'POST',
        signal,
        cache: 'no-store',
        headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
        body: JSON.stringify({
          message: request.message,
          conversation_id: request.conversationId,
          client_message_id: request.clientMessageId,
          page_context: request.context
        })
      });
      const type = response.headers.get('content-type') ?? '';
      if (!response.ok || !type.includes('text/event-stream') || !response.body) {
        let detail = '';
        try {
          const payload = (await response.json()) as { error?: string; message?: string };
          detail = payload.error ?? payload.message ?? '';
        } catch {
          /* not JSON */
        }
        throw new ChatHttpError(HTTP_MESSAGES[response.status] ?? (detail || 'Không gửi được câu hỏi.'), response.status);
      }
      for await (const frame of parseSse(response.body)) {
        try {
          onEvent({ event: frame.event, data: JSON.parse(frame.data) } as ChatEvent);
        } catch {
          /* ignore a malformed frame, keep reading */
        }
      }
    }
  };
}

const sleep = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve, reject) => {
    const id = setTimeout(resolve, ms);
    signal.addEventListener('abort', () => {
      clearTimeout(id);
      reject(new DOMException('aborted', 'AbortError'));
    });
  });

/** Placeholder transport: streams the fixed notice word by word so the UI behaves like the real thing. */
export const stubTransport: ChatTransport = {
  async send(_request, onEvent, signal) {
    await sleep(350, signal);
    for (const word of COMING_SOON.split(' ')) {
      onEvent({ event: 'token', data: { text: `${word} ` } });
      await sleep(35, signal);
    }
  }
};

export type AssistantAudience = 'shop' | 'ops';

export function audienceOf(role: Role | null): AssistantAudience | null {
  if (!role) return null;
  return role === 'admin' || role === 'dev' ? 'ops' : 'shop';
}

export function suggestionsFor(role: Role | null): string[] {
  switch (role) {
    case 'admin':
      return ['Hệ thống đang có sự cố nào mở?', 'Tóm tắt tình trạng các service', 'Đề xuất nào đang chờ duyệt?'];
    case 'dev':
      return ['Service nào đang chậm nhất?', 'Có dòng outbox DEAD không?', 'Tóm tắt cảnh báo đang bắn'];
    case 'seller_admin':
    case 'seller_employee':
      return ['Sản phẩm nào sắp hết hàng?', 'Tóm tắt doanh thu hôm nay', 'Đơn nào cần giao trước?'];
    case 'buyer':
      return ['Đơn hàng của tôi đang ở đâu?', 'Chính sách đổi trả thế nào?', 'Gợi ý sản phẩm cho tôi'];
    default:
      return [];
  }
}
