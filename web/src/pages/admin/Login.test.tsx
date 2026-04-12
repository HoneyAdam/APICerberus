import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AdminLoginPage } from './Login';
import { MemoryRouter } from 'react-router-dom';

const renderWithRouter = (searchParams?: string) => {
  const search = searchParams ? `?${searchParams}` : '';
  return render(
    <MemoryRouter initialEntries={[`/login${search}`]}>
      <AdminLoginPage />
    </MemoryRouter>
  );
};

describe('AdminLoginPage', () => {
  it('renders the login form', () => {
    renderWithRouter();
    // Check form elements are present - the actual text depends on BrandingProvider context
    expect(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Enter admin API key')).toBeInTheDocument();
    expect(screen.getByText('Enter your admin API key to access the dashboard.')).toBeInTheDocument();
  });

  it('shows error message for missing_key', () => {
    renderWithRouter('login=missing_key');
    expect(screen.getByText('Admin key is required.')).toBeInTheDocument();
  });

  it('shows error message for invalid_key', () => {
    renderWithRouter('login=invalid_key');
    expect(screen.getByText('Invalid admin key. Please try again.')).toBeInTheDocument();
  });

  it('does not show error for unknown login error', () => {
    renderWithRouter('login=unknown');
    expect(screen.queryByText(/error/i)).not.toBeInTheDocument();
  });

  it('renders the form with correct action and method', () => {
    renderWithRouter();
    const form = document.querySelector('form');
    expect(form).toHaveAttribute('action', '/admin/login');
    expect(form).toHaveAttribute('method', 'POST');
  });

  it('renders password input with correct attributes', () => {
    renderWithRouter();
    const input = screen.getByLabelText('Admin API Key');
    expect(input).toHaveAttribute('type', 'password');
    expect(input).toHaveAttribute('name', 'admin_key');
    expect(input).toHaveAttribute('autoComplete', 'current-password');
  });

  it('renders the shield icon', () => {
    renderWithRouter();
    // The shield icon is rendered as an SVG
    const shieldIcon = document.querySelector('svg');
    expect(shieldIcon).toBeInTheDocument();
  });
});
