import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { GeoDistributionChart, WorldMapVisualization } from './GeoDistributionChart';
import type { GeoDataPoint } from './types';

const mockGeoData: GeoDataPoint[] = [
  { country: 'United States', countryCode: 'US', requests: 1000, errors: 10, avgLatencyMs: 45, uniqueIps: 500 },
  { country: 'Germany', countryCode: 'DE', requests: 500, errors: 5, avgLatencyMs: 60, uniqueIps: 200 },
  { country: 'United Kingdom', countryCode: 'GB', requests: 400, errors: 3, avgLatencyMs: 55, uniqueIps: 150 },
  { country: 'Japan', countryCode: 'JP', requests: 300, errors: 2, avgLatencyMs: 120, uniqueIps: 100 },
  { country: 'France', countryCode: 'FR', requests: 200, errors: 1, avgLatencyMs: 58, uniqueIps: 80 },
];

describe('GeoDistributionChart', () => {
  it('renders with title and description', () => {
    render(<GeoDistributionChart data={mockGeoData} />);

    expect(screen.getByText('Geographic Distribution')).toBeInTheDocument();
    expect(screen.getByText(/request distribution/i)).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    render(<GeoDistributionChart data={mockGeoData} title="Custom Title" description="Custom description" />);

    expect(screen.getByText('Custom Title')).toBeInTheDocument();
    expect(screen.getByText('Custom description')).toBeInTheDocument();
  });

  it('displays country count badge', () => {
    render(<GeoDistributionChart data={mockGeoData} />);

    expect(screen.getByText('5 countries')).toBeInTheDocument();
  });

  it('displays top countries', () => {
    render(<GeoDistributionChart data={mockGeoData} />);

    expect(screen.getByText('United States')).toBeInTheDocument();
    expect(screen.getByText('Germany')).toBeInTheDocument();
    expect(screen.getByText('United Kingdom')).toBeInTheDocument();
  });

  it('displays request counts', () => {
    render(<GeoDistributionChart data={mockGeoData} />);

    expect(screen.getAllByText(/1[.,]000/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/500/).length).toBeGreaterThan(0);
  });

  it('displays percentages', () => {
    render(<GeoDistributionChart data={mockGeoData} />);

    // US has 1000/2400 = 41.7%
    expect(screen.getByText('41.7%')).toBeInTheDocument();
  });

  it('displays latency information', () => {
    render(<GeoDistributionChart data={mockGeoData} />);

    expect(screen.getAllByText(/avg \d+ms/).length).toBeGreaterThan(0);
  });

  it('displays unique IP counts', () => {
    render(<GeoDistributionChart data={mockGeoData} />);

    expect(screen.getAllByText(/\d+ unique IPs/).length).toBeGreaterThan(0);
  });

  it('limits items based on maxItems prop', () => {
    render(<GeoDistributionChart data={mockGeoData} maxItems={3} />);

    // Should only show top 3 countries
    expect(screen.getByText('United States')).toBeInTheDocument();
    expect(screen.getByText('Germany')).toBeInTheDocument();
    expect(screen.getByText('United Kingdom')).toBeInTheDocument();
    expect(screen.queryByText('France')).not.toBeInTheDocument();
  });

  it('shows empty state when no data', () => {
    render(<GeoDistributionChart data={[]} />);

    expect(screen.getByText('No geographic data available')).toBeInTheDocument();
  });

  it('displays region stats', () => {
    render(<GeoDistributionChart data={mockGeoData} />);

    // Should show region breakdown
    expect(screen.getByText('North America')).toBeInTheDocument();
    expect(screen.getByText('Europe')).toBeInTheDocument();
  });
});

describe('WorldMapVisualization', () => {
  it('renders SVG map', () => {
    const { container } = render(<WorldMapVisualization data={mockGeoData} />);

    expect(container.querySelector('svg')).toBeInTheDocument();
  });

  it('renders legend', () => {
    render(<WorldMapVisualization data={mockGeoData} />);

    expect(screen.getByText('Low')).toBeInTheDocument();
    expect(screen.getByText('High')).toBeInTheDocument();
  });
});
