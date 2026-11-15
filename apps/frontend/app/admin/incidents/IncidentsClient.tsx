'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useAuth } from '../../../components/auth/AuthProvider';
import { EmptyBlock, ErrorBlock, LoadingBlock } from '../../../components/ui/StateBlock';
import { listOpsIncidentsRequest } from '../../../lib/api';
import { formatDateTime } from '../../../lib/format';
import { useFeatureFlags } from '../../../lib/features';
import {
  ACTIVE,
  OUTCOME_LABEL,
  STATUS_LABEL,
  confirmRcaDraft,
  decideProposal,
  executeProposal,
  getRcaRun,
  listIncidentProposals,
  listRcaRuns,
  recheckRca,
  recordExecuted,
  startRca,
  type OpsProposal,
  type RcaRun,
  type RcaView
} from '../../../lib/rca';
import type { OpsIncident } from '../../../lib/types';

const SEVERITY_TONE: Record<string, string> = { critical: 'bg-danger-soft text-danger', warning: 'bg-warning-soft text-warning' };
const RUN_TONE: Record<string, string> = {
  running: 'bg-brand-soft text-brand',
  waiting_confirm: 'bg-warning-soft text-warning',
  waiting_approval: 'bg-warning-soft text-warning',
  done: 'bg-success-soft text-success',
  error: 'bg-danger-soft text-danger'
};
const REFRESH_MS = 5000;

