import path from 'path';
import { execSync } from 'child_process';
import { Softprobe } from '@softprobe/softprobe-js';

const softprobe = new Softprobe();  // reads SOFTPROBE_API_TOKEN
const CASE_FILE = path.resolve(__dirname, '../cases/hello.case.json');
const APP_URL = process.env.APP_URL ?? 'http://127.0.0.1:8082';

describe('GET /hello', () => {
  let sessionId = '';
  let close: () => Promise<void> = async () => {};

  beforeAll(async () => {
    const session = await softprobe.startSession({ mode: 'replay' });
    sessionId = session.id;
    close = () => session.close();

    await session.loadCaseFromFile(CASE_FILE);

    const hit = session.findInCase({
      direction: 'outbound',
      path: '/fragment',
    });

    await session.mockOutbound({
      name: 'fragment-mock',
      direction: 'outbound',
      path: '/fragment',
      response: hit.response,
    });

    // Stop the upstream to prove the test never hits the real network.
    execSync('docker compose stop upstream', { cwd: path.resolve(__dirname, '..') });
  });

  afterAll(async () => {
    execSync('docker compose start upstream', { cwd: path.resolve(__dirname, '..') });
    await close();
  });

  it('returns the captured upstream response without hitting the network', async () => {
    const res = await fetch(`${APP_URL}/hello`, {
      headers: { 'x-softprobe-session-id': sessionId },
    });

    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ message: 'hello', dep: 'ok' });
  });
});
