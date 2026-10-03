import { fireEvent, render, screen } from '@testing-library/react';
import { ConfirmationDialog } from './confirmation-dialog';

function setup(busy: boolean) {
  const onConfirm = jest.fn();
  const onCancel = jest.fn();
  render(
    <ConfirmationDialog
      open
      title="Confirm the plan?"
      confirmLabel="Confirm 3 routes"
      busyLabel="Confirming…"
      busy={busy}
      onConfirm={onConfirm}
      onCancel={onCancel}
    >
      Summary
    </ConfirmationDialog>,
  );
  return { onConfirm, onCancel };
}

describe('ConfirmationDialog', () => {
  it('focuses Cancel first so confirming takes a deliberate second action', () => {
    setup(false);
    expect(document.activeElement).toBe(
      screen.getByRole('button', { name: 'Cancel' }),
    );
  });

  it('confirms or cancels on request, and Escape cancels', () => {
    const { onConfirm, onCancel } = setup(false);
    fireEvent.click(screen.getByRole('button', { name: 'Confirm 3 routes' }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('blocks both buttons and Escape while the request is in flight', () => {
    const { onCancel } = setup(true);
    expect(screen.getByRole('button', { name: 'Confirming…' })).toHaveProperty(
      'disabled',
      true,
    );
    expect(screen.getByRole('button', { name: 'Cancel' })).toHaveProperty(
      'disabled',
      true,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onCancel).not.toHaveBeenCalled();
  });
});
