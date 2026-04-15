import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { useQuery } from '@tanstack/react-query';
import { CreditsPage } from './Credits';
import { useCreditsOverview, useUserCreditTransactions } from '@/hooks/use-credits';
import { useAnalyticsTimeseries } from '@/hooks/use-analytics';

// Mock hooks
vi.mock('@/hooks/use-credits', () => ({
  useCreditsOverview: vi.fn(),
  useUserCreditTransactions: vi.fn(),
}));

vi.mock('@/hooks/use-analytics', () => ({
  useAnalyticsTimeseries: vi.fn(),
}));

vi.mock('@/lib/api', () => ({
  adminApiRequest: vi.fn(),
}));

// Mock @tanstack/react-query for direct useQuery / useMutation / useQueryClient calls
vi.mock('@tanstack/react-query', () => ({
  useQuery: vi.fn(),
  useMutation: vi.fn(() => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false })),
  useQueryClient: vi.fn(() => ({ invalidateQueries: vi.fn() })),
}));

// Mock shared components
vi.mock('@/components/shared/DataTable', () => ({
  DataTable: ({ data, searchPlaceholder }: { data: unknown[]; searchPlaceholder?: string }) => (
    <div data-testid="data-table">{data.length} rows - {searchPlaceholder}</div>
  ),
}));

vi.mock('@/components/shared/KPICard', () => ({
  KPICard: ({ label, value }: { label: string; value: unknown }) => (
    <div data-testid="kpi-card">{label}: {String(value)}</div>
  ),
}));

vi.mock('@/components/shared/TimeAgo', () => ({
  TimeAgo: ({ value }: { value: string }) => <span>{value}</span>,
}));

vi.mock('@/components/charts/BarChart', () => ({
  BarChart: ({ title, data }: { title: string; data: unknown[] }) => (
    <div data-testid="bar-chart">{title} - {data.length} points</div>
  ),
}));

describe('CreditsPage', () => {
  beforeEach(() => {
    vi.mocked(useCreditsOverview).mockReturnValue({
      data: {
        total_distributed: 50000,
        total_consumed: 30000,
        top_consumers: [{ user_id: 'u1', name: 'Alice', consumed: 5000 }],
      },
    } as any);

    vi.mocked(useAnalyticsTimeseries).mockReturnValue({
      data: { items: [{ timestamp: '2026-04-10T12:00:00Z', credits_consumed: 100, errors: 2 }] },
    } as any);

    vi.mocked(useUserCreditTransactions).mockReturnValue({
      data: {
        transactions: [{ type: 'deduct', amount: -5, created_at: '2026-04-10T12:00:00Z' }],
        total: 1,
      },
    } as any);

    vi.mocked(useQuery).mockImplementation(({ queryKey }: any) => {
      if (queryKey[0] === 'billing-config')
        return {
          data: {
            enabled: true,
            default_cost: 1,
            zero_balance_action: 'block',
            method_multipliers: { GET: 1 },
          },
        };
      if (queryKey[0] === 'billing-route-costs')
        return { data: { route_costs: { '/api/users': 2 } } };
      return { data: undefined };
    });
  });

  it('renders KPI cards for Total Distributed, Total Consumed, and Top Consumers', () => {
    render(<CreditsPage />);
    expect(screen.getAllByTestId('kpi-card')).toHaveLength(3);
    expect(screen.getByText(/Total Distributed: 50000/)).toBeInTheDocument();
    expect(screen.getByText(/Total Consumed: 30000/)).toBeInTheDocument();
    expect(screen.getByText(/Top Consumers: 1/)).toBeInTheDocument();
  });

  it('renders the bar chart', () => {
    render(<CreditsPage />);
    expect(screen.getByTestId('bar-chart')).toBeInTheDocument();
    expect(screen.getByText(/Credit Consumption \(24h\)/)).toBeInTheDocument();
  });

  it('renders the transactions section', () => {
    render(<CreditsPage />);
    expect(screen.getByText('Transactions')).toBeInTheDocument();
    expect(screen.getByText('Alice')).toBeInTheDocument();
    expect(screen.getByTestId('data-table')).toBeInTheDocument();
  });

  it('renders pricing editor with textarea', () => {
    render(<CreditsPage />);
    expect(screen.getByText('Pricing Editor')).toBeInTheDocument();
    expect(screen.getByText('Method Multipliers')).toBeInTheDocument();
    expect(screen.getByLabelText('Method Multipliers')).toBeInTheDocument();
    expect(screen.getByText('Save Pricing')).toBeInTheDocument();
  });

  it('renders route costs editor with textarea', () => {
    render(<CreditsPage />);
    expect(screen.getByText('Route Costs')).toBeInTheDocument();
    expect(screen.getByText('Route Costs JSON')).toBeInTheDocument();
    expect(screen.getByLabelText('Route Costs JSON')).toBeInTheDocument();
    expect(screen.getByText('Save Route Costs')).toBeInTheDocument();
  });

  it('shows loading state when data is undefined', () => {
    vi.mocked(useCreditsOverview).mockReturnValue({ data: undefined } as any);
    vi.mocked(useAnalyticsTimeseries).mockReturnValue({ data: undefined } as any);
    vi.mocked(useUserCreditTransactions).mockReturnValue({ data: undefined } as any);
    vi.mocked(useQuery).mockReturnValue({ data: undefined } as any);

    render(<CreditsPage />);
    expect(screen.getByText(/Total Distributed: 0/)).toBeInTheDocument();
    expect(screen.getByText(/Total Consumed: 0/)).toBeInTheDocument();
    expect(screen.getByText(/Top Consumers: 0/)).toBeInTheDocument();
    expect(screen.getByTestId('bar-chart')).toBeInTheDocument();
  });
});
