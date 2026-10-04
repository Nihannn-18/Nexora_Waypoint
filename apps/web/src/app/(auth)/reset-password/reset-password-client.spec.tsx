import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import ResetPasswordClient from './reset-password-client';
import { api } from '../../../lib/api';

jest.mock('../../../lib/api', () => ({
  api: { resetPassword: jest.fn() },
}));

const push = jest.fn();
let searchParams = new URLSearchParams();
jest.mock('next/navigation', () => ({
  useRouter: () => ({ push }),
  useSearchParams: () => searchParams,
}));

const mocked = api as jest.Mocked<typeof api>;

describe('Reset password', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    searchParams = new URLSearchParams();
  });

  it('prefills the token from the reset link', () => {
    searchParams = new URLSearchParams('token=abc123');
    render(<ResetPasswordClient />);
    expect((screen.getByLabelText(/Reset token/i) as HTMLInputElement).value).toBe(
      'abc123',
    );
  });

  it('rejects mismatched passwords before calling the API', async () => {
    render(<ResetPasswordClient />);
    fireEvent.change(screen.getByLabelText(/Reset token/i), {
      target: { value: 'tok' },
    });
    fireEvent.change(screen.getByLabelText(/New password/i), {
      target: { value: 'new-secret' },
    });
    fireEvent.change(screen.getByLabelText(/Confirm password/i), {
      target: { value: 'different' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Reset password/i }));

    expect(await screen.findByText(/do not match/i)).toBeTruthy();
    expect(mocked.resetPassword).not.toHaveBeenCalled();
  });

  it('resets the password and returns to sign in', async () => {
    mocked.resetPassword.mockResolvedValue({
      message: 'Your password has been reset.',
    });
    render(<ResetPasswordClient />);
    fireEvent.change(screen.getByLabelText(/Reset token/i), {
      target: { value: 'tok' },
    });
    fireEvent.change(screen.getByLabelText(/New password/i), {
      target: { value: 'new-secret' },
    });
    fireEvent.change(screen.getByLabelText(/Confirm password/i), {
      target: { value: 'new-secret' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Reset password/i }));

    await waitFor(() =>
      expect(mocked.resetPassword).toHaveBeenCalledWith({
        token: 'tok',
        newPassword: 'new-secret',
        confirmPassword: 'new-secret',
      }),
    );
    expect(await screen.findByText(/has been reset/i)).toBeTruthy();
  });

  it('shows a generic message for an invalid or expired token', async () => {
    const { WaypointApiError } = jest.requireActual('@waypoint/api-client');
    mocked.resetPassword.mockRejectedValue(
      new WaypointApiError('invalid', {
        status: 400,
        fieldErrors: [{ field: 'token', message: 'is invalid or has expired' }],
      }),
    );
    render(<ResetPasswordClient />);
    fireEvent.change(screen.getByLabelText(/Reset token/i), {
      target: { value: 'bad' },
    });
    fireEvent.change(screen.getByLabelText(/New password/i), {
      target: { value: 'new-secret' },
    });
    fireEvent.change(screen.getByLabelText(/Confirm password/i), {
      target: { value: 'new-secret' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Reset password/i }));

    expect(
      await screen.findByText(/invalid or has expired/i),
    ).toBeTruthy();
    expect(push).not.toHaveBeenCalled();
  });
});
