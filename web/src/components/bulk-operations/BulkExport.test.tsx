import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BulkExport, type ExportEntity } from './BulkExport';

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

describe('BulkExport', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders correctly', () => {
    render(<BulkExport />, { wrapper: createWrapper() });

    expect(screen.getByText(/bulk export/i)).toBeInTheDocument();
    expect(screen.getByText(/export configuration/i)).toBeInTheDocument();
  });

  it('displays all export entities', () => {
    render(<BulkExport />, { wrapper: createWrapper() });

    expect(screen.getByText('Routes')).toBeInTheDocument();
    expect(screen.getByText('Services')).toBeInTheDocument();
    expect(screen.getByText('Upstreams')).toBeInTheDocument();
    expect(screen.getByText('Consumers')).toBeInTheDocument();
    expect(screen.getByText('Plugins')).toBeInTheDocument();
    expect(screen.getByText('Users')).toBeInTheDocument();
    expect(screen.getByText('Audit Logs')).toBeInTheDocument();
  });

  it('selects and deselects entities', () => {
    render(<BulkExport />, { wrapper: createWrapper() });

    const routesCheckbox = screen.getByLabelText(/routes/i);
    fireEvent.click(routesCheckbox);

    expect(screen.getByText('1 selected')).toBeInTheDocument();

    fireEvent.click(routesCheckbox);
    expect(screen.getByText('0 selected')).toBeInTheDocument();
  });

  it('selects all entities', () => {
    render(<BulkExport />, { wrapper: createWrapper() });

    const selectAllButton = screen.getByRole('button', { name: /select all/i });
    fireEvent.click(selectAllButton);

    expect(screen.getByText('7 selected')).toBeInTheDocument();
  });

  it('deselects all entities', () => {
    render(<BulkExport />, { wrapper: createWrapper() });

    const selectAllButton = screen.getByRole('button', { name: /select all/i });
    fireEvent.click(selectAllButton);

    const deselectAllButton = screen.getByRole('button', { name: /deselect all/i });
    fireEvent.click(deselectAllButton);

    expect(screen.getByText('0 selected')).toBeInTheDocument();
  });

  it('switches export formats', () => {
    render(<BulkExport />, { wrapper: createWrapper() });

    const jsonButton = screen.getByRole('button', { name: /json/i });
    const csvButton = screen.getByRole('button', { name: /csv/i });
    const yamlButton = screen.getByRole('button', { name: /yaml/i });

    fireEvent.click(csvButton);
    expect(csvButton).toHaveClass('bg-primary');

    fireEvent.click(yamlButton);
    expect(yamlButton).toHaveClass('bg-primary');

    fireEvent.click(jsonButton);
    expect(jsonButton).toHaveClass('bg-primary');
  });

  it('exports data as JSON', async () => {
    const { adminApiRequest } = await import('@/lib/api');
    vi.mocked(adminApiRequest).mockResolvedValue([
      { id: '1', name: 'route1' },
      { id: '2', name: 'route2' },
    ]);

    const createElementSpy = vi.spyOn(document, 'createElement');

    render(<BulkExport />, { wrapper: createWrapper() });

    const routesCheckbox = screen.getByLabelText(/routes/i);
    fireEvent.click(routesCheckbox);

    const exportButton = screen.getByRole('button', { name: /^export$/i });
    fireEvent.click(exportButton);

    await waitFor(() => {
      expect(createElementSpy).toHaveBeenCalledWith('a');
    });
  });

  it('disables export button when no entities selected', () => {
    render(<BulkExport />, { wrapper: createWrapper() });

    const exportButton = screen.getByRole('button', { name: /^export$/i });
    expect(exportButton).toBeDisabled();
  });

  it('shows loading state during export', async () => {
    const { adminApiRequest } = await import('@/lib/api');
    vi.mocked(adminApiRequest).mockImplementation(
      () => new Promise((resolve) => setTimeout(() => resolve([]), 100))
    );

    render(<BulkExport />, { wrapper: createWrapper() });

    const routesCheckbox = screen.getByLabelText(/routes/i);
    fireEvent.click(routesCheckbox);

    const exportButton = screen.getByRole('button', { name: /^export$/i });
    fireEvent.click(exportButton);

    expect(screen.getByText(/exporting/i)).toBeInTheDocument();
  });

  it('exports multiple entities', async () => {
    const { adminApiRequest } = await import('@/lib/api');
    vi.mocked(adminApiRequest).mockResolvedValue([]);

    render(<BulkExport />, { wrapper: createWrapper() });

    const routesCheckbox = screen.getByLabelText(/routes/i);
    const servicesCheckbox = screen.getByLabelText(/services/i);

    fireEvent.click(routesCheckbox);
    fireEvent.click(servicesCheckbox);

    expect(screen.getByText('2 selected')).toBeInTheDocument();

    const exportButton = screen.getByRole('button', { name: /^export$/i });
    fireEvent.click(exportButton);

    await waitFor(() => {
      expect(adminApiRequest).toHaveBeenCalledTimes(2);
    });
  });
});