function Pill({ tone, children }: { tone: string; children: React.ReactNode }) {
  return <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ${tone}`}>{children}</span>;
}

/** Access token as a string ('' when signed out: the gateway answers 401 and the message shows). */
function useToken(): () => Promise<string> {
  const { getValidAccessToken } = useAuth();
  return useCallback(async () => (await getValidAccessToken()) ?? '', [getValidAccessToken]);
}

function errMsg(err: unknown, fallback: string): string {
  return (err as Error)?.message || fallback;
}

export function IncidentsClient() {
  const { role } = useAuth();
  const getValidAccessToken = useToken();
  const flags = useFeatureFlags();
  const [state, setState] = useState<'open' | 'investigating' | 'resolved'>('open');
  const [incidents, setIncidents] = useState<OpsIncident[] | null>(null);
  const [runs, setRuns] = useState<Record<number, RcaRun>>({});
  const [selected, setSelected] = useState<number | null>(null);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const load = useCallback(async () => {
    try {
      const token = await getValidAccessToken();
      const list = await listOpsIncidentsRequest(token, state, 50);
      setIncidents(list.incidents ?? []);
      if (flags.ai_ops) {
        const latest: Record<number, RcaRun> = {};
        for (const r of (await listRcaRuns(token)).items) if (!latest[r.incident_id]) latest[r.incident_id] = r; // newest first
        setRuns(latest);
      }
      setError('');
    } catch (err) {
      setError(errMsg(err, 'Không tải được danh sách sự cố.'));
      setIncidents((cur) => cur ?? []);
    }
  }, [getValidAccessToken, state, flags.ai_ops]);

  useEffect(() => {
    void load();
  }, [load]);

  const anyActive = useMemo(() => Object.values(runs).some((r) => ACTIVE.includes(r.status)), [runs]);
  useEffect(() => {
    if (!anyActive) return;
    const t = setInterval(() => void load(), REFRESH_MS);
    return () => clearInterval(t);
  }, [anyActive, load]);

  const investigate = async (incidentId: number) => {
    setNotice('');
    try {
      const out = await startRca(await getValidAccessToken(), incidentId);
      setNotice(out.created ? 'Đã bắt đầu điều tra (chạy nền, vài phút).' : 'Sự cố này đang có một phiên điều tra chưa xong.');
      setSelected(incidentId);
      await load();
    } catch (err) {
      setNotice(errMsg(err, 'Không bắt đầu điều tra được.'));
    }
  };

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]">
      <section className="card grid content-start gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <label className="grid gap-1 text-xs font-semibold text-muted">
            Trạng thái sự cố
            <select value={state} onChange={(e) => setState(e.target.value as typeof state)} className="h-9 rounded-lg text-sm">
              <option value="open">Đang mở</option>
              <option value="investigating">Đang điều tra</option>
              <option value="resolved">Đã xử lý</option>
            </select>
          </label>
          <button type="button" className="btn h-9 px-3 text-xs" onClick={() => void load()}>
            Làm mới
          </button>
        </div>
        {!flags.ai_ops ? <p className="text-xs text-muted">Điều tra bằng AI đang tắt (FEATURE_AI_OPS); chỉ xem được danh sách sự cố.</p> : null}
        {notice ? (
          <p role="status" className="text-sm text-muted">
            {notice}
          </p>
        ) : null}
        {error ? <ErrorBlock message={error} onRetry={() => void load()} /> : null}
        {incidents === null ? <LoadingBlock /> : null}
        {incidents && incidents.length === 0 && !error ? <EmptyBlock>Không có sự cố nào ở trạng thái này.</EmptyBlock> : null}
        <ul className="grid gap-2">
          {(incidents ?? []).map((inc) => {
            const run = runs[inc.id];
            const active = run && ACTIVE.includes(run.status);
            return (
              <li key={inc.id} className={`rounded-xl border p-3 ${selected === inc.id ? 'border-brand bg-brand-soft/40' : 'border-line'}`}>
                <div className="flex flex-wrap items-center gap-2">
                  <Pill tone={SEVERITY_TONE[inc.severity] ?? 'bg-surface2 text-muted'}>{inc.severity}</Pill>
                  <strong className="text-sm">{inc.alert_name}</strong>
                  {inc.component ? <span className="text-xs text-muted">{inc.component}</span> : null}
                </div>
                {inc.summary ? <p className="mt-1 text-sm text-muted">{inc.summary}</p> : null}
                <p className="mt-1 text-xs text-muted">
                  #{inc.id} · {inc.fire_count} lần · gần nhất {formatDateTime(inc.last_seen)}
                </p>
                {flags.ai_ops ? (
                  <div className="mt-2 flex flex-wrap items-center gap-2">
                    {run ? <Pill tone={RUN_TONE[run.status] ?? ''}>{STATUS_LABEL[run.status]}</Pill> : <span className="text-xs text-muted">Chưa điều tra</span>}
                    {run?.trigger === 'auto' ? <span className="text-xs text-muted">tự động</span> : null}
                    {run ? (
                      <button type="button" className="btn h-8 px-3 text-xs" onClick={() => setSelected(inc.id)}>
                        Xem
                      </button>
                    ) : null}
                    {!active ? (
                      <button type="button" className="btn-primary h-8 px-3 text-xs" onClick={() => void investigate(inc.id)}>
                        {run ? 'Điều tra lại' : 'Điều tra'}
                      </button>
                    ) : null}
                  </div>
                ) : null}
              </li>
            );
          })}
        </ul>
      </section>

      <section className="grid content-start gap-3">
        {selected && runs[selected] ? (
          <RunDetail key={selected} incidentId={selected} runId={runs[selected].id} role={role} onChanged={() => void load()} />
        ) : (
          <EmptyBlock>Chọn một sự cố đã điều tra để xem trợ lý đã làm gì.</EmptyBlock>
        )}
      </section>
    </div>
  );
}

function RunDetail({ incidentId, runId, role, onChanged }: { incidentId: number; runId: number; role: string | null; onChanged: () => void }) {
  const getValidAccessToken = useToken();
  const [view, setView] = useState<RcaView | null>(null);
  const [proposals, setProposals] = useState<OpsProposal[]>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const alive = useRef(true);

  const load = useCallback(async () => {
    try {
      const token = await getValidAccessToken();
      const v = await getRcaRun(token, runId);
      if (!alive.current) return;
      setView(v);
      setProposals((await listIncidentProposals(token, incidentId)).proposals ?? []);
      setError('');
    } catch (err) {
      if (alive.current) setError(errMsg(err, 'Không tải được phiên điều tra.'));
    }
  }, [getValidAccessToken, runId, incidentId]);

  useEffect(() => {
    alive.current = true;
    void load();
    return () => {
      alive.current = false;
    };
  }, [load]);

  const active = view ? ACTIVE.includes(view.run.status) : false;
  useEffect(() => {
    if (!active) return;
    const t = setInterval(() => void load(), REFRESH_MS);
    return () => clearInterval(t);
  }, [active, load]);

  const act = async (name: string, fn: (token: string) => Promise<unknown>, okMsg = '') => {
    setBusy(name);
    setError('');
    try {
      await fn(await getValidAccessToken());
      if (okMsg) setError('');
      await load();
      onChanged();
    } catch (err) {
      setError(errMsg(err, 'Không thực hiện được.'));
    } finally {
      setBusy('');
    }
  };

  if (!view) return error ? <ErrorBlock message={error} onRetry={() => void load()} /> : <LoadingBlock label="Đang tải phiên điều tra…" />;
  const { run } = view;
  const draft = run.draft;

  return (
    <div className="grid gap-3">
      <div className="card grid gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <strong>Sự cố #{incidentId}</strong>
          <Pill tone={RUN_TONE[run.status] ?? ''}>{STATUS_LABEL[run.status]}</Pill>
          {run.outcome ? <Pill tone="bg-surface2 text-text">{OUTCOME_LABEL[run.outcome] ?? run.outcome}</Pill> : null}
          <span className="text-xs text-muted">
            vòng {view.round} · {view.queries} truy vấn · {view.model_calls} lượt gọi mô hình{view.playbook ? ` · playbook ${view.playbook}` : ''}
          </span>
        </div>
        {error ? (
          <p role="alert" className="text-sm font-medium text-danger">
            {error}
          </p>
        ) : null}
        {run.status === 'waiting_approval' || run.status === 'waiting_confirm' ? (
          <button type="button" className="btn h-8 w-fit px-3 text-xs" disabled={busy !== ''} onClick={() => void act('recheck', (t) => recheckRca(t, run.id))}>
            {busy === 'recheck' ? 'Đang kiểm tra…' : 'Kiểm tra lại'}
          </button>
        ) : null}
      </div>

      {run.status === 'waiting_confirm' && draft ? (
        <div className="card grid gap-2 border-warning/40">
          <h2 className="text-sm font-bold">Nháp đề xuất của trợ lý (chưa ghi gì)</h2>
          <p className="text-sm">
            <strong>{draft.action_id}</strong> {Object.keys(draft.params).length ? <code className="text-xs">{JSON.stringify(draft.params)}</code> : null}
            {draft.risk ? <span className="ml-2 text-xs text-muted">rủi ro: {draft.risk}</span> : null}
            {draft.escalated ? <span className="ml-2 text-xs text-muted">(chuyển cho người, không có hành động tự động)</span> : null}
          </p>
          <p className="text-sm text-muted">{draft.rationale}</p>
          {role === 'operator' ? (
            <button type="button" className="btn-primary h-9 w-fit px-4 text-sm" disabled={busy !== ''} onClick={() => void act('confirm', (t) => confirmRcaDraft(t, run.id))}>
              {busy === 'confirm' ? 'Đang ghi đề xuất…' : 'Xác nhận đề xuất'}
            </button>
          ) : (
            <p className="text-xs text-muted">Chỉ operator xác nhận được nháp; admin chỉ xem.</p>
          )}
          <p className="text-xs text-muted">Xác nhận chỉ GHI một đề xuất “chờ duyệt”. Chưa có gì chạy: một người khác phải duyệt, rồi chạy.</p>
        </div>
      ) : null}

      <Proposals proposals={proposals} busy={busy} act={act} />

      {view.report ? (
        <div className="card grid gap-1">
          <h2 className="text-sm font-bold">Báo cáo của trợ lý</h2>
          <p className="text-sm">{view.report.summary}</p>
          <p className="text-sm">
            Đủ điều kiện đóng sự cố: <strong>{view.report.ready_to_close ? 'có' : 'chưa'}</strong>
            {view.report.reason ? ` · ${view.report.reason}` : ''}
          </p>
          <p className="text-xs text-muted">Trợ lý không đóng sự cố; người quyết định.</p>
        </div>
      ) : null}

      <div className="card grid gap-2">
        <h2 className="text-sm font-bold">Giả thuyết</h2>
        {view.hypotheses.length === 0 ? <p className="text-sm text-muted">Chưa có.</p> : null}
        <ul className="grid gap-1 text-sm">
          {view.hypotheses.map((h, i) => (
            <li key={i}>
              <strong>{Math.round(h.confidence * 100)}%</strong> {h.cause}
              {h.evidence_ids?.length ? <span className="text-xs text-muted"> · {h.evidence_ids.join(', ')}</span> : null}
            </li>
          ))}
        </ul>
      </div>

      <details className="card grid gap-2">
        <summary className="cursor-pointer text-sm font-bold">Bằng chứng ({view.evidence.length})</summary>
        <ul className="grid gap-2 text-xs">
          {view.evidence.map((e) => (
            <li key={e.id} className="rounded-lg bg-surface2 p-2">
              <strong>{e.id}</strong> · {e.source} {e.args && Object.keys(e.args).length ? <code>{JSON.stringify(e.args)}</code> : null}
              <pre className="mt-1 whitespace-pre-wrap break-words">{e.summary}</pre>
            </li>
          ))}
        </ul>
      </details>

      <details className="card grid gap-2" open>
        <summary className="cursor-pointer text-sm font-bold">Dòng thời gian</summary>
        <ol className="grid gap-1 text-xs">
          {view.timeline.map((t, i) => (
            <li key={i}>
              <span className="font-semibold">{t.node}</span> · {t.note}
            </li>
          ))}
        </ol>
      </details>
    </div>
  );
}

function Proposals({ proposals, busy, act }: { proposals: OpsProposal[]; busy: string; act: (name: string, fn: (t: string) => Promise<unknown>) => Promise<void> }) {
  const [command, setCommand] = useState<Record<number, string>>({});
  const getValidAccessToken = useToken();
  const { role, userId } = useAuth();
  if (proposals.length === 0) return null;
  const showCommand = async (id: number) => {
    const { getProposal } = await import('../../../lib/rca');
    const out = await getProposal(await getValidAccessToken(), id);
    setCommand((c) => ({ ...c, [id]: out.command ?? '(hành động này không có lệnh để chạy tay)' }));
  };
  return (
    <div className="card grid gap-2">
      <h2 className="text-sm font-bold">Đề xuất vận hành của sự cố này</h2>
      <ul className="grid gap-2 text-sm">
        {proposals.map((p) => {
          const approved = p.state === 'ACCEPTED' || p.state === 'ALTERNATIVE';
          // ops-service ghi người tạo là "<role>:<userID>" (X-Actor do gateway đặt); chính người đó không tự duyệt được
          const mine = userId != null && role != null && p.created_by === `${role}:${userId}`;
          return (
            <li key={p.id} className="rounded-lg border border-line p-2">
              <div className="flex flex-wrap items-center gap-2">
                <strong>
                  #{p.id} {p.action_id}
                </strong>
                <Pill tone="bg-surface2 text-text">{p.state}</Pill>
                {p.exec_state ? <Pill tone="bg-surface2 text-text">{p.exec_state}</Pill> : null}
                {p.risk ? <span className="text-xs text-muted">rủi ro: {p.risk}</span> : null}
              </div>
              {p.params && Object.keys(p.params).length ? <code className="text-xs">{JSON.stringify(p.params)}</code> : null}
              {p.state === 'PENDING' ? (
                <div className="mt-2 flex flex-wrap gap-2">
                  {mine ? null : (
                    <button type="button" className="btn-primary h-8 px-3 text-xs" disabled={busy !== ''} onClick={() => void act(`accept${p.id}`, (t) => decideProposal(t, p.id, 'accept', 'duyệt từ màn hình sự cố'))}>
                      Duyệt
                    </button>
                  )}
                  <button type="button" className="btn-danger h-8 px-3 text-xs" disabled={busy !== ''} onClick={() => void act(`reject${p.id}`, (t) => decideProposal(t, p.id, 'reject', 'từ chối từ màn hình sự cố'))}>
                    Từ chối
                  </button>
                  <span className="text-xs text-muted">{mine ? 'Bạn là người tạo đề xuất này: cần người khác duyệt.' : 'Người tạo đề xuất không tự duyệt được (server từ chối).'}</span>
                </div>
              ) : null}
              {approved && !p.exec_state ? (
                <div className="mt-2 grid gap-2">
                  <div className="flex flex-wrap gap-2">
                    <button type="button" className="btn h-8 px-3 text-xs" disabled={busy !== ''} onClick={() => void act(`run${p.id}`, (t) => executeProposal(t, p.id))}>
                      Chạy (chỉ requeue_outbox tự chạy)
                    </button>
                    <button type="button" className="btn h-8 px-3 text-xs" onClick={() => void showCommand(p.id)}>
                      Xem lệnh để chạy tay
                    </button>
                    <button type="button" className="btn h-8 px-3 text-xs" disabled={busy !== ''} onClick={() => void act(`ok${p.id}`, (t) => recordExecuted(t, p.id, 'success', 'đã chạy tay'))}>
                      Đã chạy xong
                    </button>
                    <button type="button" className="btn-danger h-8 px-3 text-xs" disabled={busy !== ''} onClick={() => void act(`fail${p.id}`, (t) => recordExecuted(t, p.id, 'failed', 'chạy tay thất bại'))}>
                      Chạy thất bại
                    </button>
                  </div>
                  {command[p.id] ? <pre className="whitespace-pre-wrap break-words rounded-lg bg-surface2 p-2 text-xs">{command[p.id]}</pre> : null}
                </div>
              ) : null}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
