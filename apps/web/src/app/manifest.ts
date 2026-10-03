import type { MetadataRoute } from 'next';

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: 'Waypoint Driver',
    short_name: 'Waypoint',
    description: 'Run sheet and offline delivery capture for Waypoint drivers.',
    start_url: '/driver',
    scope: '/',
    display: 'standalone',
    orientation: 'portrait',
    background_color: '#eef2f8',
    theme_color: '#019eff',
    icons: [
      { src: '/icon.svg', sizes: 'any', type: 'image/svg+xml', purpose: 'any' },
      { src: '/icon.svg', sizes: 'any', type: 'image/svg+xml', purpose: 'maskable' },
    ],
  };
}
