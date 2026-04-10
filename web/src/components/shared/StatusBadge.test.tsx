import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { StatusBadge } from './StatusBadge';

describe('StatusBadge', () => {
  it('renders with active status', () => {
    render(<StatusBadge status="active" />);
    const badge = screen.getByText('active');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveClass('bg-success/15');
  });

  it('renders with suspended status', () => {
    render(<StatusBadge status="suspended" />);
    const badge = screen.getByText('suspended');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveClass('bg-destructive/15');
  });

  it('renders with pending status', () => {
    render(<StatusBadge status="pending" />);
    const badge = screen.getByText('pending');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveClass('bg-warning/15');
  });

  it('normalizes status to lowercase', () => {
    render(<StatusBadge status="ACTIVE" />);
    const badge = screen.getByText('ACTIVE');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveClass('bg-success/15');
  });

  it('applies custom className', () => {
    render(<StatusBadge status="active" className="test-class" />);
    const badge = screen.getByText('active');
    expect(badge).toHaveClass('test-class');
  });

  it('renders unknown status with default style', () => {
    render(<StatusBadge status="unknown" />);
    const badge = screen.getByText('unknown');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveClass('bg-muted');
  });

  it('capitalizes the status text', () => {
    render(<StatusBadge status="active" />);
    const badge = screen.getByText('active');
    // Badge applies capitalize class
    expect(badge).toHaveClass('capitalize');
  });
});
