// Chat UI contract. The backend (POST /ai/chat, SSE) is built separately: until then the UI talks to `stubTransport`.
// To go live, implement `ChatTransport` over fetch + ReadableStream (event-stream) and pass it to <ChatProvider transport>.
// The UI never builds prompts or calls an LLM itself; the gateway adds identity from the JWT.

import type { Role } from './types';

export interface ChatMessage {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  createdAt: number;
  /** true while the assistant text is still streaming in */
  streaming?: boolean;
}

/** What the page tells the assistant about where the user is (never an authority: the gateway decides access). */
export interface ChatContext {
  path: string;
  /** e.g. the service picked on the system map */
  service?: string;
}

export interface ChatTransport {
  /** Calls `onDelta` with text chunks; resolves when the reply is complete. Must stop when `signal` aborts. */
  send(history: ChatMessage[], context: ChatContext, onDelta: (text: string) => void, signal: AbortSignal): Promise<void>;
}

/** Flip to true when /ai/chat is wired up: the assistant is then shown only when the gateway's feature flags say so. */
export const AI_BACKEND_READY = false;

export const COMING_SOON = 'Tính năng sẽ hoàn thiện sớm. Trợ lý AI đang được xây dựng; bạn sẽ hỏi được ngay tại đây khi hoàn tất.';

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
  async send(_history, _context, onDelta, signal) {
    await sleep(350, signal);
    for (const word of COMING_SOON.split(' ')) {
      onDelta(`${word} `);
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
