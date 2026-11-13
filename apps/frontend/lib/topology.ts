// Static topology of the platform (verified against deploy/services.yml and the code, 2026-11-13):
//  - only the gateway calls the six domain services (gRPC) and ops-service (HTTP); services do NOT call each other;
//  - domain services talk to each other asynchronously through Kafka (transactional outbox);
//  - each service owns its tables in one Postgres; analytics writes ClickHouse; gateway/auth/analytics use Redis.
// Live status is overlaid at runtime (lib/useSystemMap.ts). When the architecture changes, change this file.

export type NodeKind = 'client' | 'edge' | 'gateway' | 'service' | 'data' | 'obs';
export type EdgeProtocol = 'http' | 'grpc' | 'kafka' | 'sql' | 'cache' | 'metrics';
/** Edge groups the user can switch on/off on the map. */
export type EdgeLayer = 'sync' | 'async' | 'data';

export interface TopoNode {
  id: string;
  label: string;
  kind: NodeKind;
  x: number;
  y: number;
  /** One line shown in the detail panel. */
  desc: string;
  /** Prometheus `job` label for infrastructure nodes (query `infra_up`). */
  job?: string;
  /** true for the 8 Go services reported by /admin/system/health and scraped as job mm-services. */
  app?: boolean;
}

export interface TopoEdge {
  id: string;
  from: string;
  to: string;
  protocol: EdgeProtocol;
  layer: EdgeLayer;
  label?: string;
}

export const NODES: TopoNode[] = [
  { id: 'browser', label: 'Trình duyệt', kind: 'client', x: 0, y: 270, desc: 'Người dùng cuối. Gọi frontend và gateway qua Traefik.' },
  { id: 'traefik', label: 'traefik', kind: 'edge', x: 230, y: 270, job: 'traefik', desc: 'Reverse proxy / TLS. Định tuyến theo host tới frontend và api-gateway.' },
  { id: 'frontend', label: 'frontend', kind: 'edge', x: 470, y: 130, desc: 'Next.js. Chỉ nói chuyện với gateway; không có metric riêng.' },
  { id: 'api-gateway', label: 'api-gateway', kind: 'gateway', x: 470, y: 360, app: true, desc: 'REST (gin) → gRPC. JWT, RBAC, rate limit, circuit breaker, proxy /ai/chat.' },
  { id: 'auth-service', label: 'auth-service', kind: 'service', x: 740, y: 0, app: true, desc: 'Tài khoản, mật khẩu, cấp và làm mới JWT.' },
  { id: 'user-service', label: 'user-service', kind: 'service', x: 740, y: 85, app: true, desc: 'Hồ sơ người mua/bán, địa chỉ giao hàng.' },
  { id: 'product-service', label: 'product-service', kind: 'service', x: 740, y: 170, app: true, desc: 'Danh mục, tìm kiếm, tồn kho (available / reserved / sold).' },
  { id: 'order-service', label: 'order-service', kind: 'service', x: 740, y: 255, app: true, desc: 'Giỏ hàng, checkout (mỗi cửa hàng một đơn), máy trạng thái đơn.' },
  { id: 'payment-service', label: 'payment-service', kind: 'service', x: 740, y: 340, app: true, desc: 'Thanh toán MÔ PHỎNG, webhook, hoàn tiền.' },
  { id: 'analytics-service', label: 'analytics-service', kind: 'service', x: 740, y: 425, app: true, desc: 'Nhận sự kiện (Kafka + tracking), ghi ClickHouse, báo cáo theo phạm vi.' },
  { id: 'ops-service', label: 'ops-service', kind: 'service', x: 740, y: 510, app: true, desc: 'Sự cố từ cảnh báo, đề xuất, nhật ký bất biến, truy vấn allowlist.' },
  { id: 'redis', label: 'redis', kind: 'data', x: 1030, y: 0, job: 'redis', desc: 'Cache, rate limit, pwd_version cho JWT.' },
  { id: 'postgres', label: 'postgres', kind: 'data', x: 1030, y: 130, job: 'postgres', desc: 'Cơ sở dữ liệu giao dịch; mỗi service một schema/bảng riêng.' },
  { id: 'kafka', label: 'kafka', kind: 'data', x: 1030, y: 255, job: 'kafka', desc: 'Sự kiện miền (order.*, payment.*, product.*, inventory.*).' },
  { id: 'clickhouse', label: 'clickhouse', kind: 'data', x: 1030, y: 385, job: 'clickhouse', desc: 'Kho phân tích (fact tables) cho báo cáo.' },
  { id: 'prometheus', label: 'prometheus', kind: 'obs', x: 1030, y: 490, desc: 'Số liệu và cảnh báo. ops-service chỉ đọc qua truy vấn allowlist.' },
  { id: 'loki', label: 'loki', kind: 'obs', x: 1030, y: 565, job: 'loki', desc: 'Log tập trung (Vector gom về).' }
];

