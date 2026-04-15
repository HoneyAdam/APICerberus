import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { PortalDashboardPage } from './Dashboard';
import {
  usePortalUsageOverview,
  usePortalUsageTimeseries,
  usePortalUsageTopEndpoints,
} from '@/hooks/use-portal';

// Mock all hooks
vi.mock('@/hooks/use-portal', () => ({
  usePortalUsageOverview: vi.fn(),
  usePortalUsageTimeseries: vi.fn(),
  usePortalUsageTopEndpoints: vi.fn(),
}));

// Mock KPICard
vi.mock('@/components/shared/KPICard', () => ({
  KPICard: ({ label, value }: { label: string; value: unknown }) => (
    <div data-testid="kpi-card">{label}: {String(value)}</div>
  ),
}));

// Mock AreaChart
vi.mock('@/components/charts/AreaChart', () => ({
  AreaChart: ({ title, data }: { title: string; data: unknown[] }) => (
    <div data-testid="area-chart">{title} - {data.length} points</div>
  ),
}));

// Mock DataTable
vi.mock('@/components/shared/DataTable', () => ({
  DataTable: ({ data, searchPlaceholder }: { data: unknown[]; searchPlaceholder?: string }) => (
    <div data-testid="data-table">{data.length} rows - {searchPlaceholder}</div>
  ),
}));

describe('PortalDashboardPage', () => {
  beforeEach(() => {
    vi.mocked(usePortalUsageOverview).mockReturnValue({
      data: {
        credit_balance: 5000,
        total_requests: 25000,
        error_rate: 0.015,
        avg_latency_ms: 42.5,
      },
    } as any);

    vi.mocked(usePortalUsageTimeseries).mockReturnValue({
      data: {
        items: [
          { timestamp: '2026-04-10T12:00:00Z', requests: 500, errors: 8 },
        ],
      },
    } as any);

    vi.mocked(usePortalUsageTopEndpoints).mockReturnValue({
      data: {
        items: [
          { route_name: '/api/users', count: 10000 },
        ],
      },
    } as any);
  });

  it('renders Dashboard heading and description', () => {
    render(<PortalDashboardPage />);
    expect(screen.getByText('Dashboard')).toBeInTheDocument();
    expect(screen.getByText('Quick overview of request volume, reliability and credits.')).toBeInTheDocument();
  });

  it('renders all 4 KPI cards', () => {
    render(<PortalDashboardPage />);
    expect(screen.getByText(/Credit Balance/)).toBeInTheDocument();
    expect(screen.getByText(/Requests \(24h\)/)).toBeInTheDocument();
    expect(screen.getByText(/Error Rate/)).toBeInTheDocument();
    expect(screen.getByText(/Avg Latency/)).toBeInTheDocument();
  });

  it('renders the area chart with correct title', () => {
    render(<PortalDashboardPage />);
    expect(screen.getByTestId('area-chart')).toBeInTheDocument();
    expect(screen.getByText(/Request & Error Trend/)).toBeInTheDocument();
  });

  it('renders top endpoints table', () => {
    render(<PortalDashboardPage />);
    expect(screen.getByText('Top Endpoints')).toBeInTheDocument();
    expect(screen.getByTestId('data-table')).toBeInTheDocument();
  });

  it('shows loading state when data is undefined (KPIs show fallback 0)', () => {
    vi.mocked(usePortalUsageOverview).mockReturnValue({ data: undefined } as any);
    vi.mocked(usePortalUsageTimeseries).mockReturnValue({ data: undefined } as any);
    vi.mocked(usePortalUsageTopEndpoints).mockReturnValue({ data: undefined } as any);

    render(<PortalDashboardPage />);
    expect(screen.getByText('Dashboard')).toBeInTheDocument();
    // KPI cards should show fallback 0 values
    expect(screen.getAllByText(/0/).length).toBeGreaterThanOrEqual(1);
  });
});
