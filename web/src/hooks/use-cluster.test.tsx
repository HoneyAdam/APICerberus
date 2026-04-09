import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import {
  useClusterStatus,
  useClusterRealtime,
  type ClusterStatus,
  type ClusterNode,
} from './use-cluster';

// Mock fetch
global.fetch = vi.fn();

// Mock WebSocket
class MockWebSocket {
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  send = vi.fn();
  close = vi.fn();

  constructor() {
    setTimeout(() => this.onopen?.(), 0);
  }
}

global.WebSocket = MockWebSocket as unknown as typeof WebSocket;

// Wrapper for React Query
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });

  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe('useClusterStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('should fetch cluster status successfully', async () => {
    const mockData: ClusterStatus = {
      enabled: true,
      mode: 'raft',
      nodeId: 'node-1',
      leaderId: 'node-1',
      term: 5,
      commitIndex: 100,
      appliedIndex: 100,
      nodes: [
        {
          id: 'node-1',
          name: 'Node 1',
          address: '127.0.0.1:12000',
          role: 'leader',
          state: 'healthy',
          lastSeen: new Date().toISOString(),
        },
      ],
      edges: [],
    };

    (fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => mockData,
    });

    const { result } = renderHook(() => useClusterStatus(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockData);
  });

  it('should return standalone status on 404 error', async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      status: 404,
    });

    const { result } = renderHook(() => useClusterStatus(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.mode).toBe('standalone');
    expect(result.current.data?.enabled).toBe(false);
  });

  it('should return standalone status on network error', async () => {
    (fetch as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error('Network error')
    );

    const { result } = renderHook(() => useClusterStatus(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.mode).toBe('standalone');
  });

  it('should fetch cluster status without explicit auth header', async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        enabled: true,
        mode: 'raft',
        nodeId: 'node-1',
        nodes: [],
        edges: [],
      }),
    });

    renderHook(() => useClusterStatus(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        '/admin/api/v1/cluster/status',
        { credentials: 'same-origin' }
      );
    });
  });
});

describe('useClusterRealtime', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('should establish WebSocket connection', { timeout: 10000 }, async () => {
    const { result } = renderHook(() => useClusterRealtime());

    // Fast-forward timers to trigger connection
    vi.advanceTimersByTime(500);

    await waitFor(() => {
      expect(result.current.isConnected).toBe(true);
    });
  });

  it('should update status on cluster message', { timeout: 10000 }, async () => {
    // Create a mock WebSocket that captures the instance
    let wsInstance: MockWebSocket | null = null;
    const MockWebSocketWithCapture = vi.fn().mockImplementation(() => {
      wsInstance = new MockWebSocket();
      return wsInstance;
    });
    global.WebSocket = MockWebSocketWithCapture as unknown as typeof WebSocket;

    const { result } = renderHook(() => useClusterRealtime());

    // Wait for connection
    vi.advanceTimersByTime(500);
    await waitFor(() => {
      expect(result.current.isConnected).toBe(true);
    });

    // Simulate WebSocket message through the captured instance
    await act(async () => {
      if (wsInstance && wsInstance.onmessage) {
        wsInstance.onmessage({
          data: JSON.stringify({
            type: 'cluster',
            payload: {
              leaderId: 'node-2',
              commitIndex: 50,
            },
          }),
        });
      }
    });

    await waitFor(() => {
      expect(result.current.status.leaderId).toBe('node-2');
      expect(result.current.status.commitIndex).toBe(50);
    });
  });

  it('should handle WebSocket errors gracefully', { timeout: 10000 }, async () => {
    // Mock WebSocket to fail
    const ErrorWebSocket = vi.fn().mockImplementation(() => {
      throw new Error('Connection failed');
    });
    global.WebSocket = ErrorWebSocket as unknown as typeof WebSocket;

    const { result } = renderHook(() => useClusterRealtime());

    vi.advanceTimersByTime(100);

    await waitFor(() => {
      expect(result.current.isConnected).toBe(false);
    });

    // Status should still be available (standalone fallback)
    expect(result.current.status).toBeDefined();
  });
});

describe('ClusterNode interface', () => {
  it('should create a valid cluster node', () => {
    const node: ClusterNode = {
      id: 'test-node',
      name: 'Test Node',
      address: '127.0.0.1:8080',
      role: 'leader',
      state: 'healthy',
      lastSeen: new Date().toISOString(),
      metadata: {
        version: '1.0.0',
      },
    };

    expect(node).toBeDefined();
    expect(node.id).toBe('test-node');
    expect(node.role).toBe('leader');
    expect(node.state).toBe('healthy');
  });
});
