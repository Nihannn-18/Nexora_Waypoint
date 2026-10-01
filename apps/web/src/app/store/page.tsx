import { ScreenStub } from '../../components/screen-stub';

export default function StoreHomePage() {
  return (
    <ScreenStub
      screenId="S-01"
      title="Store home"
      purpose="Ishara's standing question: is tomorrow's delivery coming, and when?"
      endpoints={[
        'GET /orders/{id}',
        'GET /orders/{id}/eta',
        'POST /orders/{id}/receipt',
      ]}
      designNotes={[
        'A deferral notice (S-05) carries the REASON, not just the fact — that is the whole point of recording one.',
        'Fresh places dry and chilled as separate orders for the same day; never merge them into one.',
      ]}
    />
  );
}
