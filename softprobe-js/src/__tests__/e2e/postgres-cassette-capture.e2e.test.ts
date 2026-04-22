/**
 * Task 12.2.1: CAPTURE script writes NDJSON with rows (Postgres E2E).
 * Runs a child process with CAPTURE config; asserts the cassette file
 * contains at least one outbound postgres record with responsePayload.rows.
 */

import fs from 'fs';
import path from 'path';
import { PostgreSqlContainer, StartedPostgreSqlContainer } from '@testcontainers/postgresql';
import { runChild } from './run-child';
import { loadCassetteRecordsByPath } from '../helpers/read-cassette-file';
import type { SoftprobeCassetteRecord } from '../../types/schema';
import { E2eArtifacts } from './helpers/e2e-artifacts';

const WORKER_SCRIPT = path.join(__dirname, 'helpers', 'pg-cassette-capture-worker.ts');
const REPLAY_WORKER = path.join(__dirname, 'helpers', 'pg-cassette-replay-worker.ts');

function getPostgresOutboundRecords(
  records: SoftprobeCassetteRecord[]
): SoftprobeCassetteRecord[] {
  return records.filter(
    (r) => r.type === 'outbound' && r.protocol === 'postgres'
  );
}

describe('E2E Postgres cassette capture (Task 12.2.1)', () => {
  let artifacts: E2eArtifacts;
  let pgContainer: StartedPostgreSqlContainer | undefined;
  let cassettePath: string;
  let captureConfigPath: string;

  beforeAll(async () => {
    artifacts = new E2eArtifacts();
    try {
      const timeoutMs = 15000;
      pgContainer = await new Promise<StartedPostgreSqlContainer>((resolve, reject) => {
        const t = setTimeout(() => reject(new Error('Docker start timeout')), timeoutMs);
        new PostgreSqlContainer('postgres:16')
          .start()
          .then((c) => {
            clearTimeout(t);
            resolve(c);
          })
          .catch(reject);
      });
    } catch (e) {
      console.warn('Skipping Postgres E2E: Docker unavailable', e);
      return;
    }
    cassettePath = artifacts.createTempFile('softprobe-e2e-cassette-pg', '.case.json');
    captureConfigPath = artifacts.createSoftprobeConfig('softprobe-e2e-pg-capture', {
      mode: 'CAPTURE',
      cassetteDirectory: path.dirname(cassettePath),
      traceId: path.basename(cassettePath, '.case.json'),
    });
  }, 60000);

  afterAll(async () => {
    await pgContainer?.stop();
    artifacts.cleanup();
  });

  it('12.2.1: CAPTURE script writes NDJSON with rows', async () => {
    if (!pgContainer) return;
    const result = runChild(
      WORKER_SCRIPT,
      {
        SOFTPROBE_CONFIG_PATH: captureConfigPath,
        PG_URL: pgContainer.getConnectionUri(),
      },
      { useTsNode: true }
    );

    expect(result.exitCode).toBe(0);
    expect(result.stderr).toBe('');

    expect(fs.existsSync(cassettePath)).toBe(true);
    const records = await loadCassetteRecordsByPath(cassettePath);
    const pgRecords = getPostgresOutboundRecords(records);
    expect(pgRecords.length).toBeGreaterThanOrEqual(1);

    for (const rec of pgRecords) {
      expect(rec.version).toBe('4.1');
      expect(rec.identifier).toBeDefined();
      expect(rec.responsePayload).toBeDefined();
      const payload = rec.responsePayload as { rows?: unknown[] };
      expect(Array.isArray(payload.rows)).toBe(true);
      expect(payload.rows!.length).toBeGreaterThanOrEqual(0);
    }
  }, 60000);
});

/**
 * Task 12.2.2: REPLAY works with DB disconnected.
 * Capture uses a real Postgres container to record; replay uses the cassette only (dummy PG URL).
 */
describe('E2E Postgres cassette replay (Task 12.2.2)', () => {
  let artifacts: E2eArtifacts;
  let pgContainer: StartedPostgreSqlContainer | undefined;
  let cassettePath: string;
  let captureConfigPath: string;
  let replayConfigPath: string;

  beforeAll(async () => {
    artifacts = new E2eArtifacts();
    try {
      const timeoutMs = 15000;
      pgContainer = await new Promise<StartedPostgreSqlContainer>((resolve, reject) => {
        const t = setTimeout(() => reject(new Error('Docker start timeout')), timeoutMs);
        new PostgreSqlContainer('postgres:16')
          .start()
          .then((c) => {
            clearTimeout(t);
            resolve(c);
          })
          .catch(reject);
      });
    } catch (e) {
      console.warn('Skipping Postgres E2E replay: Docker unavailable', e);
      return;
    }
    cassettePath = artifacts.createTempFile('softprobe-e2e-replay-pg', '.case.json');
    const cassetteDirectory = path.dirname(cassettePath);
    const traceId = path.basename(cassettePath, '.case.json');
    captureConfigPath = artifacts.createSoftprobeConfig('softprobe-e2e-pg-replay-capture', {
      mode: 'CAPTURE',
      cassetteDirectory,
      traceId,
    });
    replayConfigPath = artifacts.createSoftprobeConfig('softprobe-e2e-pg-replay', {
      mode: 'REPLAY',
      cassetteDirectory,
      traceId,
      strictReplay: true,
    });
  }, 60000);

  afterAll(async () => {
    await pgContainer?.stop();
    artifacts.cleanup();
  });

  it('12.2.2: REPLAY script works with DB disconnected', async () => {
    if (!pgContainer) return;
    // CAPTURE step: run worker with a real Postgres URL so it can record the query into the cassette.
    const captureResult = runChild(
      WORKER_SCRIPT,
      {
        SOFTPROBE_CONFIG_PATH: captureConfigPath,
        PG_URL: pgContainer.getConnectionUri(), // valid URL — real Postgres container
      },
      { useTsNode: true }
    );
    expect(captureResult.exitCode).toBe(0);

    const records = await loadCassetteRecordsByPath(cassettePath);
    const recordedTraceId = records.find(
      (r) => r.type === 'outbound' && r.protocol === 'postgres'
    )?.traceId;

    const replayResult = runChild(
      REPLAY_WORKER,
      {
        SOFTPROBE_CONFIG_PATH: replayConfigPath,
        PG_URL: 'postgres://127.0.0.1:63999/offline',
        ...(recordedTraceId && { REPLAY_TRACE_ID: recordedTraceId }),
      },
      { useTsNode: true }
    );

    expect(replayResult.exitCode).toBe(0);
    expect(replayResult.stderr).toBe('');
    const replayStdout = replayResult.stdout.trim();
    if (replayStdout) {
      const replayed = JSON.parse(replayStdout) as { rows: unknown[]; rowCount: number };
      expect(Array.isArray(replayed.rows)).toBe(true);
      expect(replayed.rowCount).toBeGreaterThanOrEqual(0);
    }
  }, 30000);
});
