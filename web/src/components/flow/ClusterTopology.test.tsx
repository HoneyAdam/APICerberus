import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { ClusterTopology } from './ClusterTopology';
import type { ClusterMember } from './ClusterTopology';

// Mock ReactFlow
vi.mock('@xyflow/react', () => ({
  ReactFlow: ({ children, nodes, edges }: {
    children: React.ReactNode;
    nodes: unknown[];
    edges: unknown[];
  }) => (
    <div data-testid="react-flow" data-nodes={nodes?.length} data-edges={edges?.length}>
      {children}
    </div>
  ),
  Background: () => <div data-testid="background" />,
  Controls: () => <div data-testid="controls" />,
  MiniMap: () => <div data-testid="minimap" />,
  Panel: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="panel">{children}</div>
  ),
  Position: {
    TopLeft: 'top-left',
    TopRight: 'top-right',
    BottomLeft: 'bottom-left',
    BottomRight: 'bottom-right',
  },
  useNodesState: vi.fn().mockReturnValue([[], vi.fn(), vi.fn()]),
  useEdgesState: vi.fn().mockReturnValue([[], vi.fn(), vi.fn()]),
  useReactFlow: vi.fn().mockReturnValue({
    fitView: vi.fn(),
  }),
  addEdge: vi.fn(),
  MarkerType: {
    ArrowClosed: 'arrowclosed',
  },
  Handle: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  NodeResizer: () => null,
  NodeToolbar: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

describe('ClusterTopology', () => {
  const mockMembers: ClusterMember[] = [
    {
      id: 'node-1',
      name: 'Leader Node',
      address: '127.0.0.1:12000',
      role: 'leader',
      state: 'healthy',
      lastSeen: new Date().toISOString(),
    },
    {
      id: 'node-2',
      name: 'Follower 1',
      address: '127.0.0.1:12001',
      role: 'follower',
      state: 'healthy',
      lastSeen: new Date().toISOString(),
    },
    {
      id: 'node-3',
      name: 'Follower 2',
      address: '127.0.0.1:12002',
      role: 'follower',
      state: 'healthy',
      lastSeen: new Date().toISOString(),
    },
  ];

  const mockEdges = [
    {
      from: 'node-1',
      to: 'node-2',
      type: 'raft' as const,
      status: 'connected' as const,
      latencyMs: 5,
    },
    {
      from: 'node-1',
      to: 'node-3',
      type: 'raft' as const,
      status: 'connected' as const,
      latencyMs: 8,
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should render topology with members and edges', async () => {
    render(<ClusterTopology members={mockMembers} edges={mockEdges} mode="raft" />);

    await waitFor(() => {
      expect(screen.getByTestId('react-flow')).toBeInTheDocument();
    });

    expect(screen.getByTestId('background')).toBeInTheDocument();
    expect(screen.getByTestId('controls')).toBeInTheDocument();
  });

  it('should render standalone mode', async () => {
    render(<ClusterTopology members={[]} edges={[]} mode="standalone" />);

    await waitFor(() => {
      expect(screen.getByTestId('react-flow')).toBeInTheDocument();
    });
  });

  it('should render with single member', async () => {
    render(<ClusterTopology members={[mockMembers[0]]} edges={[]} mode="raft" />);

    await waitFor(() => {
      expect(screen.getByTestId('react-flow')).toBeInTheDocument();
    });
  });

  it('should handle unhealthy nodes', async () => {
    const unhealthyMembers: ClusterMember[] = [
      {
        id: 'node-1',
        name: 'Leader Node',
        address: '127.0.0.1:12000',
        role: 'leader',
        state: 'healthy',
        lastSeen: new Date().toISOString(),
      },
      {
        id: 'node-2',
        name: 'Unhealthy Node',
        address: '127.0.0.1:12001',
        role: 'follower',
        state: 'unhealthy',
        lastSeen: new Date(Date.now() - 60000).toISOString(),
      },
    ];

    render(<ClusterTopology members={unhealthyMembers} edges={[]} mode="raft" />);

    await waitFor(() => {
      expect(screen.getByTestId('react-flow')).toBeInTheDocument();
    });
  });

  it('should handle candidate node role', async () => {
    const candidateMembers: ClusterMember[] = [
      {
        id: 'node-1',
        name: 'Candidate Node',
        address: '127.0.0.1:12000',
        role: 'candidate',
        state: 'healthy',
        lastSeen: new Date().toISOString(),
      },
    ];

    render(<ClusterTopology members={candidateMembers} edges={[]} mode="raft" />);

    await waitFor(() => {
      expect(screen.getByTestId('react-flow')).toBeInTheDocument();
    });
  });

  it('should render with latency information on edges', async () => {
    const edgesWithLatency = [
      {
        from: 'node-1',
        to: 'node-2',
        type: 'heartbeat' as const,
        status: 'connected' as const,
        latencyMs: 15,
      },
    ];

    render(<ClusterTopology members={mockMembers} edges={edgesWithLatency} mode="raft" />);

    await waitFor(() => {
      expect(screen.getByTestId('react-flow')).toBeInTheDocument();
    });
  });
});
