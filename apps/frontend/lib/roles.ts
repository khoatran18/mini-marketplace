import type { Role } from './types';

export const ROLE_LABEL: Record<Role, string> = {
  buyer: 'Người mua',
  seller_admin: 'Chủ cửa hàng',
  seller_employee: 'Nhân viên bán hàng',
  admin: 'Quản trị',
  dev: 'Vận hành (dev)'
};

export const ROLE_HINT: Record<Role, string> = {
  buyer: 'Mua sắm, theo dõi đơn',
  seller_admin: 'Quản lý cửa hàng và đơn bán',
  seller_employee: 'Xử lý đơn, tồn kho',
  admin: 'Toàn bộ console quản trị',
  dev: 'Vận hành hệ thống, không xem số liệu kinh doanh'
};
