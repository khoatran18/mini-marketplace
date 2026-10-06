'use client';

import { summaryKpis } from '../../components/analytics/AnalyticsDashboard';
import { AnalyticsPanel } from '../../components/analytics/AnalyticsPanel';
import { KpiRow } from '../../components/analytics/KpiRow';
import { parseSummary, rangeParams } from '../../lib/analytics';

export function AdminOverviewClient() {
  return <AnalyticsPanel title="Hôm nay" scope="admin" report="summary" params={rangeParams({ period: 'today' })} parse={parseSummary} render={(summary) => <KpiRow kpis={summaryKpis(summary)} />} />;
}