const grpc = (to: string): TopoEdge => ({ id: `api-gateway>${to}`, from: 'api-gateway', to, protocol: 'grpc', layer: 'sync' });

export const EDGES: TopoEdge[] = [
  { id: 'browser>traefik', from: 'browser', to: 'traefik', protocol: 'http', layer: 'sync' },
  { id: 'traefik>frontend', from: 'traefik', to: 'frontend', protocol: 'http', layer: 'sync' },
  { id: 'traefik>api-gateway', from: 'traefik', to: 'api-gateway', protocol: 'http', layer: 'sync' },
  grpc('auth-service'),
  grpc('user-service'),
  grpc('product-service'),
  grpc('order-service'),
  grpc('payment-service'),
  grpc('analytics-service'),
  { id: 'api-gateway>ops-service', from: 'api-gateway', to: 'ops-service', protocol: 'http', layer: 'sync' },

  // asynchronous (Kafka, transactional outbox)
  { id: 'product-service>kafka', from: 'product-service', to: 'kafka', protocol: 'kafka', layer: 'async', label: 'product.*' },
  { id: 'order-service>kafka', from: 'order-service', to: 'kafka', protocol: 'kafka', layer: 'async', label: 'order.*' },
  { id: 'payment-service>kafka', from: 'payment-service', to: 'kafka', protocol: 'kafka', layer: 'async', label: 'payment.*' },
  { id: 'kafka>order-service', from: 'kafka', to: 'order-service', protocol: 'kafka', layer: 'async' },
  { id: 'kafka>payment-service', from: 'kafka', to: 'payment-service', protocol: 'kafka', layer: 'async' },
  { id: 'kafka>analytics-service', from: 'kafka', to: 'analytics-service', protocol: 'kafka', layer: 'async' },

  // data stores
  { id: 'api-gateway>redis', from: 'api-gateway', to: 'redis', protocol: 'cache', layer: 'data' },
  { id: 'auth-service>redis', from: 'auth-service', to: 'redis', protocol: 'cache', layer: 'data' },
  { id: 'auth-service>postgres', from: 'auth-service', to: 'postgres', protocol: 'sql', layer: 'data' },
  { id: 'user-service>postgres', from: 'user-service', to: 'postgres', protocol: 'sql', layer: 'data' },
  { id: 'product-service>postgres', from: 'product-service', to: 'postgres', protocol: 'sql', layer: 'data' },
  { id: 'order-service>postgres', from: 'order-service', to: 'postgres', protocol: 'sql', layer: 'data' },
  { id: 'payment-service>postgres', from: 'payment-service', to: 'postgres', protocol: 'sql', layer: 'data' },
  { id: 'ops-service>postgres', from: 'ops-service', to: 'postgres', protocol: 'sql', layer: 'data' },
  { id: 'analytics-service>clickhouse', from: 'analytics-service', to: 'clickhouse', protocol: 'sql', layer: 'data' },
  { id: 'ops-service>prometheus', from: 'ops-service', to: 'prometheus', protocol: 'metrics', layer: 'data' },
  { id: 'ops-service>loki', from: 'ops-service', to: 'loki', protocol: 'metrics', layer: 'data' }
];

export const PROTOCOL_LABEL: Record<EdgeProtocol, string> = {
  http: 'HTTP',
  grpc: 'gRPC',
  kafka: 'Kafka',
  sql: 'SQL',
  cache: 'Redis',
  metrics: 'Truy vấn'
};

export const LAYER_LABEL: Record<EdgeLayer, string> = {
  sync: 'Gọi trực tiếp (HTTP/gRPC)',
  async: 'Sự kiện (Kafka)',
  data: 'Kho dữ liệu'
};

export const nodeById = (id: string): TopoNode | undefined => NODES.find((n) => n.id === id);
