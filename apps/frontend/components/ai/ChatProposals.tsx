'use client';

import { useEffect, useState } from 'react';
import type { ChatProposal } from '../../lib/ai-chat';

function remaining(ms: number) {
  const s = Math.max(0, Math.ceil(ms / 1000));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

/** Confirmation card for one action the agent proposed. Nothing has run until the user presses Xác nhận. */
function ProposalCard({ proposal, onConfirm, onDismiss }: { proposal: ChatProposal; onConfirm: () => void; onDismiss: () => void }) {
  const [now, setNow] = useState(() => Date.now());
  const live = proposal.state === 'pending' || proposal.state === 'confirming';
  useEffect(() => {
    if (!live) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [live]);

  const left = proposal.expiresAt - now;
  const expired = proposal.state === 'pending' && left <= 0;

  return (
    <div className="overflow-hidden rounded-xl border border-brand/40 bg-surface text-sm">
      <p className="border-b border-line bg-brand-soft px-3 py-1.5 text-xs font-semibold text-brand">Đề xuất cần bạn xác nhận</p>
      <div className="grid gap-2 px-3 py-2.5">
        <p className="font-medium text-text">{proposal.summary}</p>
        {proposal.state === 'done' ? (
          <p className="text-xs font-semibold text-success">Đã thực hiện. Tải lại trang để thấy trạng thái mới.</p>
        ) : proposal.state === 'dismissed' ? (
          <p className="text-xs text-muted">Bạn đã bỏ qua đề xuất này.</p>
        ) : expired ? (
          <p className="text-xs text-muted">Đề xuất đã hết hạn. Hãy yêu cầu lại nếu vẫn cần.</p>
        ) : (
          <>
            {proposal.error ? <p className="text-xs text-danger">{proposal.error}</p> : null}
            <div className="flex items-center gap-2">
              <button type="button" className="btn-primary h-8 px-3 text-xs" disabled={proposal.state === 'confirming' || proposal.state === 'failed'} onClick={onConfirm}>
                {proposal.state === 'confirming' ? 'Đang thực hiện…' : 'Xác nhận'}
              </button>
              <button type="button" className="btn h-8 px-3 text-xs" disabled={proposal.state === 'confirming'} onClick={onDismiss}>
                Bỏ qua
              </button>
              {proposal.state !== 'failed' ? <span className="ml-auto text-[11px] text-muted">Hết hạn sau {remaining(left)}</span> : null}
            </div>
          </>
        )}
      </div>
    </div>
  );
}

export function ChatProposals({ proposals, onConfirm, onDismiss }: { proposals: ChatProposal[]; onConfirm: (id: string) => void; onDismiss: (id: string) => void }) {
  return (
    <div className="grid gap-2">
      {proposals.map((p) => (
        <ProposalCard key={p.id} proposal={p} onConfirm={() => onConfirm(p.id)} onDismiss={() => onDismiss(p.id)} />
      ))}
    </div>
  );
}
