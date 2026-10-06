'use client';

import { AnalyticsPanel } from '../../components/analytics/AnalyticsPanel';
import { KpiRow } from '../../components/analytics/KpiRow';
import { toKpis } from '../../lib/analytics';

export function AdminOverviewClient() {
  return <AnalyticsPanel title="Hôm nay" scope="admin" report="summary" params={{ period: 'today', compare: 'prev' }} render={(data) => <KpiRow kpis={toKpis(data)} />} />;
}
