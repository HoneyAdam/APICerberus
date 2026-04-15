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

// Mock adminApiRequest — useClusterStatus uses it, not raw fetch
vi.mock('@/lib/api', () => ({
  adminApiRequest: vi.fn(),
}));

vi.mock('@/lib/ws', () => ({
  ReconnectingWebSocketClient: vi.fn().mockImplementation(() => {
    const messageListeners = new Set();
    const statusListeners = new Set();
    let currentStatus = 'idle';

    return {
      connect: vi.fn(() => {
        currentStatus = 'connecting';
        for (const l of statusListeners) l(currentStatus);
        setTimeout(() => {
          currentStatus = 'open';
          for (const l of statusListeners) l(currentStatus);
        }, 0);
      }),
      disconnect: vi.fn(() => {
        currentStatus = 'closed';
        for (const l of statusListeners) l(currentStatus);
      }),
      send: vi.fn(),
      subscribe: vi.fn((listener) => {
        messageListeners.add(listener);
        return () => { messageListeners.delete(listener); };
      }),
      onStatusChange: vi.fn((listener) => {
        statusListeners.add(listener);
        listener(currentStatus);
        return () => { statusListeners.delete(listener); };
      }),
      _simulateMessage: (data) => {
        for (const l of messageListeners) l(data, { data: JSON.stringify(data) });
      },
    };
  }),
}));

import { adminApiRequest } from '@/lib/api';
import { ReconnectingWebSocketClient } from '@/lib/ws';

// Helper to create a fresh mock WS instance for capturing in tests
function createCapturableWSMock() {
  let captured: Record<string, unknown> | null = null;
  const impl = () => {
    const messageListeners = new Set<(msg: unknown, evt: unknown) => void>();
    const statusListeners = new Set<(status: string) => void>();
    let currentStatus = 'idle';
    const instance: Record<string, unknown> = {
      connect: vi.fn(() => {
        currentStatus = 'connecting';
        for (const l of statusListeners) l(currentStatus);
        setTimeout(() => {
          currentStatus = 'open';
          for (const l of statusListeners) l(currentStatus);
        }, 0);
      }),
      disconnect: vi.fn(() => {
        currentStatus = 'closed';
        for (const l of statusListeners) l(currentStatus);
      }),
      send: vi.fn(),
      subscribe: vi.fn((listener: (msg: unknown, evt: unknown) => void) => {
        messageListeners.add(listener);
        return () => { messageListeners.delete(listener); };
      }),
      onStatusChange: vi.fn((listener: (status: string) => void) => {
        statusListeners.add(listener);
        listener(currentStatus);
        return () => { statusListeners.delete(listener); };
      }),
      _simulateMessage: (data: unknown) => {
        for (const l of messageListeners) l(data, { data: JSON.stringify(data) });
      },
    };
    captured = instance;
    return instance;
  };
  return { impl, getCaptured: () => captured };
}

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

    vi.mocked(adminApiRequest).mockResolvedValueOnce(mockData);

    const { result } = renderHook(() => useClusterStatus(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockData);
    expect(adminApiRequest).toHaveBeenCalledWith('/admin/api/v1/cluster/status');
  });

  it('should return standalone status on 404 error', async () => {
    vi.mocked(adminApiRequest).mockRejectedValueOnce(new Error('Not found'));

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
    vi.mocked(adminApiRequest).mockRejectedValueOnce(new Error('Network error'));

    const { result } = renderHook(() => useClusterStatus(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.mode).toBe('standalone');
  });

  it('should call adminApiRequest for cluster status', async () => {
    vi.mocked(adminApiRequest).mockResolvedValueOnce({
      enabled: true,
      mode: 'raft',
      nodeId: 'node-1',
      nodes: [],
      edges: [],
    });

    renderHook(() => useClusterStatus(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(adminApiRequest).toHaveBeenCalledWith('/admin/api/v1/cluster/status');
    });
  });
});

describe('useClusterRealtime', () => {
  // Helper to create a working WS mock instance
  function mockWSInstance() {
    const messageListeners = new Set<(msg: unknown, evt: unknown) => void>();
    const statusListeners = new Set<(status: string) => void>();
    let currentStatus = 'idle';
    return {
      instance: {
        connect: vi.fn(() => {
          currentStatus = 'connecting';
          for (const l of statusListeners) l(currentStatus);
          setTimeout(() => {
            currentStatus = 'open';
            for (const l of statusListeners) l(currentStatus);
          }, 0);
        }),
        disconnect: vi.fn(() => {
          currentStatus = 'closed';
          for (const l of statusListeners) l(currentStatus);
        }),
        send: vi.fn(),
        subscribe: vi.fn((listener: (msg: unknown, evt: unknown) => void) => {
          messageListeners.add(listener);
          return () => { messageListeners.delete(listener); };
        }),
        onStatusChange: vi.fn((listener: (status: string) => void) => {
          statusListeners.add(listener);
          listener(currentStatus);
          return () => { statusListeners.delete(listener); };
        }),
        _simulateMessage: (data: unknown) => {
          for (const l of messageListeners) l(data, { data: JSON.stringify(data) });
        },
      } as Record<string, unknown>,
      messageListeners,
      statusListeners,
    };
  }

  beforeEach(() => {
    vi.clearAllMocks();
    // Re-apply WS mock implementation since clearAllMocks resets it
    vi.mocked(ReconnectingWebSocketClient).mockImplementation(() => mockWSInstance().instance);
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('should establish connection and set isConnected to true', async () => {
    const { result } = renderHook(() => useClusterRealtime());

    // Advance timers to allow the setTimeout(0) in the mock to fire
    await act(async () => {
      vi.advanceTimersByTime(100);
    });

    await waitFor(() => {
      expect(result.current.isConnected).toBe(true);
    });

    // Verify the WS was created and connect was called
    expect(ReconnectingWebSocketClient).toHaveBeenCalled();
  });

  it('should update status on cluster message', async () => {
    // Capture the instance created by the hook
    const { impl, getCaptured } = createCapturableWSMock();
    vi.mocked(ReconnectingWebSocketClient).mockImplementation(impl);

    const { result } = renderHook(() => useClusterRealtime());

    await act(async () => {
      vi.advanceTimersByTime(100);
    });

    await waitFor(() => {
      expect(result.current.isConnected).toBe(true);
    });

    // Simulate a cluster message
    const captured = getCaptured() as unknown as { _simulateMessage: (d: unknown) => void };
    await act(async () => {
      captured._simulateMessage({
        type: 'cluster',
        payload: {
          leaderId: 'node-2',
          commitIndex: 50,
        },
      });
    });

    await waitFor(() => {
      expect(result.current.status.leaderId).toBe('node-2');
      expect(result.current.status.commitIndex).toBe(50);
    });
  });

  it('should handle connection errors gracefully', async () => {
    const { result } = renderHook(() => useClusterRealtime());

    await act(async () => {
      vi.advanceTimersByTime(100);
    });

    // Status should always be defined (standalone fallback)
    expect(result.current.status).toBeDefined();
    expect(result.current.status.mode).toBe('standalone');
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
