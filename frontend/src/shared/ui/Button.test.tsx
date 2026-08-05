import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { RefreshButton } from './Button';

describe('RefreshButton', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows a busy state and prevents duplicate refreshes', async () => {
    vi.useFakeTimers();
    let resolveRefresh: (() => void) | undefined;
    const onRefresh = vi.fn(() => new Promise<void>((resolve) => {
      resolveRefresh = resolve;
    }));

    render(<RefreshButton onRefresh={onRefresh}>Refresh</RefreshButton>);
    const button = screen.getByRole('button', { name: 'Refresh' });

    await act(async () => {
      fireEvent.click(button);
      await Promise.resolve();
    });
    fireEvent.click(button);

    expect(onRefresh).toHaveBeenCalledTimes(1);
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('aria-busy', 'true');
    expect(button).toHaveClass('is-refreshing');

    await act(async () => {
      resolveRefresh?.();
      await Promise.resolve();
      await vi.runAllTimersAsync();
    });

    expect(button).not.toBeDisabled();
    expect(button).toHaveAttribute('aria-busy', 'false');
    expect(button).not.toHaveClass('is-refreshing');
  });
});
