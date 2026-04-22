/**
 * Task 13.1: Strict replay — unrecorded call hard-fails and does not touch real network.
 * Test: attempt unrecorded identifier; assert thrown (exit 1 + stderr) and verify passthrough not called.
 */

import fs from 'fs';
import path from 'path';
import { runChild } from './run-child';
import { E2eArtifacts } from './helpers/e2e-artifacts';
import { buildCaseDocumentFromRecords } from '../../core/cassette/case-bridge';
import type { SoftprobeCassetteRecord } from '../../types/schema';

const STRICT_REPLAY_WORKER = path.join(__dirname, 'helpers', 'http-strict-replay-worker.ts');

function buildMinimalCaseFile(traceId: string, identifier: string): string {
  const record: SoftprobeCassetteRecord = {
    version: '4.1',
    traceId,
    spanId: 'span-1',
    timestamp: new Date().toISOString(),
    type: 'outbound',
    protocol: 'http',
    identifier,
    responsePayload: { statusCode: 200, body: { ok: true } },
  };
  return `${JSON.stringify(buildCaseDocumentFromRecords([record], { caseId: traceId, mode: 'replay' }), null, 2)}\n`;
}

describe('E2E strict mode (Task 13.1)', () => {
  let artifacts: E2eArtifacts;
  let cassettePath: string;
  let replayConfigPath: string;
  const RECORDED_IDENTIFIER = 'GET http://example.com/recorded';
  const UNRECORDED_URL = 'http://example.com/unrecorded';

  beforeAll(() => {
    artifacts = new E2eArtifacts();
    cassettePath = artifacts.createTempFile('softprobe-e2e-strict', '.case.json');
    replayConfigPath = artifacts.createSoftprobeConfig('softprobe-e2e-strict-replay', {
      mode: 'REPLAY',
      cassetteDirectory: path.dirname(cassettePath),
      traceId: path.basename(cassettePath, '.case.json'),
      strictReplay: true,
    });
    const traceIdForFile = path.basename(cassettePath, '.case.json');
    fs.writeFileSync(cassettePath, buildMinimalCaseFile(traceIdForFile, RECORDED_IDENTIFIER), 'utf8');
  });

  afterAll(() => {
    artifacts.cleanup();
  });

  it('13.1: in strict replay, unrecorded call hard-fails and does not touch real network', () => {
    const result = runChild(
      STRICT_REPLAY_WORKER,
      {
        SOFTPROBE_CONFIG_PATH: replayConfigPath,
        UNRECORDED_URL,
      },
      { useTsNode: true }
    );

    expect(result.stderr).toBe('');
    const out = JSON.parse(result.stdout) as { status: number; body: string };
    expect(out.status).toBe(500);
    expect(out.body).toContain('[Softprobe]');
    expect(out.body).toMatch(/no recorded traces|No recorded traces/i);
    // If passthrough had been called, the request would hit the real network and we would not get our softprobe 500.
    expect(result.exitCode).toBe(0);
  });
});
