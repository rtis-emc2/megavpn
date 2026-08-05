import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import '../shared/i18n';
import { RouteErrorBoundary } from './RouteErrorBoundary';

let shouldThrow = true;

function FragileRoute() {
  if (shouldThrow) {
    throw new Error('sensitive backend detail');
  }
  return <div>Recovered route</div>;
}

describe('RouteErrorBoundary', () => {
  afterEach(() => {
    shouldThrow = true;
    vi.restoreAllMocks();
  });

  it('shows a safe recovery screen and retries the route', () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    render(
      <RouteErrorBoundary>
        <FragileRoute />
      </RouteErrorBoundary>,
    );

    expect(screen.getByRole('alert')).toHaveTextContent('Request failed');
    expect(screen.queryByText('sensitive backend detail')).not.toBeInTheDocument();

    shouldThrow = false;
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(screen.getByText('Recovered route')).toBeInTheDocument();
  });
});
