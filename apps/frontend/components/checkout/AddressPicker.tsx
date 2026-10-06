'use client';

import { useEffect, useState } from 'react';
import { deleteAddressRequest, getAddressesRequest, saveAddressRequest } from '../../lib/api';
import { useApiData, useAuthedAction } from '../../lib/hooks';
import type { Address } from '../../lib/types';
import { EmptyBlock, ErrorBlock, LoadingBlock, Notice } from '../ui/StateBlock';

const emptyAddress: Address = { label: 'Nhà', receiver_name: '', phone: '', line1: '', ward: '', district: '', city: '', is_default: false };

interface Props {
  /** when set, addresses are selectable (checkout); otherwise the component only manages them (profile) */
  selectedId?: number | null;
  onSelect?: (id: number | null) => void;
}

export function formatAddress(a: { line1?: string; ward?: string; district?: string; city?: string }) {
  return [a.line1, a.ward, a.district, a.city].filter(Boolean).join(', ');
}

/** Delivery addresses of the signed-in buyer: list, pick, add, edit, delete (GET/POST/PUT/DELETE /users/me/addresses). */
export function AddressPicker({ selectedId, onSelect }: Props) {
  const run = useAuthedAction();
  const list = useApiData((token) => getAddressesRequest(token as string), []);
  const [editing, setEditing] = useState<Address | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const addresses = list.data?.addresses ?? [];

  // preselect the default (or only) address
  useEffect(() => {
    if (!onSelect || addresses.length === 0) return;
    if (selectedId && addresses.some((address) => address.id === selectedId)) return;
    const preferred = addresses.find((address) => address.is_default) ?? addresses[0];
    onSelect(preferred.id ?? null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [addresses]);

  const save = async () => {
    if (!editing) return;
    setSaving(true);
    setError(null);
    try {
      const saved = await run((token) => saveAddressRequest(editing, token));
      setEditing(null);
      list.reload();
      if (onSelect && saved.address?.id && !editing.id) onSelect(saved.address.id);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const remove = async (id: number) => {
    setError(null);
    try {
      await run((token) => deleteAddressRequest(id, token));
      if (selectedId === id) onSelect?.(null);
      list.reload();
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const field = (key: keyof Address, label: string, required = true, type = 'text') => (
    <label>
      {label}
      <input
        type={type}
        required={required}
        value={String(editing?.[key] ?? '')}
        onChange={(event) => setEditing((prev) => (prev ? { ...prev, [key]: event.target.value } : prev))}
      />
    </label>
  );

  return (
    <div className="grid gap-3">
      {list.loading ? <LoadingBlock label="Đang tải địa chỉ…" /> : null}
      {list.error ? <ErrorBlock message={list.error} onRetry={list.reload} /> : null}
      {error ? <Notice tone="error">{error}</Notice> : null}
      {!list.loading && !list.error && addresses.length === 0 && !editing ? <EmptyBlock>Bạn chưa có địa chỉ giao hàng. Hãy thêm địa chỉ để đặt hàng.</EmptyBlock> : null}

      <ul className="grid gap-2">
        {addresses.map((address) => (
          <li key={address.id} className={`flex flex-wrap items-start justify-between gap-3 rounded-xl border p-3 ${selectedId === address.id ? 'border-brand bg-brand-soft' : 'border-line'}`}>
            <label className="flex flex-1 items-start gap-3 font-normal">
              {onSelect ? <input type="radio" name="address" checked={selectedId === address.id} onChange={() => onSelect(address.id ?? null)} className="mt-1" /> : null}
              <span className="grid gap-0.5 text-sm text-text">
                <span className="font-semibold">
                  {address.label} – {address.receiver_name} · {address.phone}
                  {address.is_default ? <span className="ml-2 rounded-full bg-surface2 px-2 py-0.5 text-xs text-muted">Mặc định</span> : null}
                </span>
                <span className="text-muted">{formatAddress(address)}</span>
              </span>
            </label>
            <span className="flex gap-2">
              <button type="button" className="btn" onClick={() => setEditing({ ...address })}>
                Sửa
              </button>
              <button type="button" className="btn-danger" onClick={() => void remove(address.id as number)}>
                Xoá
              </button>
            </span>
          </li>
        ))}
      </ul>

      {editing ? (
        <form
          className="grid gap-3 rounded-xl border border-line p-4"
          onSubmit={(event) => {
            event.preventDefault();
            void save();
          }}
        >
          <h3 className="font-semibold text-text">{editing.id ? 'Sửa địa chỉ' : 'Thêm địa chỉ mới'}</h3>
          <div className="grid gap-3 sm:grid-cols-2">
            {field('label', 'Nhãn (Nhà, Công ty…)')}
            {field('receiver_name', 'Người nhận')}
            {field('phone', 'Số điện thoại', true, 'tel')}
            {field('line1', 'Số nhà, đường')}
            {field('ward', 'Phường / Xã', false)}
            {field('district', 'Quận / Huyện', false)}
            {field('city', 'Tỉnh / Thành phố')}
          </div>
          <label className="flex items-center gap-2 font-normal">
            <input type="checkbox" checked={editing.is_default} onChange={(event) => setEditing((prev) => (prev ? { ...prev, is_default: event.target.checked } : prev))} />
            Đặt làm địa chỉ mặc định
          </label>
          <div className="flex gap-2">
            <button type="submit" className="btn-primary" disabled={saving}>
              {saving ? 'Đang lưu…' : 'Lưu địa chỉ'}
            </button>
            <button type="button" className="btn" onClick={() => setEditing(null)}>
              Huỷ
            </button>
          </div>
        </form>
      ) : (
        <div>
          <button type="button" className="btn" onClick={() => setEditing({ ...emptyAddress, is_default: addresses.length === 0 })}>
            + Thêm địa chỉ
          </button>
        </div>
      )}
    </div>
  );
}
