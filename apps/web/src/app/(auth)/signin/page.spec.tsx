import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import SignInPage from './page';
import { ROLE_ROUTES, routeForRole } from '../../../lib/roles';

const mockPush = jest.fn();
const mockSignIn = jest.fn();
const mockMe = jest.fn();

jest.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}));

jest.mock('../../../lib/auth-client', () => ({
  authClient: {
    signIn: { email: (...args: unknown[]) => mockSignIn(...args) },
  },
}));

jest.mock('../../../lib/api', () => ({
  api: { me: () => mockMe() },
}));

/**
 * G-01 is the judge's entry point. These tests guard the two things that would
 * break the walkthrough: a missing role, and a role card that does not reach
 * that role's workspace after a real sign-in.
 */
describe('Sign in (G-01)', () => {
  beforeEach(() => {
    mockPush.mockReset();
    mockSignIn.mockReset();
    mockMe.mockReset();
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

  it('pre-fills the demo account when a role card is chosen', () => {
    render(<SignInPage />);

    fireEvent.click(screen.getByRole('button', { name: /Dispatcher/i }));

    expect((screen.getByLabelText('Email') as HTMLInputElement).value).toBe(
      'priyantha.w@waypoint.lk',
    );
    expect((screen.getByLabelText('Password') as HTMLInputElement).value).toBe(
      'waypoint2026',
    );
  });

  it('signs in and routes to the role the server reports', async () => {
    mockSignIn.mockResolvedValue({ data: {}, error: null });
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
    fireEvent.click(screen.getByRole('button', { name: /^Sign in$/i }));

    await waitFor(() => expect(mockPush).toHaveBeenCalledWith('/driver'));
    expect(mockSignIn).toHaveBeenCalledWith({
      email: 'kasun.p@waypoint.lk',
      password: 'waypoint2026',
    });
  });

  it('shows an accessible error when credentials are rejected', async () => {
    mockSignIn.mockResolvedValue({ data: null, error: { message: 'invalid' } });

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
