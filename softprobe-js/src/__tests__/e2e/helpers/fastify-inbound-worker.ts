/**
 * Shared Fastify app for inbound capture and replay E2E (Task 14.4.3).
 * Mode is driven by YAML config (SOFTPROBE_CONFIG_PATH).
 * - CAPTURE: cassettePath in config; GET /exit awaits pending case writes then exits.
 * - REPLAY: uses runtime replay wiring from init + middleware/context run path; GET /exit exits.
 * PORT required for both.
 * Same route flow as Express worker: GET / does outbound fetch to httpbin.org.
 */

import path from 'path';

import '../../../init';
import { ConfigManager } from '../../../config/config-manager';
import { SoftprobeContext } from '../../../context';
import { applyLegacyFrameworkPatches } from '../../../legacy';
import { NodeSDK } from '@opentelemetry/sdk-node';
import { getNodeAutoInstrumentations } from '@opentelemetry/auto-instrumentations-node';

const sdk = new NodeSDK({
  instrumentations: [getNodeAutoInstrumentations()],
});
sdk.start();

const configPath = process.env.SOFTPROBE_CONFIG_PATH ?? './.softprobe/config.yml';
let softprobeMode = 'PASSTHROUGH';
try {
  const cfg = new ConfigManager(configPath).get() as {
    mode?: string;
    cassettePath?: string;
    cassetteDirectory?: string;
    replay?: { strictReplay?: boolean; strictComparison?: boolean };
  };
  softprobeMode = cfg.mode ?? 'PASSTHROUGH';
  const cassetteDirectory =
    cfg.cassetteDirectory ??
    (typeof cfg.cassettePath === 'string' && cfg.cassettePath ? path.dirname(cfg.cassettePath) : undefined);
  SoftprobeContext.initGlobal({
    mode: softprobeMode,
    cassetteDirectory,
    strictReplay: cfg.replay?.strictReplay,
    strictComparison: cfg.replay?.strictComparison,
  });
} catch {
  softprobeMode = 'PASSTHROUGH';
}
applyLegacyFrameworkPatches();

async function startServer(): Promise<void> {
  const fastify = require('fastify');
  // Disable logger so Fastify does not call os.networkInterfaces() on listen (fails in sandbox/restricted envs).
  const app = await fastify({ logger: false });

  app.get('/', async (_req: unknown, reply: { status: (n: number) => { send: (body: unknown) => Promise<unknown> }; send: (body: unknown) => Promise<unknown> }) => {
    const r = await fetch('https://httpbin.org/get', { signal: AbortSignal.timeout(15000) });
    const j = (await r.json()) as Record<string, unknown>;
    return reply.status(200).send({ ok: true, outbound: j });
  });

  /** Route that performs an outbound call not in the default fixture (for strict-negative E2E). Propagates outbound error status so client sees failure when strict replay returns 500. */
  app.get('/unrecorded', async (_req: unknown, reply: { status: (n: number) => { send: (body: unknown) => Promise<unknown> }; send: (body: unknown) => Promise<unknown> }) => {
    const r = await fetch('https://httpbin.org/post', { method: 'POST', body: '{}', signal: AbortSignal.timeout(15000) });
    const j = (await r.json()) as Record<string, unknown>;
    if (!r.ok) return reply.status(r.status).send(j);
    return reply.status(200).send({ ok: true, outbound: j });
  });

  app.get('/exit', (_req: unknown, reply: { send: (s: string) => unknown }) => {
    reply.send('ok');
    setImmediate(() => {
      void SoftprobeContext.flushPendingCassettes()
        .then(() => process.exit(0))
        .catch(() => process.exit(0));
    });
  });

  const port = parseInt(process.env.PORT || '0', 10) || 39302;
  await app.listen({ port, host: '0.0.0.0' });
  process.stdout.write(JSON.stringify({ port }) + '\n');
}

startServer().catch((err: unknown) => {
  process.stderr.write((err instanceof Error ? err.stack : String(err)) ?? '');
  process.exit(1);
});
