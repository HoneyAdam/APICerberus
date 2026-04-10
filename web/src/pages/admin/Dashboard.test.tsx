import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { DashboardPage } from './Dashboard';
import { useAnalyticsOverview, useAnalyticsTimeseries, useAnalyticsTopRoutes } from '@/hooks/use-analytics';
import { useRealtime } from '@/hooks/use-realtime';
import { useUsers } from '@/hooks/use-users';

// Mock all hooks
vi.mock('@/hooks/use-analytics', () => ({
  useAnalyticsOverview: vi.fn(),
  useAnalyticsTimeseries: vi.fn(),
  useAnalyticsTopRoutes: vi.fn(),
}));

vi.mock('@/hooks/use-realtime', () => ({
  useRealtime: vi.fn(),
}));

vi.mock('@/hooks/use-users', () => ({
  useUsers: vi.fn(),
}));

// Mock ScrollArea to avoid ref issues
vi.mock('@/components/ui/scroll-area', () => ({
  ScrollArea: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
}));

// Mock TimeAgo
vi.mock('@/components/shared/TimeAgo', () => ({
  TimeAgo: ({ value }: { value: string }) => <span>{value}</span>,
}));

// Mock AreaChart
vi.mock('@/components/charts/AreaChart', () => ({
  AreaChart: ({ data }: { data: unknown[] }) => (
    <div data-testid="area-chart">{data.length} data points</div>
  ),
}));

// Mock DataTable
vi.mock('@/components/shared/DataTable', () => ({
  DataTable: ({ data, searchPlaceholder }: { data: unknown[]; searchPlaceholder?: string }) => (
    <div data-testid="data-table">{data.length} rows - {searchPlaceholder}</div>
  ),
}));

describe('DashboardPage', () => {
  beforeEach(() => {
    vi.mocked(useAnalyticsOverview).mockReturnValue({
      data: {
        total_requests: 10000,
        credits_consumed: 5000,
        error_rate: 0.02,
      },
    } as any);

    vi.mocked(useAnalyticsTimeseries).mockReturnValue({
      data: {
        items: [
          { timestamp: '2026-04-10T12:00:00Z', requests: 100, errors: 2 },
        ],
      },
    } as any);

    vi.mocked(useAnalyticsTopRoutes).mockReturnValue({
      data: {
        routes: [
          { route_name: 'api-users', count: 5000 },
          { route_name: 'api-orders', count: 3000 },
        ],
      },
    } as any);

    vi.mocked(useRealtime).mockReturnValue({
      status: 'open',
      connected: true,
      trafficSeries: [],
      requestTail: [
        { timestamp: '2026-04-10T12:00:00Z', method: 'GET', path: '/api/users', status_code: 200, route_name: 'api-users', latency_ms: 45, bytes_out: 1024 },
      ],
    } as any);

    vi.mocked(useUsers).mockReturnValue({
      data: { total: 150 },
    } as any);
  });

  it('renders all KPI cards', () => {
    render(<DashboardPage />);
    expect(screen.getByText('Requests (1h)')).toBeInTheDocument();
    expect(screen.getByText('Users')).toBeInTheDocument();
    expect(screen.getByText('Credits Consumed')).toBeInTheDocument();
    expect(screen.getByText('Error Rate')).toBeInTheDocument();
  });

  it('displays correct KPI values', () => {
    render(<DashboardPage />);
    expect(screen.getByText('10000')).toBeInTheDocument();
    expect(screen.getByText('150')).toBeInTheDocument();
    expect(screen.getByText('5000')).toBeInTheDocument();
    expect(screen.getByText('2.00%')).toBeInTheDocument();
  });

  it('renders the live request tail section', () => {
    render(<DashboardPage />);
    expect(screen.getByText('Live Request Tail')).toBeInTheDocument();
    expect(screen.getByText('WebSocket status:')).toBeInTheDocument();
  });

  it('renders the top routes section', () => {
    render(<DashboardPage />);
    expect(screen.getByText('Top Routes')).toBeInTheDocument();
    expect(screen.getByTestId('data-table')).toBeInTheDocument();
  });

  it('renders the area chart', () => {
    render(<DashboardPage />);
    expect(screen.getByTestId('area-chart')).toBeInTheDocument();
  });

  it('displays realtime event data', () => {
    render(<DashboardPage />);
    expect(screen.getByText('GET /api/users')).toBeInTheDocument();
    expect(screen.getByText('200')).toBeInTheDocument();
    expect(screen.getByText('api-users')).toBeInTheDocument();
  });

  it('shows loading state when data is unavailable', () => {
    vi.mocked(useAnalyticsOverview).mockReturnValue({ data: undefined } as any);
    vi.mocked(useAnalyticsTimeseries).mockReturnValue({ data: undefined } as any);
    vi.mocked(useAnalyticsTopRoutes).mockReturnValue({ data: undefined } as any);
    vi.mocked(useUsers).mockReturnValue({ data: undefined } as any);
    vi.mocked(useRealtime).mockReturnValue({
      status: 'connecting',
      connected: false,
      trafficSeries: [],
      requestTail: [],
    } as any);

    render(<DashboardPage />);
    expect(screen.getByText('Requests (1h)')).toBeInTheDocument();
    // All KPI cards should show fallback 0
    expect(screen.getAllByText('0').length).toBeGreaterThanOrEqual(1);
  });
});
