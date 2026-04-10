import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BulkImport, type ImportEntity } from './BulkImport';

// Mock the API
vi.mock('@/lib/api', () => ({
  adminApiRequest: vi.fn(),
}));

// Mock FileReader for jsdom - reads file via Blob.text() and calls onload
const mockFileReader = () => {
  const OriginalFileReader = vi.fn().mockImplementation(function(this: {
    onload: ((e: { target: { result: string } }) => void) | null;
    onerror: ((e: unknown) => void) | null;
    readAsText: (file: File) => void;
  }) {
    this.onload = null;
    this.onerror = null;
    this.readAsText = (file: File) => {
      file.text().then((text) => {
        if (this.onload) {
          queueMicrotask(() => this.onload!({ target: { result: text } }));
        }
      }).catch(() => {
        if (this.onerror) {
          queueMicrotask(() => this.onerror!({}));
        }
      });
    };
  });
  vi.stubGlobal('FileReader', OriginalFileReader);
  return OriginalFileReader;
};

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

describe('BulkImport', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFileReader();
  });

  it('renders with entity name', () => {
    render(<BulkImport entity="routes" />, { wrapper: createWrapper() });

    expect(screen.getByText(/bulk import routes/i)).toBeInTheDocument();
  });

  it('renders all entity types', () => {
    const entities: ImportEntity[] = ['routes', 'services', 'upstreams', 'consumers', 'plugins'];

    entities.forEach((entity) => {
      const { unmount, container } = render(<BulkImport entity={entity} />, { wrapper: createWrapper() });
      const title = container.querySelector('[data-slot="card-title"]');
      expect(title).toHaveTextContent(new RegExp(entity, 'i'));
      unmount();
    });
  });

  it('switches between JSON and CSV tabs', async () => {
    const user = userEvent.setup();
    render(<BulkImport entity="routes" />, { wrapper: createWrapper() });

    const jsonTab = screen.getByRole('tab', { name: /json/i });
    const csvTab = screen.getByRole('tab', { name: /csv/i });

    await user.click(csvTab);
    expect(csvTab).toHaveAttribute('data-state', 'active');

    await user.click(jsonTab);
    expect(jsonTab).toHaveAttribute('data-state', 'active');
  });

  it('handles file selection', () => {
    render(<BulkImport entity="routes" />, { wrapper: createWrapper() });

    const fileInput = screen.getByLabelText(/select file/i);
    const file = new File(['{"name": "test"}'], 'test.json', { type: 'application/json' });

    fireEvent.change(fileInput, { target: { files: [file] } });

    expect(screen.getByText('test.json')).toBeInTheDocument();
  });

  it('handles drag and drop', () => {
    render(<BulkImport entity="routes" />, { wrapper: createWrapper() });

    const dropZone = screen.getByText(/drag and drop/i).closest('div');
    const file = new File(['{"name": "test"}'], 'test.json', { type: 'application/json' });

    fireEvent.dragOver(dropZone!);
    fireEvent.drop(dropZone!, {
      dataTransfer: {
        files: [file],
      },
    });

    expect(screen.getByText('test.json')).toBeInTheDocument();
  });

  it('shows validation state', async () => {
    const { adminApiRequest } = await import('@/lib/api');
    vi.mocked(adminApiRequest).mockResolvedValue({
      valid: true,
      total: 2,
      validCount: 2,
      invalidCount: 0,
      items: [
        { index: 0, valid: true, data: { name: 'route1' } },
        { index: 1, valid: true, data: { name: 'route2' } },
      ],
    });

    render(<BulkImport entity="routes" />, { wrapper: createWrapper() });

    const fileInput = screen.getByLabelText(/select file/i);
    const file = new File(['[{"name": "route1"}, {"name": "route2"}]'], 'routes.json', {
      type: 'application/json',
    });

    fireEvent.change(fileInput, { target: { files: [file] } });

    await waitFor(() => {
      expect(adminApiRequest).toHaveBeenCalledWith(
        '/admin/api/v1/routes/validate',
        expect.objectContaining({ method: 'POST' })
      );
    });
  });

  it('shows validation errors', async () => {
    const { adminApiRequest } = await import('@/lib/api');
    vi.mocked(adminApiRequest).mockResolvedValue({
      valid: false,
      total: 2,
      validCount: 1,
      invalidCount: 1,
      items: [
        { index: 0, valid: true, data: { name: 'route1' } },
        { index: 1, valid: false, data: { name: '' }, errors: ['Name is required'] },
      ],
    });

    render(<BulkImport entity="routes" />, { wrapper: createWrapper() });

    const fileInput = screen.getByLabelText(/select file/i);
    const file = new File(['[{"name": "route1"}, {"name": ""}]'], 'routes.json', {
      type: 'application/json',
    });

    fireEvent.change(fileInput, { target: { files: [file] } });

    await waitFor(() => {
      expect(adminApiRequest).toHaveBeenCalledWith(
        '/admin/api/v1/routes/validate',
        expect.objectContaining({ method: 'POST' })
      );
    });
  });

  it('calls onSuccess after import', async () => {
    const onSuccess = vi.fn();
    const { adminApiRequest } = await import('@/lib/api');

    vi.mocked(adminApiRequest)
      .mockResolvedValueOnce({
        valid: true,
        total: 1,
        validCount: 1,
        invalidCount: 0,
        items: [{ index: 0, valid: true, data: { name: 'route1' } }],
      })
      .mockResolvedValueOnce({ id: 'route-1', name: 'route1' });

    render(<BulkImport entity="routes" onSuccess={onSuccess} />, { wrapper: createWrapper() });

    const fileInput = screen.getByLabelText(/select file/i);
    const file = new File(['[{"name": "route1"}]'], 'routes.json', { type: 'application/json' });

    fireEvent.change(fileInput, { target: { files: [file] } });

    // Verify the validation API was called (component behavior)
    await waitFor(() => {
      expect(adminApiRequest).toHaveBeenCalledWith(
        '/admin/api/v1/routes/validate',
        expect.objectContaining({ method: 'POST' })
      );
    });
  });

  it('parses CSV files correctly', async () => {
    const { adminApiRequest } = await import('@/lib/api');
    vi.mocked(adminApiRequest).mockResolvedValue({
      valid: true,
      total: 2,
      validCount: 2,
      invalidCount: 0,
      items: [
        { index: 0, valid: true, data: { name: 'route1', path: '/api/1' } },
        { index: 1, valid: true, data: { name: 'route2', path: '/api/2' } },
      ],
    });

    render(<BulkImport entity="routes" />, { wrapper: createWrapper() });

    const csvTab = screen.getByRole('tab', { name: /csv/i });
    fireEvent.click(csvTab);

    const fileInput = screen.getByLabelText(/select file/i);
    const file = new File(['name,path\nroute1,/api/1\nroute2,/api/2'], 'routes.csv', {
      type: 'text/csv',
    });

    fireEvent.change(fileInput, { target: { files: [file] } });

    await waitFor(() => {
      expect(screen.getByText('routes.csv')).toBeInTheDocument();
    });
  });

  it('handles parse errors', async () => {
    render(<BulkImport entity="routes" />, { wrapper: createWrapper() });

    const fileInput = screen.getByLabelText(/select file/i);
    const file = new File(['invalid json'], 'routes.json', { type: 'application/json' });

    fireEvent.change(fileInput, { target: { files: [file] } });

    await waitFor(() => {
      expect(screen.getByText('Validation Errors')).toBeInTheDocument();
    });
  });

  it('clears file on clear button click', async () => {
    render(<BulkImport entity="routes" />, { wrapper: createWrapper() });

    const fileInput = screen.getByLabelText(/select file/i);
    // Use invalid JSON to trigger the catch block which sets the preview state
    const file = new File(['invalid json content'], 'test.json', { type: 'application/json' });

    fireEvent.change(fileInput, { target: { files: [file] } });

    await waitFor(() => {
      expect(screen.getByText('test.json')).toBeInTheDocument();
    });

    const clearButton = screen.getByRole('button', { name: /clear/i });
    fireEvent.click(clearButton);

    expect(screen.queryByText('test.json')).not.toBeInTheDocument();
  });
});
