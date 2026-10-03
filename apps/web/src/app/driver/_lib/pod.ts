/**
 * Proof-of-delivery capture helpers.
 *
 * The canvas can fail to yield a blob (unsupported browser, out-of-memory,
 * private mode). The form must treat that as "no signature", never enqueue a
 * DELIVERED event without a real artefact — the server would reject it and the
 * driver would only find out after finishing the stop.
 */
export interface PodInput {
  readonly receiverName: string;
  readonly signature: Blob | null;
  readonly photo: Blob | null;
  /** Whether the pad shows a drawn signature (the blob may still be null). */
  readonly signed: boolean;
}

/** Captures the drawn signature; resolves null when the canvas yields nothing. */
export function signatureBlob(
  canvas: HTMLCanvasElement | null,
  signed: boolean,
): Promise<Blob | null> {
  if (!signed || !canvas || typeof canvas.toBlob !== 'function') {
    return Promise.resolve(null);
  }
  return new Promise((resolve) => {
    try {
      canvas.toBlob((blob) => resolve(blob), 'image/png');
    } catch {
      resolve(null);
    }
  });
}

/** Inline validation for the POD form. A message when invalid, else null. */
export function podValidationError(input: PodInput): string | null {
  if (!input.receiverName.trim()) {
    return 'Enter the name of the person receiving the goods.';
  }
  if (input.signature || input.photo) return null;
  return input.signed
    ? 'Couldn’t capture the signature. Clear the pad and sign again, or add a photo.'
    : 'Add a signature or a photo as proof of delivery.';
}
