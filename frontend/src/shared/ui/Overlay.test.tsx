import { useState } from 'react';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeAll, describe, expect, it } from 'vitest';
import i18n from '../i18n';
import { Button } from './Button';
import { Drawer, Modal } from './Overlay';

function Harness() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Open dialog</Button>
      <Modal title="Accessible dialog" open={open} onClose={() => setOpen(false)}>
        <Button>First action</Button>
        <Button onClick={() => setOpen(false)}>Close dialog</Button>
      </Modal>
    </>
  );
}

describe('Modal accessibility lifecycle', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('en');
  });

  it('traps focus, locks page scrolling and restores focus after close', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    const trigger = screen.getByRole('button', { name: 'Open dialog' });
    await user.click(trigger);

    const dialog = screen.getByRole('dialog', { name: 'Accessible dialog' });
    const closeIcon = within(dialog).getByRole('button', { name: 'Close' });
    const lastAction = within(dialog).getByRole('button', { name: 'Close dialog' });
    expect(closeIcon).toHaveFocus();
    expect(document.body).toHaveClass('overlay-open');

    await user.tab({ shift: true });
    expect(lastAction).toHaveFocus();
    await user.tab();
    expect(closeIcon).toHaveFocus();

    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
    expect(document.body).not.toHaveClass('overlay-open');
  });

  it('applies the wide workspace variant to drawers', () => {
    render(
      <Drawer title="Node workspace" open onClose={() => undefined}>
        <div>Node details</div>
      </Drawer>,
    );

    expect(screen.getByRole('dialog', { name: 'Node workspace' })).toHaveClass('drawer-wide');
  });
});
