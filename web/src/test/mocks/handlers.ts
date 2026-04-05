import { http, HttpResponse, type RequestHandler } from 'msw';

// Base API URL
const API_BASE = '/api/v1';

// Mock data
export const mockClusterNodes = [
  {
    id: 'node-1',
    name: 'Node 1',
    address: '127.0.0.1:12000',
    role: 'leader',
    state: 'healthy',
    lastSeen: new Date().toISOString(),
  },
  {
    id: 'node-2',
    name: 'Node 2',
    address: '127.0.0.1:12001',
    role: 'follower',
    state: 'healthy',
    lastSeen: new Date().toISOString(),
  },
  {
    id: 'node-3',
    name: 'Node 3',
    address: '127.0.0.1:12002',
    role: 'follower',
    state: 'healthy',
    lastSeen: new Date().toISOString(),
  },
];

export const mockClusterStats = {
  mode: 'cluster',
  nodeCount: 3,
  leaderId: 'node-1',
  commitIndex: 42,
};

export const mockGatewayMetrics = {
  requests: {
    total: 1000,
    perSecond: 10.5,
  },
  latency: {
    p50: 45,
    p95: 120,
    p99: 250,
  },
  errors: {
    rate: 0.5,
    count: 5,
  },
  activeConnections: 150,
};

export const mockUpstreamHealth = [
  {
    id: 'upstream-1',
    name: 'User Service',
    healthy: true,
    targets: [
      { id: 'target-1', healthy: true, address: '10.0.0.1:8080' },
      { id: 'target-2', healthy: true, address: '10.0.0.2:8080' },
    ],
  },
];

// Request handlers
export const handlers: RequestHandler[] = [
  // Cluster endpoints
  http.get(`${API_BASE}/cluster/nodes`, () => {
    return HttpResponse.json({ data: mockClusterNodes });
  }),

  http.get(`${API_BASE}/cluster/stats`, () => {
    return HttpResponse.json({ data: mockClusterStats });
  }),

  http.post(`${API_BASE}/cluster/nodes/:id/join`, () => {
    return HttpResponse.json({ success: true });
  }),

  http.post(`${API_BASE}/cluster/nodes/:id/leave`, () => {
    return HttpResponse.json({ success: true });
  }),

  // Gateway metrics
  http.get(`${API_BASE}/gateway/metrics`, () => {
    return HttpResponse.json({ data: mockGatewayMetrics });
  }),

  // Upstream health
  http.get(`${API_BASE}/upstreams/health`, () => {
    return HttpResponse.json({ data: mockUpstreamHealth });
  }),

  // Configuration
  http.get(`${API_BASE}/config`, () => {
    return HttpResponse.json({
      data: {
        gateway: {
          addr: ':8080',
          readTimeout: 30,
          writeTimeout: 30,
        },
        admin: {
          addr: ':9876',
          apiKey: 'test-key',
        },
      },
    });
  }),

  http.put(`${API_BASE}/config`, async ({ request }) => {
    const body = await request.json();
    return HttpResponse.json({ success: true, data: body });
  }),

  // Audit logs
  http.get(`${API_BASE}/audit/logs`, () => {
    return HttpResponse.json({
      data: [
        {
          id: 'log-1',
          timestamp: new Date().toISOString(),
          action: 'CREATE',
          resource: 'apikey',
          userId: 'user-1',
          status: 'success',
        },
      ],
      pagination: {
        total: 100,
        page: 1,
        perPage: 20,
      },
    });
  }),

  // API Keys
  http.get(`${API_BASE}/apikeys`, () => {
    return HttpResponse.json({
      data: [
        {
          id: 'key-1',
          name: 'Test Key',
          key: 'ck_test_xxx',
          createdAt: new Date().toISOString(),
          expiresAt: null,
        },
      ],
    });
  }),

  http.post(`${API_BASE}/apikeys`, async ({ request }) => {
    const body = await request.json();
    return HttpResponse.json({
      success: true,
      data: {
        id: 'key-new',
        ...body,
        key: 'ck_test_new',
        createdAt: new Date().toISOString(),
      },
    });
  }),

  // Error cases
  http.get(`${API_BASE}/error`, () => {
    return HttpResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }),

  http.get(`${API_BASE}/unauthorized`, () => {
    return HttpResponse.json(
      { error: 'Unauthorized' },
      { status: 401 }
    );
  }),
];

// Error handlers
export const errorHandlers: RequestHandler[] = [
  http.get(`${API_BASE}/cluster/nodes`, () => {
    return HttpResponse.json(
      { error: 'Failed to fetch cluster nodes' },
      { status: 500 }
    );
  }),

  http.get(`${API_BASE}/gateway/metrics`, () => {
    return HttpResponse.json(
      { error: 'Service unavailable' },
      { status: 503 }
    );
  }),
];
