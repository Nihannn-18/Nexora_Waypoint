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

const finish = () =>
  fireEvent.click(screen.getByRole('button', { name: /finish delivery/i }));

/** jsdom has no real canvas: let the pad draw but make toBlob yield nothing. */
const saved = new Map<string, PropertyDescriptor | undefined>();
const proto = HTMLCanvasElement.prototype as unknown as Record<string, unknown>;

function stubCanvas(toBlob: (cb: (b: Blob | null) => void) => void) {
  for (const name of ['getContext', 'setPointerCapture', 'toBlob']) {
    if (!saved.has(name)) {
      saved.set(name, Object.getOwnPropertyDescriptor(proto, name));
    }
  }
  proto.getContext = () => ({
    beginPath: jest.fn(),
    moveTo: jest.fn(),
    lineTo: jest.fn(),
    stroke: jest.fn(),
    clearRect: jest.fn(),
  });
  proto.setPointerCapture = () => undefined;
  proto.toBlob = toBlob;
}

afterEach(() => {
  for (const [name, descriptor] of saved) {
    if (descriptor) Object.defineProperty(proto, name, descriptor);
    else delete proto[name];
  }
  saved.clear();
});

beforeEach(() => jest.clearAllMocks());

describe('Proof of delivery', () => {
  it('requires the receiver name, as the API does', async () => {
    render(<PodPage />);
    finish();
    await waitFor(() =>
      expect(screen.getByRole('alert').textContent).toMatch(/name/i),
    );
    expect(enqueue).not.toHaveBeenCalled();
  });

  it('requires a signature or photo', async () => {
    render(<PodPage />);
    fireEvent.change(screen.getByPlaceholderText(/receiver/i), {
      target: { value: 'R. Silva' },
    });
    finish();
    await waitFor(() =>
      expect(screen.getByRole('alert').textContent).toMatch(
        /signature or a photo/i,
      ),
    );
    expect(enqueue).not.toHaveBeenCalled();
  });

  it('saves DELIVERED with the receiver and photo on the phone', async () => {
    render(<PodPage />);
    fireEvent.change(screen.getByPlaceholderText(/receiver/i), {
      target: { value: ' R. Silva ' },
    });
    const photo = new File(['x'], 'dock.jpg', { type: 'image/jpeg' });
    fireEvent.change(screen.getByLabelText(/dock photo/i), {
      target: { files: [photo] },
    });
    finish();

    await waitFor(() =>
      expect(push).toHaveBeenCalledWith('/driver/saved/evt-1'),
    );
    expect(enqueue).toHaveBeenCalledWith(
      expect.objectContaining({
        outcome: 'DELIVERED',
        receiverName: 'R. Silva',
        photo,
      }),
    );
  });

  it('does not enqueue DELIVERED when the signature cannot be captured', async () => {
    stubCanvas((cb) => cb(null));
    render(<PodPage />);
    fireEvent.change(screen.getByPlaceholderText(/receiver/i), {
      target: { value: 'R. Silva' },
    });

    const pad = screen.getByRole('img');
    fireEvent.pointerDown(pad, { pointerId: 1, clientX: 5, clientY: 5 });
    fireEvent.pointerMove(pad, { pointerId: 1, clientX: 10, clientY: 10 });
    finish();

    await waitFor(() =>
      expect(screen.getByRole('alert').textContent).toMatch(
        /couldn’t capture the signature/i,
      ),
    );
    expect(enqueue).not.toHaveBeenCalled();
  });
});
