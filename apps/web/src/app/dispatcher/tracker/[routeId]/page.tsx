import type { Metadata } from 'next';
import { RouteDetail } from './route-detail';

export const metadata: Metadata = { title: 'Route' };

export default async function RoutePage({
  params,
}: {
  params: Promise<{ routeId: string }>;
}) {
  const { routeId } = await params;
  return <RouteDetail routeId={routeId} />;
}
