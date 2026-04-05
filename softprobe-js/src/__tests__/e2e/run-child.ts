/**
 * E2E helper: spawn a Node script with env and return stdout and exit code.
 * Used so capture/replay workloads run in a child process with native require
 * (Jest's module loader breaks require-in-the-middle instrumentations).
 */

import path from 'path';
import fs from 'fs';
import { spawn, spawnSync, type ChildProcess } from 'child_process';

export interface RunChildOptions {
  /** When true, run script with `npx ts-node` so .ts files work. */
  useTsNode?: boolean;
}

const LEGACY_SOFTPROBE_ENV_KEYS = [
  'SOFTPROBE_CASSETTE_PATH',
  'SOFTPROBE_CASSETTE_DIRECTORY',
  'SOFTPROBE_STRICT_REPLAY',
  'SOFTPROBE_STRICT_COMPARISON',
] as const;

function validateSoftprobeEnv(env: Record<string, string>): Record<string, string> {
  const childEnv = { ...env };
  const hasLegacy = LEGACY_SOFTPROBE_ENV_KEYS.some((k) => childEnv[k] != null && childEnv[k] !== '');
  if (hasLegacy) {
    throw new Error(
      'SOFTPROBE_CONFIG_PATH is required for E2E workers; legacy SOFTPROBE_CASSETTE_* env is not supported'
    );
  }
  return childEnv;
}

/**
 * Spawns a Node script with the given env. Returns stdout, stderr, and exit code.
 * With useTsNode: true, runs `node -r ts-node/register/transpile-only scriptPath` so TypeScript
 * workers run without invoking npx (avoids npm warnings on stderr).
 */
export function runChild(
  scriptPath: string,
  env: Record<string, string> = {},
  options: RunChildOptions = {}
): { stdout: string; stderr: string; exitCode: number } {
  const { useTsNode } = options;
  const childEnv = validateSoftprobeEnv(env);
  const args = useTsNode
    ? ['-r', 'ts-node/register/transpile-only', scriptPath]
    : [scriptPath];
  const executable = process.execPath;
  const result = spawnSync(executable, args, {
    encoding: 'utf-8',
    env: { ...process.env, ...childEnv },
    cwd: path.resolve(__dirname, '..', '..', '..'),
  });
  return {
    stdout: result.stdout ?? '',
    stderr: result.stderr ?? '',
    exitCode: result.status ?? -1,
  };
}

/** Project root (repo root when tests run from repo). */
const PROJECT_ROOT = path.resolve(__dirname, '..', '..', '..');

// #region agent log
function _dbg(_location: string, _message: string, _data: Record<string, unknown>, _hypothesisId: string): void {
  // No-op in test: avoid fetch() to 127.0.0.1:7242 keeping the process alive when nothing is listening.
  // fetch(..., { signal: AbortSignal.timeout(500) }).catch(() => {});
}
// #endregion

/**
 * Spawns a long-running Node script (e.g. Express server). Caller must call
 * closeServer(child) when done so pipes are destroyed and Jest can exit.
 */
export function runServer(
  scriptPath: string,
  env: Record<string, string> & { PORT: string },
  options: RunChildOptions = {}
): ChildProcess {
  const { useTsNode = true } = options;
  const childEnv = validateSoftprobeEnv(env);
  const port = env.PORT;
  // #region agent log
  _dbg('run-child.ts:runServer', 'runServer called', { port, scriptPath, cwd: PROJECT_ROOT, useTsNode }, 'A');
  // #endregion
  const args = useTsNode
    ? ['-r', 'ts-node/register/transpile-only', scriptPath]
    : [scriptPath];
  const executable = process.execPath;
  const child = spawn(executable, args, {
    env: { ...process.env, ...childEnv },
    stdio: ['ignore', 'pipe', 'pipe'],
    cwd: PROJECT_ROOT,
  });
  // #region agent log
  _dbg('run-child.ts:runServer', 'spawn done', { port, pid: child.pid }, 'E');
  // #endregion
  return child;
}

/**
 * Kills the child if still running, waits for exit, then destroys stdio streams
 * so Jest does not report open PIPEWRAP handles.
 */
export async function closeServer(child: ChildProcess): Promise<void> {
  if (child.exitCode === null) {
    child.kill('SIGKILL');
    await new Promise<void>((r) => {
      const t = setTimeout(r, 5000);
      child.once('exit', () => {
        clearTimeout(t);
        r();
      });
    });
  }
  child.stdin?.destroy();
  child.stdout?.destroy();
  child.stderr?.destroy();
}

/** Poll until GET http://127.0.0.1:port/ returns any HTTP response (server is listening and responding) or timeout. */
export async function waitForServer(
  port: number,
  timeoutMs = 15000
): Promise<void> {
  const start = Date.now();
  let attempts = 0;
  let lastErr: string | null = null;
  let lastStatus: number | null = null;
  let lastLogTime = start;
  while (Date.now() - start < timeoutMs) {
    attempts += 1;
    try {
      const res = await fetch(`http://127.0.0.1:${port}/`);
      lastStatus = res.status;
      lastErr = null;
      return;
    } catch (e) {
      lastErr = e instanceof Error ? e.message : String(e);
      lastStatus = null;
    }
    // #region agent log
    if (attempts === 1) {
      _dbg('run-child.ts:waitForServer', 'first attempt result', { port, lastErr, lastStatus }, 'B');
    }
    if (Date.now() - lastLogTime >= 5000) {
      _dbg('run-child.ts:waitForServer', 'still waiting', { port, attempts, lastErr, lastStatus, elapsed: Date.now() - start }, 'C');
      lastLogTime = Date.now();
    }
    // #endregion
    await new Promise((r) => setTimeout(r, 200));
  }
  // #region agent log
  _dbg('run-child.ts:waitForServer', 'timeout', { port, attempts, lastErr, lastStatus, timeoutMs }, 'D');
  // #endregion
  throw new Error(`Server on port ${port} not ready within ${timeoutMs}ms`);
}
