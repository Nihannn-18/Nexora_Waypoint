/**
 * @waypoint/api-client — typed access to the Go REST API.
 *
 * Framework-agnostic on purpose: it is plain `fetch`, so it works in a server
 * component, a client component and the Driver's service-worker outbox alike.
 */
export * from './lib/http';
export * from './lib/waypoint-client';
