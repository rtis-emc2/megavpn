import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import {
  ApplyFooter,
  LoadingSkeletonList,
  Pagination,
  SearchInput,
  Stepper,
  Toast,
} from './Composite';

describe('shared composite UI', () => {
  it('renders a bounded accessible skeleton list', () => {
    const { container } = render(<LoadingSkeletonList rows={20} label="Loading clients" />);
    expect(screen.getByRole('status', { name: 'Loading clients' })).toBeInTheDocument();
    expect(container.querySelectorAll('.skeleton-row')).toHaveLength(12);
  });

  it('keeps pagination within the valid page range', () => {
    const onPageChange = vi.fn();
    render(<Pagination page={2} pageCount={3} onPageChange={onPageChange} />);
    expect(screen.getByRole('navigation', { name: 'Page 2 of 3' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Previous' }));
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    expect(onPageChange).toHaveBeenNthCalledWith(1, 1);
    expect(onPageChange).toHaveBeenNthCalledWith(2, 3);
  });

  it('exposes search and clear controls without synthetic input events', () => {
    const onClear = vi.fn();
    render(<SearchInput label="Client search" value="alice" onChange={() => undefined} onClear={onClear} />);
    expect(screen.getByRole('searchbox', { name: 'Client search' })).toHaveValue('alice');
    fireEvent.click(screen.getByRole('button', { name: 'Clear search' }));
    expect(onClear).toHaveBeenCalledOnce();
  });

  it('announces dangerous toasts and the active wizard step', () => {
    render(
      <>
        <Toast title="Apply failed" tone="danger">Node rejected the policy.</Toast>
        <Stepper steps={[
          { id: 'profile', label: 'Profile', status: 'complete' },
          { id: 'deploy', label: 'Deploy', status: 'current' },
        ]} />
      </>,
    );
    expect(screen.getByRole('alert')).toHaveTextContent('Node rejected the policy.');
    expect(screen.getByText('Deploy').closest('li')).toHaveAttribute('aria-current', 'step');
  });

  it('blocks duplicate apply while a mutation is pending', () => {
    const onApply = vi.fn();
    render(
      <ApplyFooter
        primaryLabel="Apply"
        pendingLabel="Applying"
        pending
        onApply={onApply}
      />,
    );
    const button = screen.getByRole('button', { name: 'Applying' });
    expect(button).toBeDisabled();
    fireEvent.click(button);
    expect(onApply).not.toHaveBeenCalled();
  });
});
