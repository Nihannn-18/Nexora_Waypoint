'use client';

import { useRouter } from 'next/navigation';
import { api } from '../../../../lib/api';
import { useApiQuery } from '../../../../lib/use-api-query';
import { LoadingState, ErrorState } from '../../../../components/states';
import { PageHeader } from '../../_components/ui';
import { UserForm } from '../_components/user-form';

/**
 * Dispatcher · New user. Creates an operational account (driver, loader or
 * store manager). The role is chosen here; the server validates it and the
 * role-specific assignment.
 */
export function NewUserScreen() {
  const router = useRouter();
  const depots = useApiQuery('depots', () => api.getDepots());
  const outlets = useApiQuery('all-outlets', () => api.getOutlets());

  const ready =
    depots.data !== undefined && outlets.data !== undefined;
  const failed = depots.error ?? outlets.error;

  return (
    <>
      <PageHeader
        title="Create user"
        description="Create an operational account. The role decides which depot or outlet it is scoped to."
        actions={
          <button
            type="button"
            onClick={() => router.push('/dispatcher/users')}
            className="tap-target rounded-control bg-card px-4 text-sm font-medium text-ink ring-1 ring-ink/15 hover:bg-page"
          >
            Back to users
          </button>
        }
      />
      {failed && !ready ? (
        <ErrorState
          error={failed}
          onRetry={() => {
            depots.reload();
            outlets.reload();
          }}
        />
      ) : ready ? (
        <UserForm depots={depots.data ?? []} outlets={outlets.data ?? []} />
      ) : (
        <LoadingState label="Loading depots and outlets…" />
      )}
    </>
  );
}
