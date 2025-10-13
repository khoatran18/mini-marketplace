'use client';

import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { useAuth } from '../../components/auth/AuthProvider';
import {
  changePasswordRequest,
  createBuyerProfileRequest,
  createSellerProfileRequest,
  getBuyerProfileRequest,
  getSellerProfileRequest,
  updateBuyerProfileRequest,
  updateSellerProfileRequest
} from '../../lib/api';
import type { BuyerProfile, SellerProfile } from '../../lib/types';

const genderOptions = [
  { value: 'male', label: 'Nam' },
  { value: 'female', label: 'Nữ' },
  { value: 'other', label: 'Khác' }
];

interface BuyerFormState {
  name: string;
  gender: string;
  date_of_birth: string;
  phone: string;
  address: string;
}

interface SellerFormState {
  name: string;
  bank_account: string;
  tax_code: string;
  description: string;
  date_of_birth: string;
  phone: string;
  address: string;
}

interface PasswordFormState {
  oldPassword: string;
  newPassword: string;
  confirmPassword: string;
}

function createEmptyBuyerForm(): BuyerFormState {
  return {
    name: '',
    gender: 'other',
    date_of_birth: '',
    phone: '',
    address: ''
  };
}

function createEmptySellerForm(): SellerFormState {
  return {
    name: '',
    bank_account: '',
    tax_code: '',
    description: '',
    date_of_birth: '',
    phone: '',
    address: ''
  };
}

