import { ScreenStub } from '../../components/screen-stub';

export default function LoaderTripsPage() {
  return (
    <ScreenStub
      screenId="L-01"
      title="Today's trips"
      purpose="Trips grouped by loading bay, so Nadeesha can see what is waiting and in what order."
      endpoints={['GET /routes/{id}/legs']}
      designNotes={[
        'The load list is shown in REVERSE stop order — last stop loads first, nearest the door.',
        'A live plan change must be visible (L-02a), never a silent swap under her hands.',
      ]}
    />
  );
}
