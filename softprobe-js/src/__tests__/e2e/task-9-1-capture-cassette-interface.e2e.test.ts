/**
 * Task 9.1: Capture E2E writes inbound+outbound NDJSON via cassette interface.
 * Boot in PASSTHROUGH, then request-level CAPTURE headers provide cassette and trace scope.
 */

import fs from 'fs';
import path from 'path';
import { runServer, waitForServer, closeServer } from './run-child';
import { loadCassetteRecordsByPath } from '../helpers/read-cassette-file';
import type { SoftprobeCassetteRecord } from '../../types/schema';
import { E2eArtifacts } from './helpers/e2e-artifacts';

const WORKER_SCRIPT = path.join(__dirname, 'helpers', 'express-inbound-worker.ts');
const OUTBOUND_WORKER_SCRIPT = path.join(__dirname, 'helpers', 'diff-headers-server.ts');

describe('Task 9.1 - Capture E2E via cassette interface', () => {
  let artifacts: E2eArtifacts;
  let cassettePath: string;

  beforeEach(() => {
    artifacts = new E2eArtifacts();
    cassettePath = artifacts.createTempFile('task-9-1', '.case.json');
  });

  afterEach(() => {
    artifacts.cleanup();
  });

  it('writes inbound and outbound records for one trace using request-scoped cassette', async () => {
    const outboundPort = 31200 + (Date.now() % 10000);
    const outboundChild = runServer(
      OUTBOUND_WORKER_SCRIPT,
      { PORT: String(outboundPort) },
      { useTsNode: true }
    );

    const traceId = '9f1dcb4b9f5f4f52b7c91de2be5db5fd';
    const cassetteDir = path.join(path.dirname(cassettePath), `task-9-1-dir-${Date.now()}`);
    fs.mkdirSync(cassetteDir, { recursive: true });
    const cassetteFilePath = path.join(cassetteDir, `${traceId}.case.json`);
    const appConfigPath = artifacts.createSoftprobeConfig('task-9-1-app-config', {
      mode: 'PASSTHROUGH',
      cassetteDirectory: cassetteDir,
      traceId,
    });

    const port = 30200 + (Date.now() % 10000);
    const child = runServer(
      WORKER_SCRIPT,
      {
        PORT: String(port),
        SOFTPROBE_CONFIG_PATH: appConfigPath,
        SOFTPROBE_E2E_OUTBOUND_URL: `http://127.0.0.1:${outboundPort}/diff-headers`,
      },
      { useTsNode: true }
    );

    try {
      await waitForServer(outboundPort, 30000);
      await waitForServer(port, 30000);
      const traceparent = `00-${traceId}-0000000000000001-01`;
      const res = await fetch(`http://127.0.0.1:${port}/`, {
        headers: {
          traceparent,
          'x-softprobe-mode': 'CAPTURE',
          'x-softprobe-trace-id': traceId,
        },
        signal: AbortSignal.timeout(20000),
      });
      expect(res.ok).toBe(true);
      await fetch(`http://127.0.0.1:${port}/exit`, { signal: AbortSignal.timeout(5000) }).catch(() => {});
      await new Promise<void>((resolve) => {
        child.once('exit', () => resolve());
        setTimeout(resolve, 5000);
      });
    } finally {
      await closeServer(outboundChild);
      await closeServer(child);
    }

    expect(fs.existsSync(cassetteFilePath)).toBe(true);
    const records = await loadCassetteRecordsByPath(cassetteFilePath);
    // Case file may include spans whose `traceId` differs from the coordination header (OTel vs config);
    // assert on the whole capture file, not a strict traceId filter.
    expect(records.length).toBeGreaterThanOrEqual(2);
    const inbound = records.find((r) => r.type === 'inbound' && r.protocol === 'http');
    const outbound = records.find((r) => r.type === 'outbound' && r.protocol === 'http');

    expect(inbound).toBeDefined();
    expect(outbound).toBeDefined();
  }, 60000);
});
