/**
 * Task 13.1: Child worker for strict-replay E2E.
 * Env: SOFTPROBE_CONFIG_PATH, UNRECORDED_URL
 * Fetches UNRECORDED_URL (not in cassette) under softprobe.run(REPLAY); expects 500.
 * Stdout: JSON { status, body }
 */

import path from 'path';

import '../../../init';
import { applyLegacyFrameworkPatches } from '../../../legacy';
import { softprobe } from '../../../api';
import { ConfigManager } from '../../../config/config-manager';
import { NodeSDK } from '@opentelemetry/sdk-node';
import { getNodeAutoInstrumentations } from '@opentelemetry/auto-instrumentations-node';

async function main(): Promise<void> {
  applyLegacyFrameworkPatches();
  const unrecordedUrl = process.env.UNRECORDED_URL;
  const configPath = process.env.SOFTPROBE_CONFIG_PATH ?? './.softprobe/config.yml';
  let cassetteDirectory: string | undefined;
  let traceId: string | undefined;
  let strictReplay = false;
  try {
    const cfg = new ConfigManager(configPath).get() as {
      cassetteDirectory?: string;
      traceId?: string;
      cassettePath?: string;
      replay?: { strictReplay?: boolean };
    };
    strictReplay = cfg.replay?.strictReplay === true;
    cassetteDirectory = cfg.cassetteDirectory;
    traceId = cfg.traceId;
    if (!cassetteDirectory || !traceId) {
      const fromPath = cfg.cassettePath;
      if (typeof fromPath === 'string' && fromPath) {
        cassetteDirectory = path.dirname(fromPath);
        traceId = path.basename(fromPath, '.case.json');
      }
    }
  } catch {
    cassetteDirectory = undefined;
    traceId = undefined;
  }
  if (!unrecordedUrl) throw new Error('UNRECORDED_URL is required');
  if (!cassetteDirectory || !traceId) {
    throw new Error('cassetteDirectory + traceId or cassettePath is required in config');
  }

  const sdk = new NodeSDK({
    instrumentations: [getNodeAutoInstrumentations()],
  });
  sdk.start();

  await softprobe.run(
    {
      mode: 'REPLAY',
      traceId,
      cassetteDirectory,
      strictReplay,
    },
    async () => {
      const response = await fetch(unrecordedUrl);
      const body = await response.text();
      process.stdout.write(JSON.stringify({ status: response.status, body }));
    }
  );
  try {
    await sdk.shutdown();
  } catch {
    /* ignore */
  }
  process.exit(0);
}

main().catch((err: unknown) => {
  process.stderr.write((err instanceof Error ? err.stack : String(err)) ?? '');
  process.exit(1);
});
