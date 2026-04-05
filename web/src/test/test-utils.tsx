import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, type RenderOptions } from '@testing-library/react';
import type { ReactElement, ReactNode } from 'react';
import { MemoryRouter } from 'react-router-dom';

// Test query client configuration
export function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
        staleTime: 0,
      },
    },
  });
}

interface ProvidersProps {
  children: ReactNode;
  initialRoute?: string;
}

// Wrap components with necessary providers
export function AllProviders({
  children,
  initialRoute = '/',
}: ProvidersProps) {
  const testQueryClient = createTestQueryClient();

  return (
    <MemoryRouter initialEntries={[initialRoute]}>
      <QueryClientProvider client={testQueryClient}>
        {children}
      </QueryClientProvider>
    </MemoryRouter>
  );
}

// Custom render function with providers
interface CustomRenderOptions extends Omit<RenderOptions, 'wrapper'> {
  initialRoute?: string;
}

export function customRender(
  ui: ReactElement,
  { initialRoute = '/', ...options }: CustomRenderOptions = {}
) {
  return render(ui, {
    wrapper: ({ children }) => (
      <AllProviders initialRoute={initialRoute}>{children}</AllProviders>
    ),
    ...options,
  });
}

// Re-export testing library utilities
export { screen, waitFor, within } from '@testing-library/react';
export { userEvent } from '@testing-library/user-event';

// Helper to wait for query promises to resolve
export async function waitForQueries(queryClient: QueryClient) {
  await new Promise((resolve) => setTimeout(resolve, 0));
}

// Mock API responses helper
export function createMockResponse<T>(data: T, delay = 0): Promise<T> {
  return new Promise((resolve) => {
    setTimeout(() => resolve(data), delay);
  });
}

// Mock error response
export function createMockError(message: string, status = 500): Error {
  const error = new Error(message);
  (error as Error & { status: number }).status = status;
  return error;
}
