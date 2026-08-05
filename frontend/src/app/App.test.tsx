import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';

describe('App', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/v1/auth/me')) {
        return new Response(JSON.stringify({ status: 'error', error: 'unauthorized' }), {
          status: 401,
          headers: { 'content-type': 'application/json' },
        });
      }
      return new Response(JSON.stringify({ status: 'ready', version: '8.0.0' }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      });
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders unauthenticated login shell', async () => {
    render(<App />);
    await waitFor(() => expect(screen.getByRole('heading', { name: /вход оператора|operator login/i })).toBeInTheDocument());
  });
});
