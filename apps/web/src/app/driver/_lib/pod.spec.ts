import { podValidationError, signatureBlob } from './pod';

const blob = () => new Blob(['x'], { type: 'image/png' });

/** A minimal canvas whose toBlob behaves as told. */
const canvasWith = (toBlob: HTMLCanvasElement['toBlob']): HTMLCanvasElement =>
  ({ toBlob }) as unknown as HTMLCanvasElement;

describe('signatureBlob', () => {
  it('returns null when nothing was drawn', async () => {
    expect(await signatureBlob(canvasWith(jest.fn()), false)).toBeNull();
  });

  it('returns null when the canvas has no toBlob', async () => {
    expect(
      await signatureBlob({} as unknown as HTMLCanvasElement, true),
    ).toBeNull();
  });

  it('returns null when the browser yields no blob', async () => {
    const c = canvasWith((cb) => cb(null));
    expect(await signatureBlob(c, true)).toBeNull();
  });

  it('returns null when drawing throws', async () => {
    const c = canvasWith(() => {
      throw new Error('no canvas');
    });
    expect(await signatureBlob(c, true)).toBeNull();
  });

  it('returns the blob when drawing succeeds', async () => {
    const c = canvasWith((cb) => cb(blob()));
    expect(await signatureBlob(c, true)).toBeInstanceOf(Blob);
  });
});

describe('podValidationError', () => {
  it('requires the receiver name first', () => {
    expect(
      podValidationError({ receiverName: '  ', signature: null, photo: null, signed: false }),
    ).toMatch(/name/i);
  });

  it('asks for a signature or photo when none is attached', () => {
    expect(
      podValidationError({ receiverName: 'R. Silva', signature: null, photo: null, signed: false }),
    ).toMatch(/signature or a photo/i);
  });

  it('flags a signature that failed to capture instead of silently proceeding', () => {
    expect(
      podValidationError({ receiverName: 'R. Silva', signature: null, photo: null, signed: true }),
    ).toMatch(/couldn’t capture the signature/i);
  });

  it('accepts a captured signature or a photo', () => {
    expect(
      podValidationError({ receiverName: 'R. Silva', signature: blob(), photo: null, signed: true }),
    ).toBeNull();
    expect(
      podValidationError({ receiverName: 'R. Silva', signature: null, photo: blob(), signed: false }),
    ).toBeNull();
  });
});
