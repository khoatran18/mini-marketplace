// RCA (điều tra sự cố, ADR-46/47) và luồng duyệt đề xuất vận hành của ops-service. Mọi lời gọi đi qua gateway bằng JWT của người dùng:
// ai-agent chỉ đọc và soạn nháp; GHI (tạo đề xuất, duyệt, chạy) luôn là người bấm, bằng token của chính họ.
import { apiFetch } from './api';
import { confirmProposal, type ChatProposal } from './ai-chat';

export type RunStatus = 'running' | 'waiting_confirm' | 'waiting_approval' | 'done' | 'error';
export type Outcome = '' | 'recovered' | 'not_recovered' | 'escalated' | 'blocked' | 'rejected' | 'expired' | 'dismissed' | 'failed' | 'budget' | 'error' | 'interrupted' | 'cancelled';

export interface RcaDraft {
  action_id: string;
  params: Record<string, unknown>;
  rationale: string;
  evidence_ids: string[];
  risk?: string | null;
  mode?: string | null;
  escalated: boolean;
}

export interface RcaRun {
  id: number;
  incident_id: number;
  thread_id: string;
  trigger: string;
  status: RunStatus;
  draft: RcaDraft | null;
  proposal_id: number | null;
  waiting_since: number | null;
  outcome: Outcome | null;
  report: { summary?: string; ready_to_close?: boolean; reason?: string } | null;
  created_at: number;
}

export interface RcaEvidence {
  id: string;
  source: string;
  args?: Record<string, unknown>;
  summary: string;
}

export interface RcaHypothesis {
  cause: string;
  confidence: number;
  evidence_ids?: string[];
  refuted_by?: string;
}

export interface RcaView {
  run: RcaRun;
  round: number;
  model_calls: number;
  input_tokens?: number;
  output_tokens?: number;
  queries: number;
  hypotheses: RcaHypothesis[];
  evidence: RcaEvidence[];
  timeline: { at?: number; node?: string; note?: string }[];
  playbook?: string | null;
  report?: RcaRun['report'];
  outcome: string;
}

export interface OpsProposal {
  id: number;
  incident_id?: number;
  action_id: string;
  params?: Record<string, unknown>;
  rationale?: string;
  risk?: string;
  state: 'PENDING' | 'ACCEPTED' | 'REJECTED' | 'NEEDS_REVIEW' | 'ALTERNATIVE' | 'EXPIRED' | 'WITHDRAWN';
  created_by?: string;
  created_at?: string;
  expires_at?: string;
  exec_state?: 'EXECUTING' | 'EXECUTED' | 'FAILED' | null;
  decision_reason?: string;
  decided_by?: string | null;
}

export const ACTIVE: RunStatus[] = ['running', 'waiting_confirm', 'waiting_approval'];

export const STATUS_LABEL: Record<RunStatus, string> = {
  running: 'Đang điều tra',
  waiting_confirm: 'Chờ operator xác nhận',
  waiting_approval: 'Chờ người duyệt / chạy',
  done: 'Xong',
  error: 'Lỗi'
};

export const OUTCOME_LABEL: Record<string, string> = {
  recovered: 'Đã hồi phục',
  not_recovered: 'Chưa hồi phục',
  escalated: 'Chuyển cho người',
  blocked: 'Bị chặn (đã có đủ đề xuất chờ)',
  rejected: 'Đề xuất bị từ chối',
  expired: 'Hết hạn chờ',
  dismissed: 'Operator bỏ nháp',
  failed: 'Hành động thất bại',
  budget: 'Hết ngân sách điều tra',
  error: 'Lỗi',
  interrupted: 'Bị ngắt do khởi động lại',
  cancelled: 'Đã hủy điều tra'
};

export function listRcaRuns(token: string, incidentId?: number): Promise<{ items: RcaRun[] }> {
  return apiFetch(`/ops/rca/runs${incidentId ? `?incident_id=${incidentId}` : '?limit=100'}`, { method: 'GET', token });
}

export function getRcaRun(token: string, id: number): Promise<RcaView> {
  return apiFetch(`/ops/rca/runs/${id}`, { method: 'GET', token });
}

