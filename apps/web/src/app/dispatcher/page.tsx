import { ScreenStub } from '../../components/screen-stub';

export default function DispatchControlPage() {
  return (
    <ScreenStub
      screenId="D-01"
      title="Dispatch Control"
      purpose="Priyantha's first screen of the day: KPI cards, the alert feed, and a live countdown to the 16:00 order cutoff."
      endpoints={[
        'GET /orders/queue?date=',
        'GET /routes/live',
        'GET /vehicles',
      ]}
      designNotes={[
        'The 16:00 countdown is live, not a static label — it is what makes the cutoff feel real.',
        'Alerts link straight to the screen that resolves them, not to a generic list.',
      ]}
    />
  );
}
