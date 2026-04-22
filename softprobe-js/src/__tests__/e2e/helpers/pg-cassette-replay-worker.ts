/**
 * Task 12.2.2: Child worker for Postgres NDJSON replay E2E (DB disconnected).
 * Loads softprobe/init (REPLAY), then runs query under softprobe.run(REPLAY); responses are
 * mocked from the case JSON cassette — no real DB connection.
 *
 * Env: SOFTPROBE_CONFIG_PATH
 * Stdout: JSON { rows, rowCount } from replayed query.
 */

import path from 'path';
import { ConfigManager } from '../../../config/config-manager';
import { softprobe } from '../../../api';

const initPath = path.join(__dirname, '..', '..', '..', 'init.ts');
require(initPath);
const { applyLegacyFrameworkPatches } = require('../../../legacy');
applyLegacyFrameworkPatches();

async function main() {
  const { NodeSDK } = require('@opentelemetry/sdk-node');
  const { getNodeAutoInstrumentations } = require('@opentelemetry/auto-instrumentations-node');
  const sdk = new NodeSDK({ instrumentations: getNodeAutoInstrumentations() });
  sdk.start();

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
  if (!cassetteDirectory || !traceId) {
    process.stderr.write('cassetteDirectory + traceId or cassettePath is required in config');
    process.exit(1);
  }

  let output: { rows: unknown[]; rowCount: number } | undefined;
  await softprobe.run(
    {
      mode: 'REPLAY',
      traceId,
      cassetteDirectory,
      strictReplay,
    },
    async () => {
    const { Client } = require('pg');
    const client = new Client({ connectionString: process.env.PG_URL || 'postgres://localhost:9999/nodb' });
    const queryText = 'SELECT 1 AS num, $1::text AS label';
    const values = ['e2e-cassette'];
    const result = await client.query(queryText, values);
    output = { rows: result.rows, rowCount: result.rowCount };
    }
  );

  try {
    await sdk.shutdown();
  } catch {
    /* ignore */
  }
  process.stdout.write(JSON.stringify(output ?? { rows: [], rowCount: 0 }));
}

main().catch((err) => {
  process.stderr.write(err.stack ?? String(err));
  process.exit(1);
});
