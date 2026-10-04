import { redirect } from 'next/navigation';

/** The vehicle list is Fleet (D-09); this path stays valid for old links. */
export default function VehiclesPage() {
  redirect('/dispatcher/fleet');
}
