/**
 * PD3.1b, PD3.1c, PD3.1d, PD3.1e — public surface alignment with the docs.
 *
 * These tests guard the subpath exports the docs promise:
 *   - `@softprobe/softprobe-js/hooks` exports hook types
 *   - `@softprobe/softprobe-js/suite` exports `runSuite`, `loadSuite`
 *   - top-level package exports `setLogger`, `VERSION`
 */
import fs from 'fs';
import path from 'path';

import { setLogger, getLogger, buildConsoleLogger, VERSION } from '..';
// Subpath imports — compiling this file at all verifies they resolve.
import * as hooksMod from '../hooks';
import { runSuite, loadSuite } from '../suite';

describe('PD3.1b hooks subpath exports', () => {
  it('re-exports hook type marker (type-only module compiles)', () => {
    // `hooks.ts` is a type-only module; at runtime it exports nothing, so
    // `Object.keys(hooksMod).length` is 0. Importing it at all is the real
    // test — if the module failed to resolve, the whole file would throw.
    expect(Object.keys(hooksMod)).toEqual([]);
  });
});

describe('PD3.1c suite subpath exports', () => {
  const tmpSuitePath = path.join(__dirname, 'fixtures', 'pd31c.suite.yaml');

  beforeAll(() => {
    fs.mkdirSync(path.dirname(tmpSuitePath), { recursive: true });
    fs.writeFileSync(
      tmpSuitePath,
      [
        'name: pd31c-smoke',
        'version: 1',
        'cases:',
        '  - path: cases/a.case.json',
        '  - path: cases/b.case.json',
        '    name: case-b',
        '',
      ].join('\n')
    );
  });
  afterAll(() => {
    try {
      fs.rmSync(tmpSuitePath);
    } catch {
      // ignore
    }
  });

  it('loadSuite parses name / cases from YAML', () => {
    const suite = loadSuite(tmpSuitePath);
    expect(suite.name).toBe('pd31c-smoke');
    expect(suite.version).toBe(1);
    expect(suite.cases.map((c) => c.path)).toEqual(['cases/a.case.json', 'cases/b.case.json']);
    expect(suite.cases[1].name).toBe('case-b');
  });

  it('runSuite is an exported function', () => {
    expect(typeof runSuite).toBe('function');
  });
});

describe('PD3.1d logger + SOFTPROBE_LOG', () => {
  afterEach(() => setLogger(null));

  it('setLogger installs and getLogger retrieves', () => {
    const calls: unknown[][] = [];
    setLogger({ debug: (...args) => calls.push(args) });
    getLogger().debug?.('x', 1);
    expect(calls).toEqual([['x', 1]]);
  });

  it('SOFTPROBE_LOG=debug yields a console logger', () => {
    const prev = process.env.SOFTPROBE_LOG;
    process.env.SOFTPROBE_LOG = 'debug';
    try {
      setLogger(null);
      const log = getLogger();
      expect(typeof log.debug).toBe('function');
      expect(typeof log.warn).toBe('function');
    } finally {
      if (prev === undefined) delete process.env.SOFTPROBE_LOG;
      else process.env.SOFTPROBE_LOG = prev;
    }
  });

  it('buildConsoleLogger(warn) emits only warn+error', () => {
    const log = buildConsoleLogger('warn');
    expect(log.debug).toBeUndefined();
    expect(log.info).toBeUndefined();
    expect(typeof log.warn).toBe('function');
    expect(typeof log.error).toBe('function');
  });
});

describe('PD3.1e VERSION string', () => {
  it('matches package.json#version', () => {
    const pkg = JSON.parse(
      fs.readFileSync(path.join(__dirname, '..', '..', 'package.json'), 'utf8')
    ) as { version: string };
    expect(VERSION).toBe(pkg.version);
  });
});
