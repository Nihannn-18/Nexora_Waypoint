import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { AppNotification } from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { DispatcherScopeProvider } from '../_components/dispatcher-context';
import { NotificationsScreen } from './notifications-screen';

jest.mock('../../../lib/api', () => ({
  api: {
    meta: jest.fn(),
    getDepots: jest.fn(),
    getUnreadNotificationCount: jest.fn(),
    listNotifications: jest.fn(),
    markNotificationRead: jest.fn(),
    markAllNotificationsRead: jest.fn(),
  },
}));
jest.mock('next/navigation', () => ({
  usePathname: () => '/dispatcher/notifications',
}));

const mocked = api as jest.Mocked<typeof api>;

const n = (id: string, read: boolean): AppNotification => ({
  id,
  type: 'DELIVERY_FAILED',
  title: `Delivery failed ${id}`,
  message: 'A delivery at OUT027 failed and needs attention.',
  reference: `delivery:${id}`,
  outletId: 'OUT027',
  read,
  createdAt: '2026-09-26T05:12:00+05:30',
});

let store: AppNotification[];

beforeEach(() => {
  jest.clearAllMocks();
  store = [n('n1', false), n('n2', true)];
  mocked.meta.mockResolvedValue({
    now: '2026-09-26T05:30:00+05:30',
    demoMode: true,
    timezone: 'Asia/Colombo',
  });
  mocked.getDepots.mockResolvedValue([]);
  mocked.listNotifications.mockImplementation(async () => store);
  mocked.getUnreadNotificationCount.mockImplementation(
    async () => store.filter((x) => !x.read).length,
  );
  mocked.markNotificationRead.mockImplementation(async (id: string) => {
    store = store.map((x) => (x.id === id ? { ...x, read: true } : x));
  });
  mocked.markAllNotificationsRead.mockImplementation(async () => {
    const changed = store.filter((x) => !x.read).length;
    store = store.map((x) => ({ ...x, read: true }));
    return changed;
  });
});

function renderScreen() {
  return render(
    <DispatcherScopeProvider>
      <NotificationsScreen />
    </DispatcherScopeProvider>,
  );
}

describe('Notifications', () => {
  it('marks one notification read and refreshes the unread count', async () => {
    renderScreen();
    expect(await screen.findByText('1 unread notification')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Mark as read' }));
    await waitFor(() =>
      expect(mocked.markNotificationRead).toHaveBeenCalledWith('n1'),
    );
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Mark as read' })).toBeNull(),
    );
    expect(mocked.getUnreadNotificationCount.mock.calls.length).toBeGreaterThan(
      1,
    );
  });

  it('marks all read, then disables the action', async () => {
    renderScreen();
    const all = await screen.findByRole('button', { name: 'Mark all as read' });
    await waitFor(() => expect(all).toHaveProperty('disabled', false));
    fireEvent.click(all);
    await waitFor(() =>
      expect(mocked.markAllNotificationsRead).toHaveBeenCalled(),
    );
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Mark all as read' }),
      ).toHaveProperty('disabled', true),
    );
  });

  it('links a delivery problem to the outlet’s orders', async () => {
    renderScreen();
    const links = await screen.findAllByRole('link', {
      name: /Open OUT027’s orders/,
    });
    expect(links[0]?.getAttribute('href')).toBe(
      '/dispatcher/queue?q=OUT027&days=all',
    );
  });

  it('explains an empty inbox', async () => {
    store = [];
    renderScreen();
    expect(await screen.findByText('You’re all caught up')).toBeTruthy();
  });
});
