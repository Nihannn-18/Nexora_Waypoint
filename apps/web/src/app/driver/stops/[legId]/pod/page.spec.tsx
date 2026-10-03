import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import PodPage from './page';
import { enqueue } from '../../../_lib/outbox';

const push = jest.fn();

jest.mock('next/navigation', () => ({
  useParams: () => ({ legId: 'leg-1' }),
  useRouter: () => ({ push }),
}));
jest.mock('../../../_lib/use-outbox', () => ({
  useLeg: () => ({ status: 'loading' }),
}));
jest.mock('../../../_lib/outbox', () => ({
  enqueue: jest.fn(async () => ({ clientEventId: 'evt-1' })),
}));

const finish = () => fireEvent.click(screen.getByRole('button', { name: /finish delivery/i }));

describe('Proof of delivery', () => {
  it('requires the receiver name, as the API does', () => {
    render(<PodPage />);
    finish();
    expect(screen.getByRole('alert').textContent).toMatch(/name/i);
    expect(enqueue).not.toHaveBeenCalled();
  });

  it('requires a signature or photo', () => {
    render(<PodPage />);
    fireEvent.change(screen.getByPlaceholderText(/receiver/i), { target: { value: 'R. Silva' } });
    finish();
    expect(screen.getByRole('alert').textContent).toMatch(/signature or a photo/i);
    expect(enqueue).not.toHaveBeenCalled();
  });

  it('saves DELIVERED with the receiver and photo on the phone', async () => {
    render(<PodPage />);
    fireEvent.change(screen.getByPlaceholderText(/receiver/i), { target: { value: ' R. Silva ' } });
    const photo = new File(['x'], 'dock.jpg', { type: 'image/jpeg' });
    fireEvent.change(screen.getByLabelText(/dock photo/i), { target: { files: [photo] } });
    finish();

    await waitFor(() => expect(push).toHaveBeenCalledWith('/driver/saved/evt-1'));
    expect(enqueue).toHaveBeenCalledWith(
      expect.objectContaining({ outcome: 'DELIVERED', receiverName: 'R. Silva', photo }),
    );
  });
});
