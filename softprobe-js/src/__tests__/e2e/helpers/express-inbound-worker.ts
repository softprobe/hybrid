/**
 * Shared Express app for inbound capture and replay E2E (Tasks 14.4.1, 14.4.2).
 *
 * Inbound vs outbound (for capture/replay):
 * - Inbound = HTTP request INTO this app (e.g. GET /). Express middleware records it (type 'inbound').
 * - Outbound = Calls this app makes TO external backends (e.g. fetch()). Instrumentation records them (type 'outbound').
 *
 * Mode is driven by YAML config (SOFTPROBE_CONFIG_PATH).
 * - CAPTURE: cassettePath in config; GET /exit awaits pending case writes then exits.
 * - REPLAY: uses runtime replay wiring from init + middleware/context run path; GET /exit exits.
 * - SOFTPROBE_E2E_OUTBOUND_URL optionally overrides default outbound URL for deterministic local tests.
 * - SOFTPROBE_E2E_UNRECORDED_URL optionally overrides strict-negative outbound URL.
 * PORT required for both.
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
const outboundUrl = process.env.SOFTPROBE_E2E_OUTBOUND_URL || 'https://httpbin.org/get';
const unrecordedUrl = process.env.SOFTPROBE_E2E_UNRECORDED_URL || 'https://httpbin.org/post';

async function startServer(): Promise<void> {
  const express = require('express');
  const app = express();

  // INBOUND: This route is the "inbound" — the request comes INTO our app (GET /).
  // Express/Softprobe middleware records it as type 'inbound' (request + response).
  app.get('/', async (_req: unknown, res: { status: (n: number) => { json: (body: unknown) => void }; json: (body: unknown) => void }) => {
    // OUTBOUND: This fetch() is a call FROM our app TO an external backend.
    // Instrumentation records it as type 'outbound' (e.g. protocol 'http').
    const r = await fetch(outboundUrl, { signal: AbortSignal.timeout(15000) });
    const j = (await r.json()) as Record<string, unknown>;
    res.status(200).json({ ok: true, outbound: j });
  });

  /** INBOUND: GET /unrecorded is the inbound request. OUTBOUND: fetch(unrecordedUrl) below. In strict replay the unrecorded outbound fails. */
  app.get('/unrecorded', async (_req: unknown, res: { status: (n: number) => { json: (body: unknown) => void } }) => {
    const r = await fetch(unrecordedUrl, { signal: AbortSignal.timeout(15000) });
    const j = (await r.json()) as Record<string, unknown>;
    if (!r.ok) return res.status(r.status).json(j);
    return res.status(200).json({ ok: true, outbound: j });
  });

  app.get('/exit', (_req: unknown, res: { send: (s: string) => void }) => {
    res.send('ok');
    setImmediate(() => {
      void SoftprobeContext.flushPendingCassettes()
        .then(() => process.exit(0))
        .catch(() => process.exit(0));
    });
  });

  const port = parseInt(process.env.PORT || '0', 10) || 39301;
  app.listen(port, () => {
    process.stdout.write(JSON.stringify({ port }) + '\n');
  });
}

startServer().catch((err: unknown) => {
  process.stderr.write((err instanceof Error ? err.stack : String(err)) ?? '');
  process.exit(1);
});
