import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { RateLimitStatsCard } from './RateLimitStats';
import type { RateLimitStats } from './types';

const mockStats: RateLimitStats = {
  totalRequests: 10000,
  allowed: 9500,
  throttled: 400,
  blocked: 100,
  byWindow: [
    { window: '1m', allowed: 1000, throttled: 50, blocked: 10, avgLatencyMs: 45 },
    { window: '5m', allowed: 5000, throttled: 200, blocked: 50, avgLatencyMs: 50 },
    { window: '1h', allowed: 9500, throttled: 400, blocked: 100, avgLatencyMs: 55 },
  ],
  byRoute: [
    { routeId: 'route-1', routeName: 'API Route 1', allowed: 5000, throttled: 200, blocked: 50 },
    { routeId: 'route-2', routeName: 'API Route 2', allowed: 3000, throttled: 150, blocked: 30 },
    { routeId: 'route-3', routeName: 'API Route 3', allowed: 1500, throttled: 50, blocked: 20 },
  ],
  byConsumer: [
    { consumerId: 'consumer-1', consumerName: 'Consumer A', allowed: 4000, throttled: 300, blocked: 80 },
    { consumerId: 'consumer-2', consumerName: 'Consumer B', allowed: 3000, throttled: 100, blocked: 20 },
  ],
};

describe('RateLimitStatsCard', () => {
  it('renders with title and description', () => {
    render(<RateLimitStatsCard data={mockStats} />);

    expect(screen.getByText('Rate Limiting Statistics')).toBeInTheDocument();
    expect(screen.getByText(/request throttling/i)).toBeInTheDocument();
  });

  it('displays total requests badge', () => {
    render(<RateLimitStatsCard data={mockStats} />);

    expect(screen.getByText(/10[.,]000 total/)).toBeInTheDocument();
  });

  it('displays overview stats', () => {
    render(<RateLimitStatsCard data={mockStats} />);

    expect(screen.getByText('Allowed')).toBeInTheDocument();
    expect(screen.getByText(/9[.,]500/)).toBeInTheDocument();
    expect(screen.getByText('95.0%')).toBeInTheDocument();

    expect(screen.getByText('Throttled')).toBeInTheDocument();
    expect(screen.getByText('400')).toBeInTheDocument();
    expect(screen.getByText('4.0%')).toBeInTheDocument();

    expect(screen.getByText('Blocked')).toBeInTheDocument();
    expect(screen.getByText('100')).toBeInTheDocument();
    expect(screen.getByText('1.0%')).toBeInTheDocument();
  });

  it('displays time window breakdown', () => {
    render(<RateLimitStatsCard data={mockStats} />);

    expect(screen.getByText('By Time Window')).toBeInTheDocument();
    expect(screen.getByText('1m')).toBeInTheDocument();
    expect(screen.getByText('5m')).toBeInTheDocument();
    expect(screen.getByText('1h')).toBeInTheDocument();
  });

  it('displays window request counts', () => {
    render(<RateLimitStatsCard data={mockStats} />);

    expect(screen.getByText(/1[.,]060 requests/)).toBeInTheDocument(); // 1m window
    expect(screen.getByText(/5[.,]250 requests/)).toBeInTheDocument(); // 5m window
  });

  it('displays top affected routes', () => {
    render(<RateLimitStatsCard data={mockStats} />);

    expect(screen.getByText('Top Affected Routes')).toBeInTheDocument();
    expect(screen.getByText('API Route 1')).toBeInTheDocument();
    expect(screen.getByText('API Route 2')).toBeInTheDocument();
  });

  it('displays route stats', () => {
    render(<RateLimitStatsCard data={mockStats} />);

    expect(screen.getAllByText('5000 allowed').length).toBeGreaterThan(0);
    expect(screen.getAllByText('200 throttled').length).toBeGreaterThan(0);
    expect(screen.getAllByText('50 blocked').length).toBeGreaterThan(0);
  });

  it('displays top affected consumers', () => {
    render(<RateLimitStatsCard data={mockStats} />);

    expect(screen.getByText('Top Affected Consumers')).toBeInTheDocument();
    expect(screen.getByText('Consumer A')).toBeInTheDocument();
    expect(screen.getByText('Consumer B')).toBeInTheDocument();
  });

  it('calculates blocked rates correctly', () => {
    render(<RateLimitStatsCard data={mockStats} />);

    // Route 1: (200 + 50) / 5250 = 4.8%
    expect(screen.getAllByText('4.8%').length).toBeGreaterThan(0);
  });

  it('handles zero requests gracefully', () => {
    const emptyStats: RateLimitStats = {
      totalRequests: 0,
      allowed: 0,
      throttled: 0,
      blocked: 0,
      byWindow: [],
      byRoute: [],
      byConsumer: [],
    };

    render(<RateLimitStatsCard data={emptyStats} />);

    expect(screen.getByText('Allowed')).toBeInTheDocument();
    expect(screen.getAllByText('0').length).toBeGreaterThan(0);
  });

  it('hides sections when no data', () => {
    const minimalStats: RateLimitStats = {
      totalRequests: 100,
      allowed: 100,
      throttled: 0,
      blocked: 0,
      byWindow: [],
      byRoute: [],
      byConsumer: [],
    };

    render(<RateLimitStatsCard data={minimalStats} />);

    expect(screen.queryByText('By Time Window')).not.toBeInTheDocument();
    expect(screen.queryByText('Top Affected Routes')).not.toBeInTheDocument();
  });
});
