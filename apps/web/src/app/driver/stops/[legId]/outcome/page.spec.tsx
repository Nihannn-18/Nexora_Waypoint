import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import OutcomePage from './page';
import { enqueue } from '../../../_lib/outbox';

const push = jest.fn();
let outcomeParam = 'FAILED';

jest.mock('next/navigation', () => ({
  useParams: () => ({ legId: 'leg-1' }),
  useRouter: () => ({ push }),
  useSearchParams: () => new URLSearchParams(`o=${outcomeParam}`),
}));
jest.mock('../../../_lib/use-outbox', () => ({
  useLeg: () => ({ status: 'loading' }),
}));
jest.mock('../../../_lib/outbox', () => ({
  enqueue: jest.fn(async () => ({ clientEventId: 'evt-1' })),
}));

beforeEach(() => jest.clearAllMocks());

describe('Record outcome (R-02)', () => {
  it('blocks a failed delivery without a reason (Figma 35:5561)', () => {
    outcomeParam = 'FAILED';
    render(<OutcomePage />);
    fireEvent.click(screen.getByRole('button', { name: /save outcome/i }));

    expect(screen.getByRole('alert').textContent).toMatch(/choose a reason/i);
    expect(enqueue).not.toHaveBeenCalled();
  });

  it('saves a failed delivery with its reason on the phone first', async () => {
    outcomeParam = 'FAILED';
    render(<OutcomePage />);
    fireEvent.click(screen.getByRole('button', { name: 'Access blocked' }));
    fireEvent.click(screen.getByRole('button', { name: /save outcome/i }));

    await waitFor(() => expect(push).toHaveBeenCalledWith('/driver/saved/evt-1'));
    expect(enqueue).toHaveBeenCalledWith(
      expect.objectContaining({ legId: 'leg-1', outcome: 'FAILED', reasonCode: 'ACCESS_BLOCKED' }),
    );
  });

  it('saves a delay without asking for a reason', async () => {
    outcomeParam = 'DELAYED';
    render(<OutcomePage />);
    fireEvent.click(screen.getByRole('button', { name: /save outcome/i }));

    await waitFor(() => expect(enqueue).toHaveBeenCalled());
    expect(enqueue).toHaveBeenCalledWith(
      expect.objectContaining({ outcome: 'DELAYED', reasonCode: undefined }),
    );
  });

  it('sends a delivery on to proof of delivery instead of saving', () => {
    outcomeParam = 'DELIVERED';
    render(<OutcomePage />);
    fireEvent.click(screen.getByRole('button', { name: /proof of delivery/i }));

    expect(push).toHaveBeenCalledWith('/driver/stops/leg-1/pod');
    expect(enqueue).not.toHaveBeenCalled();
  });
});