function formatDateInput(value?: string | null) {
  if (!value) {
    return '';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  return date.toISOString().split('T')[0] ?? '';
}

function ensureIsoDate(value: string) {
  if (!value) {
    throw new Error('Vui lòng chọn ngày sinh.');
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    throw new Error('Ngày sinh không hợp lệ.');
  }
  return date.toISOString();
}

function isNotFoundMessage(message: string) {
  return /not\s+found/i.test(message);
}

export function ProfilePageClient() {
  const searchParams = useSearchParams();
  const { role, userId, username, getValidAccessToken } = useAuth();
  const isSetupFlow = useMemo(() => {
    if (!searchParams) {
      return false;
    }
    const value = searchParams.get('setup');
    return value !== null;
  }, [searchParams]);

  const [buyerForm, setBuyerForm] = useState<BuyerFormState>(createEmptyBuyerForm);
  const [sellerForm, setSellerForm] = useState<SellerFormState>(createEmptySellerForm);
  const [profileExists, setProfileExists] = useState(false);
  const [loadingProfile, setLoadingProfile] = useState(true);
  const [profileMessage, setProfileMessage] = useState<string | null>(null);
  const [profileError, setProfileError] = useState<string | null>(null);
  const [savingProfile, setSavingProfile] = useState(false);

  const [passwordForm, setPasswordForm] = useState<PasswordFormState>({
    oldPassword: '',
    newPassword: '',
    confirmPassword: ''
  });
  const [passwordMessage, setPasswordMessage] = useState<string | null>(null);
  const [passwordError, setPasswordError] = useState<string | null>(null);
  const [changingPassword, setChangingPassword] = useState(false);

  useEffect(() => {
    setBuyerForm(createEmptyBuyerForm());
    setSellerForm(createEmptySellerForm());
    setProfileExists(false);
    setProfileMessage(null);
    setProfileError(null);
  }, [role]);

  useEffect(() => {
    let cancelled = false;

    async function loadProfile() {
      if (!userId || !role) {
        setLoadingProfile(false);
        return;
      }

      setLoadingProfile(true);
      setProfileMessage(null);
      setProfileError(null);

      try {
        const token = await getValidAccessToken();
        if (!token) {
          throw new Error('Không thể xác thực. Vui lòng đăng nhập lại.');
        }

        if (role === 'buyer') {
          const response = await getBuyerProfileRequest(userId, token);
          if (cancelled) {
            return;
          }
          if (response?.buyer) {
            const buyer: BuyerProfile = response.buyer;
            setBuyerForm({
              name: buyer.name ?? '',
              gender: buyer.gender ?? 'other',
              date_of_birth: formatDateInput(buyer.date_of_birth),
              phone: buyer.phone ?? '',
              address: buyer.address ?? ''
            });
            setProfileExists(true);
          } else {
            setProfileExists(false);
          }
        } else if (role === 'seller_admin' || role === 'seller_employee') {
          const response = await getSellerProfileRequest(userId, token);
          if (cancelled) {
            return;
          }
          if (response?.seller) {
            const seller: SellerProfile = response.seller;
            setSellerForm({
              name: seller.name ?? '',
              bank_account: seller.bank_account ?? '',
              tax_code: seller.tax_code ?? '',
              description: seller.description ?? '',
              date_of_birth: formatDateInput(seller.date_of_birth),
              phone: seller.phone ?? '',
              address: seller.address ?? ''
            });
            setProfileExists(true);
          } else {
            setProfileExists(false);
          }
        } else {
          setProfileExists(false);
        }
      } catch (error) {
        if (cancelled) {
          return;
        }
        const message = error instanceof Error ? error.message : 'Không thể tải thông tin cá nhân.';
        if (isNotFoundMessage(message)) {
          setProfileExists(false);
          setProfileError(null);
        } else {
          setProfileError(message);
        }
      } finally {
        if (!cancelled) {
          setLoadingProfile(false);
        }
      }
    }

    void loadProfile();

    return () => {
      cancelled = true;
    };
  }, [userId, role, getValidAccessToken]);

  const isBuyer = role === 'buyer';
  const isSeller = role === 'seller_admin' || role === 'seller_employee';
  const canEditProfile = isBuyer || isSeller;
  const profileTitle = isBuyer
    ? 'Thông tin người mua'
    : isSeller
      ? 'Thông tin nhà bán hàng'
      : 'Thông tin cá nhân';
  const profileSubtitle = isBuyer
    ? 'Điền chính xác thông tin để thuận tiện trong quá trình mua hàng.'
    : isSeller
      ? 'Cập nhật đầy đủ thông tin cửa hàng để tăng độ tin cậy với khách hàng.'
      : 'Vai trò hiện tại chưa hỗ trợ cập nhật chi tiết thông tin.';

  const handleBuyerSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!userId) {
      setProfileError('Không xác định được người dùng.');
      return;
    }

    setSavingProfile(true);
    setProfileMessage(null);
    setProfileError(null);

    try {
      const token = await getValidAccessToken();
      if (!token) {
        throw new Error('Không thể xác thực. Vui lòng đăng nhập lại.');
      }
      const dateOfBirth = ensureIsoDate(buyerForm.date_of_birth);
      const payload: BuyerProfile = {
        user_id: userId,
        name: buyerForm.name.trim(),
        gender: buyerForm.gender.trim(),
        date_of_birth: dateOfBirth,
        phone: buyerForm.phone.trim(),
        address: buyerForm.address.trim()
      };

      if (profileExists) {
        await updateBuyerProfileRequest(userId, { buyer: payload }, token);
        setProfileMessage('Cập nhật thông tin cá nhân thành công.');
      } else {
        await createBuyerProfileRequest({ buyer: payload }, token);
        setProfileExists(true);
        setProfileMessage('Đã lưu thông tin cá nhân.');
      }
    } catch (error) {
      setProfileError(error instanceof Error ? error.message : 'Không thể lưu thông tin cá nhân.');
    } finally {
      setSavingProfile(false);
    }
  };

  const handleSellerSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!userId) {
      setProfileError('Không xác định được người dùng.');
      return;
    }

    setSavingProfile(true);
    setProfileMessage(null);
    setProfileError(null);

    try {
      const token = await getValidAccessToken();
      if (!token) {
        throw new Error('Không thể xác thực. Vui lòng đăng nhập lại.');
      }
      const dateOfBirth = ensureIsoDate(sellerForm.date_of_birth);
      const payload: SellerProfile = {
        name: sellerForm.name.trim(),
        bank_account: sellerForm.bank_account.trim(),
        tax_code: sellerForm.tax_code.trim(),
        description: sellerForm.description.trim(),
        date_of_birth: dateOfBirth,
        phone: sellerForm.phone.trim(),
        address: sellerForm.address.trim()
      };

      if (profileExists) {
        await updateSellerProfileRequest(userId, { seller: payload, user_id: userId }, token);
        setProfileMessage('Cập nhật thông tin cửa hàng thành công.');
      } else {
        await createSellerProfileRequest({ seller: payload, user_id: userId }, token);
        setProfileExists(true);
        setProfileMessage('Đã lưu thông tin cửa hàng.');
      }
    } catch (error) {
      setProfileError(error instanceof Error ? error.message : 'Không thể lưu thông tin cửa hàng.');
    } finally {
      setSavingProfile(false);
    }
  };

  const handlePasswordSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setPasswordError(null);
    setPasswordMessage(null);

    if (!username || !role) {
      setPasswordError('Không xác định được tài khoản.');
      return;
    }
    if (!passwordForm.oldPassword || !passwordForm.newPassword) {
      setPasswordError('Vui lòng nhập đầy đủ mật khẩu cũ và mật khẩu mới.');
      return;
    }
    if (passwordForm.newPassword !== passwordForm.confirmPassword) {
      setPasswordError('Mật khẩu mới không khớp.');
      return;
    }

    setChangingPassword(true);
    try {
      const token = await getValidAccessToken();
      if (!token) {
        throw new Error('Không thể xác thực. Vui lòng đăng nhập lại.');
      }
      const response = await changePasswordRequest(
        {
          username,
          old_password: passwordForm.oldPassword,
          new_password: passwordForm.newPassword,
          role
        },
        token
      );
      setPasswordMessage(response.message ?? 'Đổi mật khẩu thành công.');
      setPasswordForm({ oldPassword: '', newPassword: '', confirmPassword: '' });
    } catch (error) {
      setPasswordError(error instanceof Error ? error.message : 'Không thể đổi mật khẩu.');
    } finally {
      setChangingPassword(false);
    }
  };

  return (
    <div className="mx-auto w-full max-w-5xl space-y-8 px-4 py-6">
      <div className="space-y-2">
        <h1 className="text-3xl font-bold text-slate-900">Thông tin cá nhân</h1>
        <p className="text-sm text-slate-600">
          Quản lý dữ liệu cá nhân và cập nhật mật khẩu để bảo vệ tài khoản của bạn.
        </p>
        {isSetupFlow ? (
          <p className="rounded-xl bg-indigo-50 px-4 py-2 text-sm text-indigo-700">
            Đăng ký thành công! Vui lòng hoàn thiện thông tin bên dưới để bắt đầu sử dụng hệ thống.
          </p>
        ) : null}
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <section className="card">
          <div className="flex items-center justify-between">
            <div>
              <h2 className="text-xl font-semibold text-slate-900">{profileTitle}</h2>
              <p className="text-sm text-slate-600">{profileSubtitle}</p>
            </div>
            {loadingProfile ? <span className="text-xs font-medium text-slate-500">Đang tải...</span> : null}
          </div>

          {profileMessage ? (
            <p className="mt-4 rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{profileMessage}</p>
          ) : null}
          {profileError ? (
            <p className="mt-4 rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-600">{profileError}</p>
          ) : null}

          {canEditProfile ? (
            <form
              className="mt-6 grid gap-4"
              onSubmit={isBuyer ? handleBuyerSubmit : handleSellerSubmit}
            >
              <label className="grid gap-1 text-sm font-medium text-slate-700">
                Họ và tên
                <input
                  type="text"
                  value={isBuyer ? buyerForm.name : sellerForm.name}
                  onChange={(event) => {
                    const value = event.target.value;
                    if (isBuyer) {
                      setBuyerForm((prev) => ({ ...prev, name: value }));
                    } else {
                      setSellerForm((prev) => ({ ...prev, name: value }));
                    }
                  }}
                  required
                />
              </label>

              {isBuyer ? (
                <label className="grid gap-1 text-sm font-medium text-slate-700">
                  Giới tính
                  <select
                    value={buyerForm.gender}
                    onChange={(event) =>
                      setBuyerForm((prev) => ({ ...prev, gender: event.target.value }))
                    }
                    className="w-full rounded-xl border border-slate-300 px-3 py-2 text-base shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200"
                  >
                    {genderOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </label>
              ) : (
                <label className="grid gap-1 text-sm font-medium text-slate-700">
                  Tài khoản ngân hàng
                  <input
                    type="text"
                    value={sellerForm.bank_account}
                    onChange={(event) =>
                      setSellerForm((prev) => ({ ...prev, bank_account: event.target.value }))
                    }
                    placeholder="Ví dụ: 0123456789 - Ngân hàng A"
                    required
                  />
                </label>
              )}

              <label className="grid gap-1 text-sm font-medium text-slate-700">
                Ngày sinh
                <input
                  type="date"
                  value={isBuyer ? buyerForm.date_of_birth : sellerForm.date_of_birth}
                  onChange={(event) => {
                    const value = event.target.value;
                    if (isBuyer) {
                      setBuyerForm((prev) => ({ ...prev, date_of_birth: value }));
                    } else {
                      setSellerForm((prev) => ({ ...prev, date_of_birth: value }));
                    }
                  }}
                  required
                />
              </label>

              <label className="grid gap-1 text-sm font-medium text-slate-700">
                Số điện thoại
                <input
                  type="tel"
                  value={isBuyer ? buyerForm.phone : sellerForm.phone}
                  onChange={(event) => {
                    const value = event.target.value;
                    if (isBuyer) {
                      setBuyerForm((prev) => ({ ...prev, phone: value }));
                    } else {
                      setSellerForm((prev) => ({ ...prev, phone: value }));
                    }
                  }}
                  required
                />
              </label>

              {isSeller ? (
                <label className="grid gap-1 text-sm font-medium text-slate-700">
                  Mã số thuế
                  <input
                    type="text"
                    value={sellerForm.tax_code}
                    onChange={(event) =>
                      setSellerForm((prev) => ({ ...prev, tax_code: event.target.value }))
                    }
                    required
                  />
                </label>
              ) : null}

              <label className="grid gap-1 text-sm font-medium text-slate-700">
                Địa chỉ
                <textarea
                  value={isBuyer ? buyerForm.address : sellerForm.address}
                  onChange={(event) => {
                    const value = event.target.value;
                    if (isBuyer) {
                      setBuyerForm((prev) => ({ ...prev, address: value }));
                    } else {
                      setSellerForm((prev) => ({ ...prev, address: value }));
                    }
                  }}
                  rows={3}
                  required
                />
              </label>

              {isSeller ? (
                <label className="grid gap-1 text-sm font-medium text-slate-700">
                  Mô tả cửa hàng
                  <textarea
                    value={sellerForm.description}
                    onChange={(event) =>
                      setSellerForm((prev) => ({ ...prev, description: event.target.value }))
                    }
                    rows={3}
                    placeholder="Giới thiệu ngắn gọn về cửa hàng của bạn"
                    required
                  />
                </label>
              ) : null}

              <button
                type="submit"
                disabled={savingProfile || loadingProfile}
                className="mt-2 rounded-xl bg-indigo-600 px-5 py-2.5 font-semibold text-white shadow-sm transition hover:bg-indigo-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 disabled:cursor-not-allowed disabled:opacity-70"
              >
                {savingProfile ? 'Đang lưu...' : profileExists ? 'Cập nhật thông tin' : 'Lưu thông tin'}
              </button>
            </form>
          ) : (
            <p className="mt-6 rounded-lg bg-slate-100 px-3 py-2 text-sm text-slate-600">
              Vai trò hiện tại chưa hỗ trợ cập nhật thông tin cá nhân. Vui lòng liên hệ quản trị viên nếu bạn cần hỗ trợ.
            </p>
          )}
        </section>

        <section className="card h-max">
          <h2 className="text-xl font-semibold text-slate-900">Đổi mật khẩu</h2>
          <p className="text-sm text-slate-600">
            Sử dụng mật khẩu mạnh để đảm bảo an toàn cho tài khoản của bạn.
          </p>

          {passwordMessage ? (
            <p className="mt-4 rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{passwordMessage}</p>
          ) : null}
          {passwordError ? (
            <p className="mt-4 rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-600">{passwordError}</p>
          ) : null}

          <form className="mt-6 grid gap-4" onSubmit={handlePasswordSubmit}>
            <label className="grid gap-1 text-sm font-medium text-slate-700">
              Mật khẩu hiện tại
              <input
                type="password"
                value={passwordForm.oldPassword}
                onChange={(event) =>
                  setPasswordForm((prev) => ({ ...prev, oldPassword: event.target.value }))
                }
                required
              />
            </label>
            <label className="grid gap-1 text-sm font-medium text-slate-700">
              Mật khẩu mới
              <input
                type="password"
                value={passwordForm.newPassword}
                minLength={6}
                onChange={(event) =>
                  setPasswordForm((prev) => ({ ...prev, newPassword: event.target.value }))
                }
                required
              />
            </label>
            <label className="grid gap-1 text-sm font-medium text-slate-700">
              Nhập lại mật khẩu mới
              <input
                type="password"
                value={passwordForm.confirmPassword}
                minLength={6}
                onChange={(event) =>
                  setPasswordForm((prev) => ({ ...prev, confirmPassword: event.target.value }))
                }
                required
              />
            </label>
            <button
              type="submit"
              disabled={changingPassword}
              className="rounded-xl bg-slate-900 px-5 py-2.5 font-semibold text-white shadow-sm transition hover:bg-slate-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-900 disabled:cursor-not-allowed disabled:opacity-70"
            >
              {changingPassword ? 'Đang xử lý...' : 'Cập nhật mật khẩu'}
            </button>
          </form>
        </section>
      </div>
    </div>
  );
}
