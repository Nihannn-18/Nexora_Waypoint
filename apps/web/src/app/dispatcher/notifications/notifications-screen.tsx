'use client';

import Link from 'next/link';
import { useState } from 'react';
import { Mono, StatusBadge } from '@waypoint/ui';
import type { AppNotification } from '@waypoint/shared-types';
import { api } from '../../../lib/api';
import { formatDateTime, plural } from '../../../lib/format';
import { useApiQuery } from '../../../lib/use-api-query';
import {
  EmptyState,
  ErrorState,
  LoadingState,
} from '../../../components/states';
import { useDispatcherScope } from '../_components/dispatcher-context';
import { PageHeader, buttonClass } from '../_components/ui';
import { notificationLink, notificationType } from './notification-model';

const PAGE_SIZE = 50;

/**
 * In-app notifications: things that need a dispatcher's attention now — a
 * loading shortfall, a failed or delayed delivery. Unlike the audit trail they
 * have a read state. Marking read changes only that state; the event stays in
 * the audit trail.
 */
export function NotificationsScreen() {
  const { refreshUnread, unreadCount } = useDispatcherScope();
  const [page, setPage] = useState(0);
  const list = useApiQuery(`notifications:${page}`, () =>
    api.listNotifications({ limit: PAGE_SIZE, offset: page * PAGE_SIZE }),
  );
  const [busy, setBusy] = useState<string | null>(null);
  const [actionError, setActionError] = useState<unknown>(null);

  const act = async (key: string, fn: () => Promise<unknown>) => {
    setBusy(key);
    setActionError(null);
    try {
      await fn();
      list.reload();
      refreshUnread();
    } catch (error) {
      setActionError(error);
    } finally {
      setBusy(null);
    }
  };

  const rows = list.data ?? [];

  return (
    <>
      <PageHeader
        title="Notifications"
        description="Shortfalls and delivery problems raised for your depot, newest first."
        actions={
          <button
            type="button"
            className={buttonClass.secondary}
            disabled={!unreadCount || busy !== null}
            onClick={() => act('all', () => api.markAllNotificationsRead())}
          >
            {busy === 'all' ? 'Marking…' : 'Mark all as read'}
          </button>
        }
      />
      {actionError ? (
        <div className="mb-3">
          <ErrorState error={actionError} compact />
        </div>
      ) : null}

      {list.loading ? (
        <LoadingState label="Loading notifications…" />
      ) : list.error ? (
        <ErrorState error={list.error} onRetry={list.reload} />
      ) : rows.length === 0 ? (
        <EmptyState
          title={page > 0 ? 'No older notifications' : 'You’re all caught up'}
        >
          A notification appears when a loader records a shortfall or a driver
          reports a failed or delayed delivery.
        </EmptyState>
      ) : (
        <>
          {unreadCount ? (
            <p className="mb-2 text-sm text-ink-muted">
              {plural(unreadCount, 'unread notification')}
            </p>
          ) : null}
          <ul className="space-y-2">
            {rows.map((n) => (
              <NotificationItem
                key={n.id}
                notification={n}
                busy={busy === n.id}
                onMarkRead={() =>
                  act(n.id, () => api.markNotificationRead(n.id))
                }
              />
            ))}
          </ul>
        </>
      )}

      {(page > 0 || rows.length === PAGE_SIZE) && (
        <nav aria-label="Pages" className="mt-4 flex justify-between">
          <button
            type="button"
            className={buttonClass.secondary}
            disabled={page === 0}
            onClick={() => setPage((p) => p - 1)}
          >
            ← Newer
          </button>
          <button
            type="button"
            className={buttonClass.secondary}
            disabled={rows.length < PAGE_SIZE}
            onClick={() => setPage((p) => p + 1)}
          >
            Older →
          </button>
        </nav>
      )}
    </>
  );
}

export function NotificationItem({
  notification: n,
  busy,
  onMarkRead,
}: {
  notification: AppNotification;
  busy: boolean;
  onMarkRead: () => void;
}) {
  const type = notificationType(n.type);
  const link = notificationLink(n);
  return (
    <li
      className={`flex flex-wrap items-start gap-3 rounded-card p-3 ring-1 ${
        n.read ? 'bg-card ring-ink/10' : 'bg-brand/5 ring-brand/30'
      }`}
    >
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          {!n.read && (
            <span className="text-[0.6875rem] font-bold uppercase tracking-wide text-link">
              ● Unread
            </span>
          )}
          <StatusBadge tone={type.tone}>{type.label}</StatusBadge>
          <span className="text-xs text-ink-muted">
            <Mono>{formatDateTime(n.createdAt)}</Mono>
          </span>
        </div>
        <p className="mt-1 font-semibold text-ink">{n.title}</p>
        <p className="text-sm text-ink">{n.message}</p>
        {link && (
          <Link
            href={link.href}
            onClick={() => {
              if (!n.read) onMarkRead();
            }}
            className="mt-1 inline-block text-sm font-medium text-link hover:underline"
          >
            {link.label} →
          </Link>
        )}
      </div>
      {n.read ? (
        <span className="text-xs text-ink-muted">
          Read{n.readAt && <> {formatDateTime(n.readAt)}</>}
        </span>
      ) : (
        <button
          type="button"
          className={buttonClass.secondary}
          onClick={onMarkRead}
          disabled={busy}
        >
          {busy ? 'Marking…' : 'Mark as read'}
        </button>
      )}
    </li>
  );
}
