import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { RoutesPage } from './Routes';
import { useRoutes, useCreateRoute } from '@/hooks/use-routes';
import { useServices } from '@/hooks/use-services';

// Mock all hooks
vi.mock('@/hooks/use-routes', () => ({
  useRoutes: vi.fn(),
  useCreateRoute: vi.fn(),
}));

vi.mock('@/hooks/use-services', () => ({
  useServices: vi.fn(),
}));

vi.mock('react-router-dom', () => ({
  useNavigate: vi.fn(),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => <a href={to}>{children}</a>,
}));

// Mock DataTable
vi.mock('@/components/shared/DataTable', () => ({
  DataTable: ({ data, searchPlaceholder }: { data: unknown[]; searchPlaceholder?: string }) => (
    <div data-testid="data-table">{data.length} rows - {searchPlaceholder}</div>
  ),
}));

// Mock Badge
vi.mock('@/components/ui/badge', () => ({
  Badge: ({ children, variant }: { children: React.ReactNode; variant?: string }) => (
    <span data-variant={variant}>{children}</span>
  ),
}));

// Mock Dialog components
vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({ children, open, onOpenChange }: { children: React.ReactNode; open: boolean; onOpenChange: (open: boolean) => void }) => (
    <div data-testid="dialog" data-open={open}>{children}</div>
  ),
  DialogTrigger: ({ children, asChild }: { children: React.ReactNode; asChild?: boolean }) => <>{children}</>,
  DialogContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: React.ReactNode }) => <h3>{children}</h3>,
  DialogDescription: ({ children }: { children: React.ReactNode }) => <p>{children}</p>,
  DialogFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

// Mock Button
vi.mock('@/components/ui/button', () => ({
  Button: ({ children, onClick, disabled, variant, asChild }: { children: React.ReactNode; onClick?: () => void; disabled?: boolean; variant?: string; asChild?: boolean }) => (
    <button onClick={onClick} disabled={disabled} data-variant={variant}>{children}</button>
  ),
}));

// Mock Input
vi.mock('@/components/ui/input', () => ({
  Input: ({ id, value, onChange }: { id?: string; value: string; onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void }) => (
    <input id={id} value={value} onChange={onChange} />
  ),
}));

// Mock Label
vi.mock('@/components/ui/label', () => ({
  Label: ({ children, htmlFor }: { children: React.ReactNode; htmlFor?: string }) => (
    <label htmlFor={htmlFor}>{children}</label>
  ),
}));

// Mock Select components
vi.mock('@/components/ui/select', () => ({
  Select: ({ children, value, onValueChange }: { children: React.ReactNode; value: string; onValueChange?: (v: string) => void }) => (
    <div data-value={value}>{children}</div>
  ),
  SelectTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectValue: ({ placeholder }: { placeholder?: string }) => <span>{placeholder ?? ''}</span>,
  SelectContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children, value }: { children: React.ReactNode; value: string }) => (
    <option value={value}>{children}</option>
  ),
}));

// Mock lucide-react icons
vi.mock('lucide-react', () => ({
  Plus: () => <span>plus-icon</span>,
  Wand2: () => <span>wand2-icon</span>,
}));

describe('RoutesPage', () => {
  beforeEach(() => {
    vi.mocked(useRoutes).mockReturnValue({
      data: [
        { id: 'r1', name: 'users-route', service: 'api', methods: ['GET', 'POST'], paths: ['/users'], plugins: [] },
      ],
    } as any);

    vi.mocked(useCreateRoute).mockReturnValue({ mutateAsync: vi.fn(), isPending: false } as any);

    vi.mocked(useServices).mockReturnValue({ data: [{ id: 's1', name: 'api' }] } as any);
  });

  it('renders "Routes" heading and description', () => {
    render(<RoutesPage />);
    expect(screen.getByText('Routes')).toBeInTheDocument();
    expect(screen.getByText('Define path matching and bind plugin chain behavior.')).toBeInTheDocument();
  });

  it('renders "New Route" button', () => {
    render(<RoutesPage />);
    expect(screen.getByText('New Route')).toBeInTheDocument();
  });

  it('renders "Route Builder" link pointing to /routes/builder', () => {
    render(<RoutesPage />);
    const link = screen.getByText('Route Builder').closest('a');
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute('href', '/routes/builder');
  });

  it('renders DataTable with route data', () => {
    render(<RoutesPage />);
    expect(screen.getByTestId('data-table')).toBeInTheDocument();
    expect(screen.getByText('1 rows - Search route...')).toBeInTheDocument();
  });

  it('shows loading state when data is undefined', () => {
    vi.mocked(useRoutes).mockReturnValue({ data: undefined } as any);
    vi.mocked(useServices).mockReturnValue({ data: undefined } as any);

    render(<RoutesPage />);
    expect(screen.getByTestId('data-table')).toBeInTheDocument();
    expect(screen.getByText('0 rows - Search route...')).toBeInTheDocument();
  });
});
