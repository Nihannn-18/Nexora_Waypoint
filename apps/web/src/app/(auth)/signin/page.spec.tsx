import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import SignInPage from './page';
import { ROLE_ROUTES, routeForRole } from '../../../lib/roles';

const mockPush = jest.fn();
const mockLogin = jest.fn();
const mockMe = jest.fn();
const mockSet = jest.fn();
const mockClear = jest.fn();

jest.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}));

jest.mock('../../../lib/api', () => ({
  api: {
    login: (...args: unknown[]) => mockLogin(...args),
    me: () => mockMe(),
  },
  tokenStore: {
    set: (...args: unknown[]) => mockSet(...args),
    clear: (...args: unknown[]) => mockClear(...args),
  },
}));

/**
 * G-01 is the judge's entry point. These tests guard the two things that would
 * break the walkthrough: a missing role, and a role card that does not reach
 * that role's workspace after a real Go-API sign-in.
 */
describe('Sign in (G-01)', () => {
  beforeEach(() => {
    mockPush.mockReset();
    mockLogin.mockReset();
    mockMe.mockReset();
    mockSet.mockReset();
    mockClear.mockReset();
  });

  it('offers all four roles', () => {
    render(<SignInPage />);

    expect(screen.getByText('Dispatcher')).toBeTruthy();
    expect(screen.getByText('Loader')).toBeTruthy();
    expect(screen.getByText('Driver')).toBeTruthy();
    expect(screen.getByText('Store manager')).toBeTruthy();
  });

  it('shows each seeded account so a judge never has to hunt for credentials', () => {
    render(<SignInPage />);

    for (const route of ROLE_ROUTES) {
      expect(screen.getByText(route.email)).toBeTruthy();
    }
  });

  it('selecting a role never populates the email field', () => {
    render(<SignInPage />);

    for (const route of ROLE_ROUTES) {
      fireEvent.click(screen.getByRole('button', { name: new RegExp(route.label, 'i') }));
      expect((screen.getByLabelText('Email') as HTMLInputElement).value).toBe(
        '',
      );
    }
  });

  it('selecting a role never populates the password field', () => {
    render(<SignInPage />);

    for (const route of ROLE_ROUTES) {
      fireEvent.click(screen.getByRole('button', { name: new RegExp(route.label, 'i') }));
      expect(
        (screen.getByLabelText('Password') as HTMLInputElement).value,
      ).toBe('');
    }
  });

  it('lets the user type their own email and password', () => {
    render(<SignInPage />);

    fireEvent.change(screen.getByLabelText('Email'), {
      target: { value: 'typed@example.lk' },
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'typed-secret' },
    });

    expect((screen.getByLabelText('Email') as HTMLInputElement).value).toBe(
      'typed@example.lk',
    );
    expect(
      (screen.getByLabelText('Password') as HTMLInputElement).value,
    ).toBe('typed-secret');
  });

  it('requires an explicit submit and never logs in on role selection', () => {
    render(<SignInPage />);

    fireEvent.click(screen.getByRole('button', { name: /Dispatcher/i }));

    expect(mockLogin).not.toHaveBeenCalled();
    expect(mockPush).not.toHaveBeenCalled();
  });

  it('signs in through the Go API and routes to the server-reported role', async () => {
    mockLogin.mockResolvedValue({ token: 'opaque-token', user: {} });
    mockMe.mockResolvedValue({
      userId: 'seed-driver',
      name: 'Kasun P.',
      email: 'kasun.p@waypoint.lk',
      role: 'DRIVER',
      depotId: 'd-peli',
      outletId: null,
    });

    render(<SignInPage />);
    fireEvent.click(screen.getByRole('button', { name: /Driver/i }));
    fireEvent.change(screen.getByLabelText('Email'), {
      target: { value: 'kasun.p@waypoint.lk' },
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'waypoint2026' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^Sign in$/i }));

    await waitFor(() => expect(mockPush).toHaveBeenCalledWith('/driver'));
    expect(mockLogin).toHaveBeenCalledWith({
      email: 'kasun.p@waypoint.lk',
      password: 'waypoint2026',
    });
    expect(mockSet).toHaveBeenCalledWith('opaque-token');
  });

  it('shows an accessible error and clears the token when sign-in fails', async () => {
    mockLogin.mockRejectedValue(new Error('401'));

    render(<SignInPage />);
    fireEvent.change(screen.getByLabelText('Email'), {
      target: { value: 'priyantha.w@waypoint.lk' },
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'wrong' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^Sign in$/i }));

    expect(await screen.findByRole('alert')).toBeTruthy();
    expect(mockPush).not.toHaveBeenCalled();
    expect(mockClear).toHaveBeenCalled();
  });
});

describe('role routing table', () => {
  it('covers every role exactly once', () => {
    const roles = ROLE_ROUTES.map((r) => r.role);
    expect(new Set(roles).size).toBe(roles.length);
    expect(roles).toHaveLength(4);
  });

  it('resolves a workspace for every role', () => {
    for (const route of ROLE_ROUTES) {
      expect(routeForRole(route.role).href).toBe(route.href);
    }
  });
});
