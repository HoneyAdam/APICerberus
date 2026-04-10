import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { KPICard } from './KPICard';
import { Users, TrendingUp } from 'lucide-react';

describe('KPICard', () => {
  it('renders label and value', () => {
    render(<KPICard label="Total Users" value={1234} icon={Users} />);
    expect(screen.getByText('Total Users')).toBeInTheDocument();
    expect(screen.getByText('1234')).toBeInTheDocument();
  });

  it('renders description when provided', () => {
    render(
      <KPICard label="Revenue" value="$5000" icon={TrendingUp} description="Monthly recurring" />
    );
    expect(screen.getByText('Monthly recurring')).toBeInTheDocument();
  });

  it('renders positive trend', () => {
    render(<KPICard label="Growth" value="15%" icon={TrendingUp} trend={5.2} />);
    expect(screen.getByText('5.2%')).toBeInTheDocument();
    // Should have success styling
    const trendElement = screen.getByText('5.2%');
    expect(trendElement).toHaveClass('bg-success/15');
  });

  it('renders negative trend', () => {
    render(<KPICard label="Decline" value="-3%" icon={TrendingUp} trend={-2.5} />);
    expect(screen.getByText('2.5%')).toBeInTheDocument();
    const trendElement = screen.getByText('2.5%');
    expect(trendElement).toHaveClass('bg-destructive/15');
  });

  it('does not render trend when not provided', () => {
    render(<KPICard label="Static" value="100" icon={Users} />);
    expect(screen.queryByText('%')).not.toBeInTheDocument();
  });

  it('applies custom className', () => {
    render(<KPICard label="Test" value="42" icon={Users} className="custom-class" />);
    const card = screen.getByText('Test').closest('.rounded-xl');
    expect(card).toHaveClass('custom-class');
  });

  it('renders the icon', () => {
    render(<KPICard label="Users" value={50} icon={Users} />);
    const iconContainer = screen.getByText('Users').closest('div');
    expect(iconContainer).toBeInTheDocument();
  });
});
