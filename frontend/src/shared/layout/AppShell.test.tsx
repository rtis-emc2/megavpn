import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '../i18n';
import { AppShell } from './AppShell';

const mocks = vi.hoisted(() => ({ logout: vi.fn() }));

vi.mock('../auth/AuthProvider', () => ({
  useAuth: () => ({
    session: { user: { display_name: 'Test operator' } },
    roles: ['superadmin'],
    permissions: [],
    logout: mocks.logout,
  }),
}));

vi.mock('../query/hooks', () => ({
  useReady: () => ({ data: { status: 'ready' }, isError: false, refetch: vi.fn() }),
  useVersion: () => ({ data: { version: '8.0.0-pre.1' } }),
}));

function renderShell() {
  render(
    <MemoryRouter initialEntries={['/clients']}>
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/clients" element={<div>Clients content</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

describe('AppShell navigation', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
    mocks.logout.mockClear();
  });

  it('uses the menu button to collapse and restore desktop navigation', async () => {
    renderShell();
    const shell = document.querySelector('.app-shell');
    const closeButton = screen.getByRole('button', { name: 'Close navigation' });

    expect(closeButton).toHaveAttribute('aria-expanded', 'true');
    await userEvent.click(closeButton);

    expect(shell).toHaveClass('sidebar-collapsed');
    expect(screen.getByRole('button', { name: 'Open navigation' })).toHaveAttribute('aria-expanded', 'false');

    await userEvent.click(screen.getByRole('button', { name: 'Open navigation' }));
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
});
