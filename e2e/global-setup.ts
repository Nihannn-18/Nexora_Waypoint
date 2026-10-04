import { request } from '@playwright/test';

const HOST = process.env.E2E_API_URL ?? 'http://localhost:8080';
const api = (path: string) => `/api/v1${path}`;
const DATE = '2026-09-26';
const DISPATCHER = 'priyantha.w@waypoint.lk';
const PASSWORD = 'waypoint2026';

/**
 * Bring the running stack to the documented walkthrough start: the demo day is
 * re-seeded, the queue is closed and one full plan is confirmed. The Loader then
 * has trips to load and the Driver has a run with stops.
 */
export default async function globalSetup(): Promise<void> {
  const ctx = await request.newContext({ baseURL: HOST });
  const login = await ctx.post(api('/auth/login'), { data: { email: DISPATCHER, password: PASSWORD } });
  if (!login.ok()) throw new Error(`dispatcher login failed: ${login.status()}`);
  const { token } = await login.json();
  const auth = { Authorization: `Bearer ${token}` };

  const reset = await ctx.post(api('/demo/reset'), { headers: auth });
  if (!reset.ok()) throw new Error(`demo reset failed: ${reset.status()}`);

  const depots = await (await ctx.get(api('/depots'), { headers: auth })).json();
  const depotId = depots.depots.find((d: { code: string }) => d.code === 'PELIYAGODA')?.depotId;
  if (!depotId) throw new Error('Peliyagoda depot not found');

  const close = await ctx.post(api('/orders/close'), { headers: auth, data: { date: DATE, depotId } });
  if (!close.ok()) throw new Error(`queue close failed: ${close.status()} ${await close.text()}`);

  const suggest = await ctx.post(api('/allocations/suggest'), { headers: auth, data: { planningDate: DATE, depotId } });
  if (suggest.status() !== 202) throw new Error(`suggest failed: ${suggest.status()} ${await suggest.text()}`);
  const { jobId } = await suggest.json();

  let job: { status: string } = { status: 'QUEUED' };
  for (let i = 0; i < 100 && job.status !== 'COMPLETED' && job.status !== 'FAILED'; i++) {
    job = await (await ctx.get(api(`/planning-jobs/${jobId}`), { headers: auth })).json();
    if (job.status === 'QUEUED' || job.status === 'RUNNING') {
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
  }
  if (job.status !== 'COMPLETED') throw new Error(`planning job ended ${job.status}`);

  const results = await (await ctx.get(api(`/planning-jobs/${jobId}/results`), { headers: auth })).json();
  type Proposal = {
    orderId: string; decision: string; vehicleId?: string; tripNo?: number; seq?: number;
    constraintCode?: string; explanation?: string;
  };
  const proposals: Proposal[] = results.proposals;

  const served = proposals
    .filter((p) => p.decision === 'SERVE')
    .sort((a, b) => (a.vehicleId ?? '').localeCompare(b.vehicleId ?? '') || (a.tripNo ?? 0) - (b.tripNo ?? 0) || (a.seq ?? 0) - (b.seq ?? 0));

  const groups = new Map<string, { vehicleId: string; tripNo: number; orderIds: string[] }>();
  for (const p of served) {
    const key = `${p.vehicleId}:${p.tripNo}`;
    if (!groups.has(key)) groups.set(key, { vehicleId: p.vehicleId!, tripNo: p.tripNo!, orderIds: [] });
    groups.get(key)!.orderIds.push(p.orderId);
  }

  const deferrals = proposals
    .filter((p) => p.decision === 'DEFER')
    .map((p) => ({
      orderId: p.orderId,
      reasonType: 'CONSTRAINT',
      constraintCode: p.constraintCode,
      reasonText: p.explanation || 'No feasible trip remains.',
      deferredToDate: '2026-09-28',
    }));

  const confirm = await ctx.post(api('/allocations/confirm'), {
    headers: auth,
    data: { jobId, routes: [...groups.values()], deferrals },
  });
  if (!confirm.ok()) throw new Error(`confirm failed: ${confirm.status()} ${await confirm.text()}`);

  await ctx.dispose();
}
