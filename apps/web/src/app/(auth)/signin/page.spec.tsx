import { render, screen } from '@testing-library/react';
import SignInPage from './page';
import { ROLE_ROUTES, routeForRole } from '../../../lib/roles';

/**
 * G-01 is the judge's entry point, and the one screen whose failure makes every
 * other screen unreachable. These tests guard the two things that would break
 * the walkthrough: a missing role, or a role card that links nowhere useful.
 */
describe('Sign in (G-01)', () => {
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

  it('links each role card to that role workspace', () => {
    render(<SignInPage />);

    for (const route of ROLE_ROUTES) {
      const link = screen.getByRole('link', {
        name: new RegExp(route.label, 'i'),
      });
      expect(link.getAttribute('href')).toBe(route.href);
    }
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
