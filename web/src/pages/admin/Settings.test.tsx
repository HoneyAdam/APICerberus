import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { SettingsPage } from './Settings';

vi.mock('@/lib/api', () => ({
  adminApiRequest: vi.fn(),
}));

const renderWithQueryClient = (ui: React.ReactNode) => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
  );
};

describe('SettingsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders the portal config section', () => {
    renderWithQueryClient(<SettingsPage />);
    expect(screen.getByText('Portal Config')).toBeInTheDocument();
    expect(screen.getByText('User portal access and path setup.')).toBeInTheDocument();
  });

  it('renders the billing settings section', () => {
    renderWithQueryClient(<SettingsPage />);
    expect(screen.getByText('Billing Settings')).toBeInTheDocument();
    expect(screen.getByText('Default pricing controls.')).toBeInTheDocument();
  });

  it('renders the retention policies section', () => {
    renderWithQueryClient(<SettingsPage />);
    expect(screen.getByText('Retention Policies')).toBeInTheDocument();
    expect(screen.getByText('Audit log cleanup policy controls.')).toBeInTheDocument();
  });

  it('has default portal path value', () => {
    renderWithQueryClient(<SettingsPage />);
    const portalInput = screen.getByDisplayValue('/portal');
    expect(portalInput).toBeInTheDocument();
  });

  it('has save billing button', () => {
    renderWithQueryClient(<SettingsPage />);
    expect(screen.getByRole('button', { name: 'Save Billing' })).toBeInTheDocument();
  });

  it('has run cleanup button', () => {
    renderWithQueryClient(<SettingsPage />);
    expect(screen.getByRole('button', { name: 'Run Cleanup' })).toBeInTheDocument();
  });

  it('has default retention values', () => {
    renderWithQueryClient(<SettingsPage />);
    expect(screen.getByDisplayValue('30')).toBeInTheDocument();
    expect(screen.getByDisplayValue('500')).toBeInTheDocument();
  });

  it('has portal enabled switch label', () => {
    renderWithQueryClient(<SettingsPage />);
    expect(screen.getByText('Portal Enabled')).toBeInTheDocument();
  });

  it('has billing enabled switch label', () => {
    renderWithQueryClient(<SettingsPage />);
    expect(screen.getByText('Billing Enabled')).toBeInTheDocument();
  });
});
