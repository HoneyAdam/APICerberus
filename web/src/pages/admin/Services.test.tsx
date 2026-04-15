import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ServicesPage } from './Services';
import { useServices, useCreateService } from '@/hooks/use-services';
import { useRoutes } from '@/hooks/use-routes';
import { useUpstreams } from '@/hooks/use-upstreams';

// Mock all hooks
vi.mock('@/hooks/use-services', () => ({
  useServices: vi.fn(),
  useCreateService: vi.fn(),
}));

vi.mock('@/hooks/use-routes', () => ({
  useRoutes: vi.fn(),
}));

vi.mock('@/hooks/use-upstreams', () => ({
  useUpstreams: vi.fn(),
}));

vi.mock('react-router-dom', () => ({
  useNavigate: vi.fn(),
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}));

// Mock DataTable
vi.mock('@/components/shared/DataTable', () => ({
  DataTable: ({ data, searchPlaceholder }: { data: unknown[]; searchPlaceholder?: string }) => (
    <div data-testid="data-table">{data.length} rows - {searchPlaceholder}</div>
  ),
}));

// Mock ServiceGraph
vi.mock('@/components/flow/ServiceGraph', () => ({
  ServiceGraph: () => <div data-testid="service-graph">Graph View</div>,
}));

// Mock StatusBadge
vi.mock('@/components/shared/StatusBadge', () => ({
  StatusBadge: ({ status }: { status: string }) => <span>{status}</span>,
}));

// Mock Dialog components
vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({ children, open, onOpenChange }: { children: React.ReactNode; open: boolean; onOpenChange: (open: boolean) => void }) => (
    <div data-testid="dialog" data-open={open}>{children}</div>
  ),
  DialogTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogDescription: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

// Mock Select components
vi.mock('@/components/ui/select', () => ({
  Select: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectValue: () => <div />,
  SelectContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children }: { children: React.ReactNode; value: string }) => <div>{children}</div>,
}));

describe('ServicesPage', () => {
  beforeEach(() => {
    vi.mocked(useServices).mockReturnValue({
      data: [{ id: 's1', name: 'api', protocol: 'http', upstream: 'up1' }],
    } as any);

    vi.mocked(useCreateService).mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
    } as any);

    vi.mocked(useRoutes).mockReturnValue({ data: [] } as any);
    vi.mocked(useUpstreams).mockReturnValue({ data: [] } as any);
  });

  it('renders "Services" heading and description', () => {
    render(<ServicesPage />);
    expect(screen.getByText('Services')).toBeInTheDocument();
    expect(screen.getByText('Manage API services and map them to upstream pools.')).toBeInTheDocument();
  });

  it('renders "New Service" button', () => {
    render(<ServicesPage />);
    expect(screen.getByRole('button', { name: /new service/i })).toBeInTheDocument();
  });

  it('renders DataTable with services data', () => {
    render(<ServicesPage />);
    const table = screen.getByTestId('data-table');
    expect(table).toBeInTheDocument();
    expect(table.textContent).toContain('1 rows');
    expect(table.textContent).toContain('Search service...');
  });

  it('renders table view by default', () => {
    render(<ServicesPage />);
    expect(screen.getByTestId('data-table')).toBeInTheDocument();
    expect(screen.queryByTestId('service-graph')).not.toBeInTheDocument();
  });

  it('shows loading state when data is undefined', () => {
    vi.mocked(useServices).mockReturnValue({ data: undefined } as any);
    vi.mocked(useRoutes).mockReturnValue({ data: undefined } as any);
    vi.mocked(useUpstreams).mockReturnValue({ data: undefined } as any);

    render(<ServicesPage />);
    const table = screen.getByTestId('data-table');
    expect(table).toBeInTheDocument();
    expect(table.textContent).toContain('0 rows');
  });

  it('has Table/Graph toggle buttons', () => {
    render(<ServicesPage />);
    expect(screen.getByRole('button', { name: 'Table' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Graph' })).toBeInTheDocument();
  });
});
