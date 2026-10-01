import { ScreenStub } from '../../components/screen-stub';

export default function DriverCockpitPage() {
  return (
    <ScreenStub
      screenId="R-01"
      title="Cockpit"
      purpose="The current stop, its delivery window, and the one action Kasun needs next."
      endpoints={[
        'GET /routes/{id}/legs',
        'POST /legs/{id}/events',
        'POST /sync/events',
      ]}
      designNotes={[
        'Every outcome is captured with a clientEventId generated BEFORE the request, so a retry can never double-record a delivery.',
        'Events carry device time and keep it after syncing — a later plan change must not overwrite what the driver saw.',
        'The whole surface works from cache with no connectivity (DG-B1).',
      ]}
    />
  );
}
