import { fireEvent, render, screen } from '@testing-library/react';
import { WaypointApiError } from '@waypoint/api-client';
import { ErrorState, describeApiError } from './states';

describe('describeApiError', () => {
  it('treats a request that never reached the server as retryable and harmless', () => {
    const d = describeApiError(
      new WaypointApiError('x', { status: 0, isOffline: true }),
    );
    expect(d.title).toMatch(/can’t reach/i);
    expect(d.detail).toMatch(/nothing was changed/i);
    expect(d.retryable).toBe(true);
  });

  it('explains a role refusal without offering a pointless retry', () => {
    const d = describeApiError(
      new WaypointApiError('Forbidden', { status: 403, code: 'FORBIDDEN' }),
    );
    expect(d.title).toMatch(/dispatcher account/i);
    expect(d.retryable).toBe(false);
  });

  it('lists every invalid field on a validation failure', () => {
    const d = describeApiError(
      new WaypointApiError('invalid', {
        status: 400,
        fieldErrors: [
          { field: 'brand', message: 'must be one of FRESH, STYLE, TECH' },
          { field: 'limit', message: 'must be between 1 and 200' },
        ],
      }),
    );
    expect(d.detail).toContain('brand: must be one of FRESH, STYLE, TECH');
    expect(d.detail).toContain('limit: must be between 1 and 200');
  });

  it('says plainly when the API cannot verify sign-in yet', () => {
    const d = describeApiError(
      new WaypointApiError('Authentication is not configured', { status: 500 }),
    );
    expect(d.title).toMatch(/sign-in isn’t connected/i);
  });
});

describe('ErrorState', () => {
  it('offers Try again only for retryable failures', () => {
    const retry = jest.fn();
    const { rerender } = render(
      <ErrorState
        error={new WaypointApiError('down', { status: 503 })}
        onRetry={retry}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(retry).toHaveBeenCalledTimes(1);

    rerender(
      <ErrorState
        error={new WaypointApiError('no', { status: 403 })}
        onRetry={retry}
      />,
    );
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull();
  });
});
