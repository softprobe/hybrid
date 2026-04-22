/**
 * Task 21.2.1: softprobe diff CLI — run diff and assert the server receives the correctly injected headers.
 * Task 21.3.1: CLI shows colored diff and exits 1 when recorded vs live response differs.
 */

import fs from 'fs';
import os from 'os';
import path from 'path';
import { spawnSync } from 'child_process';
import { runServer, waitForServer, closeServer } from './e2e/run-child';
import { runDiff } from '../cli/diff';
import type { SoftprobeCassetteRecord } from '../types/schema';
import { buildCaseDocumentFromRecords } from '../core/cassette/case-bridge';

const DIFF_HEADERS_SERVER = path.join(__dirname, 'e2e', 'helpers', 'diff-headers-server.ts');
const PROJECT_ROOT = path.resolve(__dirname, '..', '..');

/** Run softprobe diff CLI; returns stdout, stderr, exit code. Uses ts-node so tests run against current source. */
function runDiffCli(cassettePath: string, targetUrl: string): { stdout: string; stderr: string; exitCode: number } {
  const srcCli = path.join(PROJECT_ROOT, 'src', 'cli.ts');
  const result = spawnSync('npx', ['ts-node', '--transpile-only', srcCli, 'diff', cassettePath, targetUrl], {
    encoding: 'utf-8',
    cwd: PROJECT_ROOT,
    env: process.env,
  });
  return {
    stdout: result.stdout ?? '',
    stderr: result.stderr ?? '',
    exitCode: result.status ?? -1,
  };
}

describe('Task 21.2.1: softprobe diff CLI', () => {
  it('run diff sends request with correctly injected coordination headers; server receives them', async () => {
    const port = 39500 + (Date.now() % 1000);
    const child = runServer(
      DIFF_HEADERS_SERVER,
      { PORT: String(port) },
      { useTsNode: true }
    );
    await waitForServer(port, 20000);

    const traceId = 'trace-diff-99';
    const cassetteDir = fs.mkdtempSync(path.join(os.tmpdir(), 'diff-cli-'));
    const cassettePath = path.join(cassetteDir, `${traceId}.case.json`);
    const inboundRecord: SoftprobeCassetteRecord = {
      version: '4.1',
      traceId,
      spanId: 'span1',
      timestamp: new Date().toISOString(),
      type: 'inbound',
      protocol: 'http',
      identifier: 'GET /diff-headers',
    };
    fs.writeFileSync(
      cassettePath,
      JSON.stringify(buildCaseDocumentFromRecords([inboundRecord], { caseId: traceId, mode: 'replay' }))
    );

    try {
      const { response } = await runDiff(cassettePath, `http://127.0.0.1:${port}`);
      const receivedHeaders = (await response.json()) as Record<string, string>;

      expect(receivedHeaders['x-softprobe-mode']).toBe('REPLAY');
      expect(receivedHeaders['x-softprobe-trace-id']).toBe('trace-diff-99');
    } finally {
      await closeServer(child);
      fs.rmSync(cassetteDir, { recursive: true, force: true });
    }
  }, 15000);

  it('ignores request body for GET cassette identifiers so diff does not throw', async () => {
    const port = 39700 + (Date.now() % 1000);
    const child = runServer(
      DIFF_HEADERS_SERVER,
      { PORT: String(port) },
      { useTsNode: true }
    );
    await waitForServer(port, 20000);

    const traceId = 'trace-diff-get-body';
    const cassetteDir = fs.mkdtempSync(path.join(os.tmpdir(), 'diff-cli-get-'));
    const cassettePath = path.join(cassetteDir, `${traceId}.case.json`);
    const inboundRecord: SoftprobeCassetteRecord = {
      version: '4.1',
      traceId,
      spanId: 'span1',
      timestamp: new Date().toISOString(),
      type: 'inbound',
      protocol: 'http',
      identifier: 'GET /diff-headers',
      requestPayload: { body: { unexpected: true } },
    };
    fs.writeFileSync(
      cassettePath,
      JSON.stringify(buildCaseDocumentFromRecords([inboundRecord], { caseId: traceId, mode: 'replay' }))
    );

    try {
      await expect(runDiff(cassettePath, `http://127.0.0.1:${port}`)).resolves.toMatchObject({
        response: expect.objectContaining({ status: 200 }),
      });
    } finally {
      await closeServer(child);
      fs.rmSync(cassetteDir, { recursive: true, force: true });
    }
  }, 15000);

  it('Task 21.3.1: on status/body mismatch CLI shows colored diff and exits with code 1', async () => {
    const port = 39600 + (Date.now() % 1000);
    const child = runServer(
      DIFF_HEADERS_SERVER,
      { PORT: String(port) },
      { useTsNode: true }
    );
    await waitForServer(port, 20000);

    const traceId = 'trace-diff-mismatch';
    const cassetteDir = fs.mkdtempSync(path.join(os.tmpdir(), 'diff-mismatch-'));
    const cassettePath = path.join(cassetteDir, `${traceId}.case.json`);
    const inboundRecord: SoftprobeCassetteRecord = {
      version: '4.1',
      traceId,
      spanId: 'span1',
      timestamp: new Date().toISOString(),
      type: 'inbound',
      protocol: 'http',
      identifier: 'GET /diff-mismatch',
      responsePayload: { statusCode: 200, body: { ok: true } },
    };
    fs.writeFileSync(
      cassettePath,
      JSON.stringify(buildCaseDocumentFromRecords([inboundRecord], { caseId: traceId, mode: 'replay' }))
    );

    try {
      const targetUrl = `http://127.0.0.1:${port}`;
      const { stdout, stderr, exitCode } = runDiffCli(cassettePath, targetUrl);
      const output = stdout + stderr;

      expect(exitCode).toBe(1);
      expect(output).toMatch(/200/);
      expect(output).toMatch(/500/);
      expect(output).toMatch(/status|mismatch|recorded|live|diff/i);
    } finally {
      await closeServer(child);
      fs.rmSync(cassetteDir, { recursive: true, force: true });
    }
  }, 30000);
});
