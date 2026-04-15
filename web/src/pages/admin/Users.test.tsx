import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { UsersPage } from './Users';
import { useUsers, useCreateUser } from '@/hooks/use-users';

// Mock all hooks
vi.mock('@/hooks/use-users', () => ({
  useUsers: vi.fn(),
  useCreateUser: vi.fn(),
}));

vi.mock('react-router-dom', () => ({
  useNavigate: vi.fn(),
}));

// Mock DataTable
vi.mock('@/components/shared/DataTable', () => ({
  DataTable: ({ data, searchPlaceholder }: { data: unknown[]; searchPlaceholder?: string }) => (
    <div data-testid="data-table">{data.length} rows - {searchPlaceholder}</div>
  ),
}));

vi.mock('@/components/shared/StatusBadge', () => ({
  StatusBadge: ({ status }: { status: string }) => <span>{status}</span>,
}));

vi.mock('@/components/shared/CreditBadge', () => ({
  CreditBadge: ({ value }: { value: number }) => <span>{value} credits</span>,
}));

vi.mock('@/components/users/UserRoleManager', () => ({
  UserRoleManager: () => <div>Role Manager</div>,
  BulkUserActions: () => <div>Bulk Actions</div>,
}));

describe('UsersPage', () => {
  beforeEach(() => {
    vi.mocked(useUsers).mockReturnValue({
      data: {
        users: [
          { id: 'u1', name: 'Alice', email: 'alice@test.com', status: 'active', credit_balance: 100, role: 'user' },
          { id: 'u2', name: 'Bob', email: 'bob@test.com', status: 'suspended', credit_balance: 0, role: 'admin' },
        ],
        total: 2,
      },
    } as any);

    vi.mocked(useCreateUser).mockReturnValue({ mutateAsync: vi.fn(), isPending: false } as any);
  });

  it('renders Users heading and description', () => {
    render(<UsersPage />);
    expect(screen.getByText('Users')).toBeInTheDocument();
    expect(screen.getByText('Create and manage portal users, status and balances.')).toBeInTheDocument();
  });

  it('renders search input with placeholder', () => {
    render(<UsersPage />);
    expect(screen.getByPlaceholderText('Search users...')).toBeInTheDocument();
  });

  it('renders New User button', () => {
    render(<UsersPage />);
    expect(screen.getByRole('button', { name: /new user/i })).toBeInTheDocument();
  });

  it('renders status tabs', () => {
    render(<UsersPage />);
    expect(screen.getByText('All')).toBeInTheDocument();
    expect(screen.getByText('Active')).toBeInTheDocument();
    expect(screen.getByText('Suspended')).toBeInTheDocument();
  });

  it('renders DataTable with user data', () => {
    render(<UsersPage />);
    expect(screen.getByTestId('data-table')).toBeInTheDocument();
    expect(screen.getByText('2 rows - Filter by name...')).toBeInTheDocument();
  });

  it('shows loading state when data is undefined', () => {
    vi.mocked(useUsers).mockReturnValue({ data: undefined } as any);

    render(<UsersPage />);
    expect(screen.getByText('Users')).toBeInTheDocument();
    expect(screen.getByText('0 rows - Filter by name...')).toBeInTheDocument();
  });
});
