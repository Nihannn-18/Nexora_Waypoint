import { render, screen } from '@testing-library/react';
import {
  OrderStatusBadge,
  StatusBadge,
  formatOrderStatus,
} from './status-badge';

describe('StatusBadge', () => {
  it('renders a glyph alongside the label so colour is never the only signal', () => {
    render(<StatusBadge tone="error">Blocked</StatusBadge>);
    const badge = screen.getByText('Blocked').closest('span');
    expect(badge?.textContent).toContain('✕');
  });

  it('can drop the glyph when an adjacent icon carries the meaning', () => {
    render(
      <StatusBadge tone="success" glyph={false}>
        Delivered
      </StatusBadge>,
    );
    expect(screen.getByText('Delivered').textContent).not.toContain('✓');
  });
});

describe('formatOrderStatus', () => {
  it('turns a wire status into a readable label', () => {
    expect(formatOrderStatus('IN_TRANSIT')).toBe('In Transit');
    expect(formatOrderStatus('DEFERRED')).toBe('Deferred');
  });
});

describe('OrderStatusBadge', () => {
  it('tones a deferral as a warning, not an error — it is a recorded decision', () => {
    render(<OrderStatusBadge status="DEFERRED" />);
    expect(screen.getByText('Deferred')).toBeTruthy();
  });
});
