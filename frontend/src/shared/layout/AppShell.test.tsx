import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '../i18n';
import { AppShell } from './AppShell';

const mocks = vi.hoisted(() => ({ logout: vi.fn(), changePassword: vi.fn() }));

vi.mock('../auth/AuthProvider', () => ({
  useAuth: () => ({
    session: { user: { username: 'operator', display_name: 'Test operator', email: 'operator@example.com' } },
    roles: ['superadmin'],
    permissions: [],
    logout: mocks.logout,
    changePassword: mocks.changePassword,
  }),
}));

vi.mock('../query/hooks', () => ({
  useReady: () => ({ data: { status: 'ready' }, isError: false, refetch: vi.fn() }),
  useVersion: () => ({ data: { version: '8.0.0-pre.2' } }),
}));

function renderShell(initialPath = '/clients') {
  render(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/clients" element={<div>Clients content</div>} />
          <Route path="/clients/groups" element={<div>Groups content</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

describe('AppShell navigation', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
    mocks.logout.mockClear();
    mocks.changePassword.mockReset();
    mocks.changePassword.mockResolvedValue(undefined);
  });

  it('uses the menu button to collapse and restore desktop navigation', async () => {
    renderShell();
    const shell = document.querySelector('.app-shell');
    const closeButton = screen.getByRole('button', { name: 'Close navigation' });

    expect(closeButton).toHaveClass('app-sidebar-toggle');
    expect(closeButton).toHaveAttribute('aria-expanded', 'true');
    expect(closeButton.querySelector('.lucide-chevron-left')).not.toBeNull();
    await userEvent.click(closeButton);

    expect(shell).toHaveClass('sidebar-collapsed');
    const openButton = screen.getByRole('button', { name: 'Open navigation' });
    expect(openButton).toHaveAttribute('aria-expanded', 'false');
    expect(openButton.querySelector('.lucide-chevron-right')).not.toBeNull();

    await userEvent.click(openButton);
    expect(shell).not.toHaveClass('sidebar-collapsed');
  });

  it('separates clickable navigation groups from their links', async () => {
    renderShell();
    const section = screen.getByRole('button', { name: 'Client access' });

    expect(section).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('link', { name: 'Clients' })).toBeVisible();

    await userEvent.click(section);
    expect(section).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('link', { name: 'Clients' })).not.toBeInTheDocument();
  });

  it('shows a human control-plane status instead of a bare ready value', () => {
    renderShell();
    expect(screen.getByText('Control plane online')).toBeInTheDocument();
    expect(screen.queryByText(/^ready$/i)).not.toBeInTheDocument();
  });

  it('marks only the exact clients route active when a child page is open', () => {
    renderShell('/clients/groups');

    expect(screen.getByRole('link', { name: 'Groups' })).toHaveClass('active');
    expect(screen.getByRole('link', { name: 'Clients' })).not.toHaveClass('active');
  });

  it('opens account controls and submits a password change', async () => {
    renderShell();

    await userEvent.click(screen.getByRole('button', { name: /Test operator/i }));
    expect(screen.getByRole('button', { name: 'RU' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Logout' })).toBeVisible();

    await userEvent.click(screen.getByRole('button', { name: 'Account settings' }));
    expect(screen.getByRole('dialog', { name: 'Account settings' })).toBeVisible();

    await userEvent.type(screen.getByLabelText('Current password'), 'current-password');
    await userEvent.type(screen.getByLabelText('New password'), 'new-password-123');
    await userEvent.type(screen.getByLabelText('Confirm new password'), 'new-password-123');
    await userEvent.click(screen.getByRole('button', { name: 'Save new password' }));

    expect(mocks.changePassword).toHaveBeenCalledWith('current-password', 'new-password-123');
  });
});
