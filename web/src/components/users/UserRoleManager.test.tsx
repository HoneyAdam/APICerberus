import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { UserRoleManager, BulkUserActions, PERMISSIONS, ROLE_PERMISSIONS } from './UserRoleManager';
import type { User } from '@/lib/types';

// Mock the API
vi.mock('@/lib/api', () => ({
  adminApiRequest: vi.fn(),
}));

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
};

const mockUser: User = {
  id: 'user-1',
  email: 'test@example.com',
  name: 'Test User',
  role: 'user',
  status: 'active',
  credit_balance: 100,
};

describe('UserRoleManager', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders manage role button', () => {
    render(<UserRoleManager user={mockUser} />, { wrapper: createWrapper() });

    expect(screen.getByRole('button', { name: /manage role/i })).toBeInTheDocument();
  });

  it('opens dialog when button is clicked', () => {
    render(<UserRoleManager user={mockUser} />, { wrapper: createWrapper() });

    const button = screen.getByRole('button', { name: /manage role/i });
    fireEvent.click(button);

    expect(screen.getByText('Manage User Role')).toBeInTheDocument();
    expect(screen.getByText(/configure role and permissions/i)).toBeInTheDocument();
  });

  it('displays user information in dialog', () => {
    render(<UserRoleManager user={mockUser} />, { wrapper: createWrapper() });

    const button = screen.getByRole('button', { name: /manage role/i });
    fireEvent.click(button);

    expect(screen.getByText(mockUser.name)).toBeInTheDocument();
    expect(screen.getByText(mockUser.email)).toBeInTheDocument();
  });

  it('displays all role options', () => {
    render(<UserRoleManager user={mockUser} />, { wrapper: createWrapper() });

    const button = screen.getByRole('button', { name: /manage role/i });
    fireEvent.click(button);

    expect(screen.getByText('Administrator')).toBeInTheDocument();
    expect(screen.getByText('Manager')).toBeInTheDocument();
    expect(screen.getByText('User')).toBeInTheDocument();
    expect(screen.getByText('Viewer')).toBeInTheDocument();
  });

  it('selects a role', () => {
    render(<UserRoleManager user={mockUser} />, { wrapper: createWrapper() });

    const button = screen.getByRole('button', { name: /manage role/i });
    fireEvent.click(button);

    const adminRole = screen.getByText('Administrator').closest('button');
    fireEvent.click(adminRole!);

    expect(adminRole).toHaveClass('border-primary');
  });

  it('displays permissions by category', () => {
    render(<UserRoleManager user={mockUser} />, { wrapper: createWrapper() });

    const button = screen.getByRole('button', { name: /manage role/i });
    fireEvent.click(button);

    expect(screen.getByText('Gateway')).toBeInTheDocument();
    expect(screen.getByText('Users')).toBeInTheDocument();
    expect(screen.getByText('Credits')).toBeInTheDocument();
    expect(screen.getByText('System')).toBeInTheDocument();
  });

  it('toggles permissions', () => {
    render(<UserRoleManager user={mockUser} />, { wrapper: createWrapper() });

    const button = screen.getByRole('button', { name: /manage role/i });
    fireEvent.click(button);

    const permission = screen.getByLabelText('View Services');
    fireEvent.click(permission);

    expect(permission).not.toBeChecked();
  });

  it('shows custom badge when permissions are customized', () => {
    render(<UserRoleManager user={mockUser} />, { wrapper: createWrapper() });

    const button = screen.getByRole('button', { name: /manage role/i });
    fireEvent.click(button);

    const permission = screen.getByLabelText('View Services');
    fireEvent.click(permission);

    expect(screen.getByText('Custom')).toBeInTheDocument();
  });

  it('calls onUpdate after saving', async () => {
    const onUpdate = vi.fn();
    const { adminApiRequest } = await import('@/lib/api');
    vi.mocked(adminApiRequest).mockResolvedValue({});

    render(<UserRoleManager user={mockUser} onUpdate={onUpdate} />, { wrapper: createWrapper() });

    const button = screen.getByRole('button', { name: /manage role/i });
    fireEvent.click(button);

    const adminRole = screen.getByText('Administrator').closest('button');
    fireEvent.click(adminRole!);

    const saveButton = screen.getByRole('button', { name: /save changes/i });
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(onUpdate).toHaveBeenCalled();
    });
  });
});

describe('BulkUserActions', () => {
  it('renders nothing when no users selected', () => {
    render(<BulkUserActions selectedUserIds={[]} />, { wrapper: createWrapper() });

    expect(screen.queryByText(/selected/i)).not.toBeInTheDocument();
  });

  it('renders when users are selected', () => {
    render(<BulkUserActions selectedUserIds={['user-1', 'user-2']} />, { wrapper: createWrapper() });

    expect(screen.getByText('2 selected')).toBeInTheDocument();
  });

  it('has activate button', () => {
    render(<BulkUserActions selectedUserIds={['user-1']} />, { wrapper: createWrapper() });

    expect(screen.getByRole('button', { name: /activate/i })).toBeInTheDocument();
  });

  it('has suspend button', () => {
    render(<BulkUserActions selectedUserIds={['user-1']} />, { wrapper: createWrapper() });

    expect(screen.getByRole('button', { name: /suspend/i })).toBeInTheDocument();
  });

  it('has change role button', () => {
    render(<BulkUserActions selectedUserIds={['user-1']} />, { wrapper: createWrapper() });

    expect(screen.getByRole('button', { name: /change role/i })).toBeInTheDocument();
  });

  it('shows role selector when change role is clicked', () => {
    render(<BulkUserActions selectedUserIds={['user-1']} />, { wrapper: createWrapper() });

    const changeRoleButton = screen.getByRole('button', { name: /change role/i });
    fireEvent.click(changeRoleButton);

    expect(screen.getByRole('combobox')).toBeInTheDocument();
  });
});

describe('PERMISSIONS', () => {
  it('has the expected permissions', () => {
    expect(PERMISSIONS).toHaveLength(20);

    const permissionIds = PERMISSIONS.map((p) => p.id);
    expect(permissionIds).toContain('services:read');
    expect(permissionIds).toContain('services:write');
    expect(permissionIds).toContain('users:read');
    expect(permissionIds).toContain('credits:write');
  });

  it('has valid categories', () => {
    const categories = [...new Set(PERMISSIONS.map((p) => p.category))];
    expect(categories).toContain('Gateway');
    expect(categories).toContain('Users');
    expect(categories).toContain('Credits');
    expect(categories).toContain('System');
  });
});

describe('ROLE_PERMISSIONS', () => {
  it('admin has all permissions', () => {
    expect(ROLE_PERMISSIONS.admin).toHaveLength(PERMISSIONS.length);
  });

  it('viewer has limited permissions', () => {
    expect(ROLE_PERMISSIONS.viewer.length).toBeLessThan(ROLE_PERMISSIONS.admin.length);
    expect(ROLE_PERMISSIONS.viewer).toContain('services:read');
    expect(ROLE_PERMISSIONS.viewer).not.toContain('services:write');
  });

  it('user has read and some write permissions', () => {
    expect(ROLE_PERMISSIONS.user).toContain('services:read');
    expect(ROLE_PERMISSIONS.user).toContain('credits:read');
    expect(ROLE_PERMISSIONS.user).not.toContain('users:write');
  });
});
