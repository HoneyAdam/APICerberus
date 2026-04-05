import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach, vi } from 'vitest';

// Cleanup after each test
afterEach(() => {
  cleanup();
});

// Mock matchMedia
defineMatchMediaMock();

// Mock window.ResizeObserver
class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}

global.ResizeObserver = ResizeObserverMock;

// Mock IntersectionObserver
class IntersectionObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}

global.IntersectionObserver = IntersectionObserverMock;

// Mock scrollTo
window.scrollTo = vi.fn();

// Mock matchMedia function
function defineMatchMediaMock() {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
}

// Suppress console errors during tests
const originalError = console.error;
console.error = (...args: unknown[]) => {
  // Filter out React act() warnings
  if (
    typeof args[0] === 'string' &&
    (/Warning.*not wrapped in act/.test(args[0]) ||
      /Error: Not implemented: navigation/.test(args[0]))
  ) {
    return;
  }
  originalError.call(console, ...args);
};