export function startRca(token: string, incidentId: number): Promise<{ run: RcaRun; created: boolean }> {
  return apiFetch('/ops/rca/runs', { method: 'POST', token, body: JSON.stringify({ incident_id: incidentId }) });
}

export function recheckRca(token: string, id: number): Promise<{ run: RcaRun }> {
  return apiFetch(`/ops/rca/runs/${id}/recheck`, { method: 'POST', token });
}

/** Dừng một lần điều tra đang chạy hoặc đang chờ (nút dừng chi phí). Ai bấm được ghi vào nhật ký điều tra. */
export function cancelRca(token: string, id: number): Promise<{ run: RcaRun }> {
  return apiFetch(`/ops/rca/runs/${id}/cancel`, { method: 'POST', token });
}

interface DraftSigned {
  proposal: string;
  id: string;
  expires_at?: string;
  summary: string;
  request: ChatProposal['request'];
}

/** Operator confirms the agent's draft: the gateway signs it (5 min, single use) and the user's own JWT creates the ops proposal. */
export async function confirmRcaDraft(token: string, runId: number): Promise<void> {
  const signed = await apiFetch<DraftSigned>(`/ops/rca/runs/${runId}/proposal`, { method: 'POST', token });
  const proposal: ChatProposal = {
    id: signed.id,
    token: signed.proposal,
    summary: signed.summary,
    expiresAt: signed.expires_at ? Date.parse(signed.expires_at) : Date.now() + 5 * 60_000,
    request: signed.request,
    state: 'confirming'
  };
  await confirmProposal(proposal, token);
}

// ---- ops-service proposals (review -> decision -> execute / executed) ----
export function listIncidentProposals(token: string, incidentId: number): Promise<{ proposals: OpsProposal[] }> {
  return apiFetch(`/ops/proposals?incident_id=${incidentId}&limit=20`, { method: 'GET', token });
}

export function getProposal(token: string, id: number): Promise<{ proposal: OpsProposal; command?: string }> {
  return apiFetch(`/ops/proposals/${id}`, { method: 'GET', token });
}

export async function decideProposal(token: string, id: number, decision: 'accept' | 'reject', reason: string): Promise<void> {
  // review gives the short-lived approval token bound to the proposal's content hash; the decision must carry it
  const review = await apiFetch<{ approval_token: string }>(`/ops/proposals/${id}/review`, { method: 'POST', token });
  await apiFetch(`/ops/proposals/${id}/decision`, { method: 'POST', token, body: JSON.stringify({ decision, reason, approval_token: review.approval_token }) });
}

/** Người TẠO rút lại đề xuất còn chờ duyệt của chính mình (trạng thái WITHDRAWN); người khác bị server từ chối. */
export function withdrawProposal(token: string, id: number, reason = ''): Promise<unknown> {
  return apiFetch(`/ops/proposals/${id}/withdraw`, { method: 'POST', token, body: JSON.stringify({ reason }) });
}

export function executeProposal(token: string, id: number): Promise<unknown> {
  return apiFetch(`/ops/proposals/${id}/execute`, { method: 'POST', token });
}

export function recordExecuted(token: string, id: number, outcome: 'success' | 'failed', note: string): Promise<unknown> {
  return apiFetch(`/ops/proposals/${id}/executed`, { method: 'POST', token, body: JSON.stringify({ outcome, note }) });
}

// ---- nhật ký điều tra (hiện như một đoạn chat) ----
export type RcaEventKind = 'step' | 'tool_call' | 'tool_result' | 'note' | 'hypotheses' | 'draft' | 'report' | 'wait' | 'error';

export interface RcaEvent {
  id: number;
  at: number;
  kind: RcaEventKind;
  title: string;
  data: Record<string, unknown>;
}

export function getRcaEvents(token: string, runId: number, after = 0): Promise<{ status: RunStatus; items: RcaEvent[] }> {
  return apiFetch(`/ops/rca/runs/${runId}/events?after=${after}`, { method: 'GET', token });
}
