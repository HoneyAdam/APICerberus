import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AnalyticsPage } from './Analytics';
import { useAnalyticsTimeseries, useAnalyticsTopRoutes } from '@/hooks/use-analytics';
import { useQuery } from '@tanstack/react-query';

// Mock hooks
vi.mock('@/hooks/use-analytics', () => ({
  useAnalyticsTimeseries: vi.fn(),
  useAnalyticsTopRoutes: vi.fn(),
}));

vi.mock('@/lib/api', () => ({
  adminApiRequest: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({
  useQuery: vi.fn(),
}));

// Mock chart components
vi.mock('@/components/charts/AreaChart', () => ({
  AreaChart: ({ title, data }: { title: string; data: unknown[] }) => (
    <div data-testid="area-chart">{title} - {data.length} points</div>
  ),
}));

vi.mock('@/components/charts/PieChart', () => ({
  PieChart: ({ title, data }: { title: string; data: unknown[] }) => (
    <div data-testid="pie-chart">{title} - {data.length} slices</div>
  ),
}));

vi.mock('@/components/charts/HeatmapChart', () => ({
  HeatmapChart: ({ title, data }: { title: string; data: unknown[] }) => (
    <div data-testid="heatmap-chart">{title} - {data.length} points</div>
  ),
}));

vi.mock('@/components/shared/DataTable', () => ({
  DataTable: ({ data, searchPlaceholder }: { data: unknown[]; searchPlaceholder?: string }) => (
    <div data-testid="data-table">{data.length} rows - {searchPlaceholder}</div>
  ),
}));

vi.mock('@/components/analytics/GeoDistributionChart', () => ({
  GeoDistributionChart: ({ data }: { data: unknown[] }) => (
    <div data-testid="geo-chart">{data.length} countries</div>
  ),
}));

vi.mock('@/components/analytics/RateLimitStats', () => ({
  RateLimitStatsCard: ({ data }: { data: unknown }) => (
    <div data-testid="rate-limit-stats">Rate limit stats</div>
  ),
}));

describe('AnalyticsPage', () => {
  beforeEach(() => {
    vi.mocked(useAnalyticsTimeseries).mockReturnValue({
      data: {
        items: [
          { timestamp: '2026-04-10T12:00:00Z', requests: 100, errors: 2, p95_latency_ms: 45, credits_consumed: 100 },
        ],
      },
    } as any);

    vi.mocked(useAnalyticsTopRoutes).mockReturnValue({
      data: { routes: [{ route_name: 'api-users', count: 5000 }] },
    } as any);

    vi.mocked(useQuery).mockImplementation(({ queryKey }: any) => {
      if (queryKey[0] === 'analytics-top-consumers')
        return { data: { consumers: [{ user_id: 'u1', consumer_name: 'Alice', count: 1000 }] } };
      if (queryKey[0] === 'analytics-status-codes')
        return { data: { status_codes: { '200': 900, '404': 50, '500': 50 } } };
      if (queryKey[0] === 'analytics-geo')
        return { data: { countries: [{ country: 'US', count: 500 }] } };
      if (queryKey[0] === 'analytics-rate-limits')
        return { data: { total_limited: 10, active_limits: 5 } };
      return { data: undefined };
    });
  });

  it('renders "Traffic Time-Series" area chart', () => {
    render(<AnalyticsPage />);
    expect(screen.getByTestId('area-chart')).toBeInTheDocument();
    expect(screen.getByTestId('area-chart').textContent).toContain('Traffic Time-Series');
  });

  it('renders "Status Code Distribution" pie chart', () => {
    render(<AnalyticsPage />);
    expect(screen.getByTestId('pie-chart')).toBeInTheDocument();
    expect(screen.getByTestId('pie-chart').textContent).toContain('Status Code Distribution');
  });

  it('renders "Latency Heatmap (p95)" heatmap chart', () => {
    render(<AnalyticsPage />);
    expect(screen.getByTestId('heatmap-chart')).toBeInTheDocument();
    expect(screen.getByTestId('heatmap-chart').textContent).toContain('Latency Heatmap (p95)');
  });

  it('renders "Top Routes" card with DataTable', () => {
    render(<AnalyticsPage />);
    expect(screen.getByText('Top Routes')).toBeInTheDocument();
    expect(screen.getByText('Highest volume route endpoints.')).toBeInTheDocument();
    const tables = screen.getAllByTestId('data-table');
    expect(tables.length).toBeGreaterThanOrEqual(1);
  });

  it('renders "Top Consumers" card with DataTable', () => {
    render(<AnalyticsPage />);
    expect(screen.getByText('Top Consumers')).toBeInTheDocument();
    expect(screen.getByText('Most active consumers in current window.')).toBeInTheDocument();
    const tables = screen.getAllByTestId('data-table');
    expect(tables.length).toBeGreaterThanOrEqual(2);
  });

  it('renders geo distribution chart when data exists', () => {
    render(<AnalyticsPage />);
    expect(screen.getByTestId('geo-chart')).toBeInTheDocument();
    expect(screen.getByTestId('geo-chart').textContent).toContain('1 countries');
  });

  it('renders rate limit stats when data exists', () => {
    render(<AnalyticsPage />);
    expect(screen.getByTestId('rate-limit-stats')).toBeInTheDocument();
  });

  it('shows loading state when data is undefined', () => {
    vi.mocked(useAnalyticsTimeseries).mockReturnValue({ data: undefined } as any);
    vi.mocked(useAnalyticsTopRoutes).mockReturnValue({ data: undefined } as any);
    vi.mocked(useQuery).mockReturnValue({ data: undefined } as any);

    render(<AnalyticsPage />);

    // Area chart renders with 0 points when no data
    expect(screen.getByTestId('area-chart')).toBeInTheDocument();
    expect(screen.getByTestId('area-chart').textContent).toContain('0 points');

    // Pie chart renders with 0 slices when no data
    expect(screen.getByTestId('pie-chart')).toBeInTheDocument();
    expect(screen.getByTestId('pie-chart').textContent).toContain('0 slices');

    // Heatmap chart renders with 0 points when no data
    expect(screen.getByTestId('heatmap-chart')).toBeInTheDocument();
    expect(screen.getByTestId('heatmap-chart').textContent).toContain('0 points');

    // Geo chart and rate limit stats should NOT render when data is missing
    expect(screen.queryByTestId('geo-chart')).not.toBeInTheDocument();
    expect(screen.queryByTestId('rate-limit-stats')).not.toBeInTheDocument();
  });
});
