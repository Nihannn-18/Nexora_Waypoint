import { redirect } from 'next/navigation';

/**
 * There is no public landing page: every surface is behind a role. The root
 * sends people to sign-in (G-01), which is where a judge starts the walkthrough.
 */
export default function Index() {
  redirect('/signin');
}
