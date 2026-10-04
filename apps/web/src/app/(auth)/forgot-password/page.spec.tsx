import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import ForgotPasswordPage from './page';
import { api } from '../../../lib/api';

jest.mock('../../../lib/api', () => ({
  api: { forgotPassword: jest.fn() },
}));

const mocked = api as jest.Mocked<typeof api>;

describe('Forgot password', () => {
  beforeEach(() => jest.clearAllMocks());

  it('shows the generic success response for any email', async () => {
    mocked.forgotPassword.mockResolvedValue({
      message:
        'If the account exists, password reset instructions have been provided.',
    });
    render(<ForgotPasswordPage />);

    fireEvent.change(screen.getByLabelText(/Email/i), {
      target: { value: 'unknown@waypoint.lk' },
    });
    fireEvent.click(
      screen.getByRole('button', { name: /Send reset instructions/i }),
    );

    expect(
      await screen.findByText(
        /If the account exists, password reset instructions have been provided/i,
      ),
    ).toBeTruthy();
    expect(mocked.forgotPassword).toHaveBeenCalledWith({
      email: 'unknown@waypoint.lk',
    });
  });

  it('never states whether the email exists', async () => {
    mocked.forgotPassword.mockResolvedValue({
      message:
        'If the account exists, password reset instructions have been provided.',
    });
    render(<ForgotPasswordPage />);
    fireEvent.change(screen.getByLabelText(/Email/i), {
      target: { value: 'someone@waypoint.lk' },
    });
    fireEvent.click(
      screen.getByRole('button', { name: /Send reset instructions/i }),
    );

    await screen.findByText(/If the account exists/i);
    expect(screen.queryByText(/does not exist/i)).toBeNull();
    expect(screen.queryByText(/no account/i)).toBeNull();
  });

  it('surfaces only a connectivity fault', async () => {
    const { WaypointApiError } = jest.requireActual('@waypoint/api-client');
    mocked.forgotPassword.mockRejectedValue(
      new WaypointApiError('offline', { status: 0, isOffline: true }),
    );
    render(<ForgotPasswordPage />);
    fireEvent.change(screen.getByLabelText(/Email/i), {
      target: { value: 'someone@waypoint.lk' },
    });
    fireEvent.click(
      screen.getByRole('button', { name: /Send reset instructions/i }),
    );

    expect(await screen.findByRole('alert')).toBeTruthy();
  });
});
