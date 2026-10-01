/**
 * @waypoint/shared-types — the contract between the Go API and the Next.js client.
 *
 * Nothing in here imports React, Next, or any runtime dependency: it is types
 * plus pure functions, so the planning arithmetic can be unit-tested on its own
 * and reused by any surface.
 */
export * from './lib/domain';
export * from './lib/constraints';
export * from './lib/entities';
export * from './lib/requests';
export * from './lib/trip-time';
