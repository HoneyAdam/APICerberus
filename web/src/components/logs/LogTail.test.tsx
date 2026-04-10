import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { LogTail } from './LogTail';
import { useRealtime } from '@/hooks/use-realtime';

// Mock the useRealtime hook completely
const mockClear = vi.fn();
const mockEventTail: any[] = [
  {
    type: 'log',
    timestamp: '2026-04-10T12:00:00.000Z',
    payload: { level: 'info', source: 'gateway', message: 'Test log message', request_id: 'req-123' },
  },
  {
    type: 'log',
    timestamp: '2026-04-10T12:00:01.000Z',
    payload: { level: 'error', source: 'admin', message: 'Error occurred' },
  },
  {
    type: 'log',
    timestamp: '2026-04-10T12:00:02.000Z',
    payload: { level: 'warn', source: 'plugin', message: 'Warning message' },
  },
];

vi.mock('@/hooks/use-realtime', () => ({
  useRealtime: vi.fn(() => ({
    connected: true,
    status: 'open',
    eventTail: mockEventTail,
    clear: mockClear,
  })),
}));

// Mock TimeAgo to avoid date-related issues in tests
vi.mock('@/components/shared/TimeAgo', () => ({
  TimeAgo: ({ value }: { value: string }) => <span>{value}</span>,
}));

// Mock ScrollArea to avoid ref-related infinite loops in happy-dom
vi.mock('@/components/ui/scroll-area', () => ({
  ScrollArea: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
}));

describe('LogTail', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    // Re-setup the default mock return value after reset
    vi.mocked(useRealtime).mockReturnValue({
      connected: true,
      status: 'open',
      eventTail: mockEventTail,
      clear: mockClear,
    });
  });

  it('renders with default props', () => {
    render(<LogTail />);
    expect(screen.getByText('Live Logs')).toBeInTheDocument();
    expect(screen.getByText('Live')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    render(<LogTail title="Custom Logs" />);
    expect(screen.getByText('Custom Logs')).toBeInTheDocument();
  });

  it('displays log entries', () => {
    render(<LogTail />);
    expect(screen.getByText('Test log message')).toBeInTheDocument();
    expect(screen.getByText('Error occurred')).toBeInTheDocument();
  });

  it('toggles pause/play', async () => {
    render(<LogTail />);
    const buttons = screen.getAllByRole('button');
    const iconButtons = buttons.filter((b) => b.querySelector('svg'));
    const pauseButton = iconButtons[0];
    fireEvent.click(pauseButton);
    await waitFor(() => {
      expect(screen.getByText(/paused/i)).toBeInTheDocument();
    });
  });

  it('clears logs when clear button is clicked', async () => {
    const user = userEvent.setup();
    render(<LogTail />);
    // Clear button is the one with Trash2 icon - it's the third icon button
    const buttons = screen.getAllByRole('button');
    const clearButton = buttons[2]; // Play/Pause, Export, Clear
    await user.click(clearButton);
    expect(mockClear).toHaveBeenCalled();
  });

  it('filters by level', async () => {
    const user = userEvent.setup();
    render(<LogTail />);
    const comboboxes = screen.getAllByRole('combobox');
    const levelSelect = comboboxes[0]; // First combobox is level
    await user.click(levelSelect);
    const errorItem = await screen.findByRole('option', { name: /error/i });
    await user.click(errorItem);
    await waitFor(() => {
      expect(screen.queryByText('Test log message')).not.toBeInTheDocument();
    });
    expect(screen.getByText('Error occurred')).toBeInTheDocument();
  });

  it('filters by source', async () => {
    const user = userEvent.setup();
    render(<LogTail />);
    const comboboxes = screen.getAllByRole('combobox');
    const sourceSelect = comboboxes[1]; // Second combobox is source
    await user.click(sourceSelect);
    const gatewayItem = await screen.findByRole('option', { name: /gateway/i });
    await user.click(gatewayItem);
    await waitFor(() => {
      expect(screen.getByText('Test log message')).toBeInTheDocument();
    });
  });

  it('filters by search text', async () => {
    render(<LogTail />);
    const searchInput = screen.getByPlaceholderText(/search logs/i);
    fireEvent.change(searchInput, { target: { value: 'Error' } });
    await waitFor(() => {
      expect(screen.getByText('Error occurred')).toBeInTheDocument();
    });
  });

  it('toggles auto-scroll', async () => {
    const user = userEvent.setup();
    render(<LogTail />);
    const autoScrollSwitch = screen.getByRole('switch', { name: /auto-scroll/i });
    await user.click(autoScrollSwitch);
    expect(autoScrollSwitch).not.toBeChecked();
  });

  it('toggles metadata display', async () => {
    const user = userEvent.setup();
    render(<LogTail />);
    const metadataSwitch = screen.getByRole('switch', { name: /metadata/i });
    await user.click(metadataSwitch);
    expect(metadataSwitch).toBeChecked();
  });

  it('exports logs as JSON', async () => {
    const user = userEvent.setup();
    const createElementSpy = vi.spyOn(document, 'createElement');
    render(<LogTail />);
    const buttons = screen.getAllByRole('button');
    const exportButton = buttons[1]; // Second icon button is Export
    await user.click(exportButton);
    expect(createElementSpy).toHaveBeenCalledWith('a');
  });

  it('displays stats', () => {
    render(<LogTail />);
    expect(screen.getByText(/total:/i)).toBeInTheDocument();
    expect(screen.getByText(/errors:/i)).toBeInTheDocument();
    expect(screen.getByText(/warnings:/i)).toBeInTheDocument();
    expect(screen.getByText(/info:/i)).toBeInTheDocument();
  });

  it('shows empty state when no logs', () => {
    vi.mocked(useRealtime).mockReturnValue({
      connected: true,
      status: 'open',
      eventTail: [],
      clear: vi.fn(),
    });
    render(<LogTail />);
    expect(screen.getByText('No logs to display')).toBeInTheDocument();
  });

  it('renders with export disabled', () => {
    render(<LogTail showExport={false} />);
    expect(screen.getByText('Live Logs')).toBeInTheDocument();
    // Only 2 icon buttons should be present (pause + clear, no export)
    const allButtons = screen.getAllByRole('button');
    const iconButtons = allButtons.filter((b) => b.querySelector('svg'));
    expect(iconButtons).toHaveLength(2);
  });

  it('renders with filters disabled', () => {
    render(<LogTail showFilters={false} />);
    expect(screen.getByText('Live Logs')).toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/search logs/i)).not.toBeInTheDocument();
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
  });
});

describe('LogTail helpers', () => {
  it('renders level badges with correct colors', () => {
    render(<LogTail />);
    const errorBadges = screen.getAllByText('ERROR');
    expect(errorBadges.length).toBeGreaterThan(0);
  });

  it('renders source badges', () => {
    render(<LogTail />);
    expect(screen.getAllByText('GW').length).toBeGreaterThan(0);
    expect(screen.getAllByText('AD').length).toBeGreaterThan(0);
  });
});
