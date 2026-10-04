import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { api } from '../../../lib/api';
import { CloseQueueButton } from './close-queue';

jest.mock('../../../lib/api', () => ({
  api: { closeQueue: jest.fn() },
}));

const mocked = api as jest.Mocked<typeof api>;

const noop = () => undefined;

function open() {
  render(
    <CloseQueueButton
      date="2026-09-26"
      depotId="d1"
      counts={{ FRESH: 12, STYLE: 4, TECH: 2 }}
      closedBrands={[]}
      onClosed={noop}
    />,
  );
  fireEvent.click(screen.getByRole('button', { name: /close queue/i }));
}

describe('CloseQueueButton', () => {
  beforeEach(() => jest.clearAllMocks());

  it('requires at least one brand before closing', () => {
    open();
    // All brands start selected (none closed), so the summary counts them.
    expect(screen.getByText(/will be frozen/i)).toBeTruthy();
  });

  it('sends the selected brands and reports the result', async () => {
    mocked.closeQueue.mockResolvedValue({
      date: '2026-09-26',
      depotId: 'd1',
      brands: ['FRESH'],
      closed: 1,
      alreadyClosed: [],
      queue: { orders: [], total: 0, limit: 200, offset: 0 },
    });
    open();
    // Deselect STYLE and TECH (rows 2 and 3) so only FRESH is sent.
    const boxes = screen.getAllByRole('checkbox');
    fireEvent.click(boxes[1]);
    fireEvent.click(boxes[2]);
    fireEvent.click(screen.getByRole('button', { name: /close selected/i }));

    await waitFor(() =>
      expect(mocked.closeQueue).toHaveBeenCalledWith({
        date: '2026-09-26',
        depotId: 'd1',
        brands: ['FRESH'],
      }),
    );
  });

  it('reports an already-closed brand without re-closing it', async () => {
    mocked.closeQueue.mockResolvedValue({
      date: '2026-09-26',
      depotId: 'd1',
      brands: ['FRESH'],
      closed: 0,
      alreadyClosed: ['FRESH'],
      queue: { orders: [], total: 0, limit: 200, offset: 0 },
    });
    render(
      <CloseQueueButton
        date="2026-09-26"
        depotId="d1"
        counts={{ FRESH: 0, STYLE: 4, TECH: 2 }}
        closedBrands={['FRESH']}
        onClosed={noop}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /close queue/i }));
    // The closed brand's checkbox is disabled (the first of the three rows).
    const boxes = screen.getAllByRole('checkbox');
    expect((boxes[0] as HTMLInputElement).disabled).toBe(true);
    fireEvent.click(screen.getByRole('button', { name: /close selected/i }));
    await waitFor(() =>
      expect(mocked.closeQueue).toHaveBeenCalledWith({
        date: '2026-09-26',
        depotId: 'd1',
        brands: ['STYLE', 'TECH'],
      }),
    );
  });
});
