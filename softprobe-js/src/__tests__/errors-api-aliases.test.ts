/**
 * PD3.1a — SDK error aliases + SoftprobeError base.
 *
 * The docs ({@link https://softprobe.dev/reference/sdk-typescript#errors}) promise:
 *
 *   import { SoftprobeError, RuntimeError, CaseLookupError, CaseLoadError } from '@softprobe/softprobe-js';
 *
 * with a single `SoftprobeError` base and three documented aliases. These
 * tests encode that contract so the public surface cannot drift from the
 * reference without failing CI.
 */
import {
  SoftprobeError,
  RuntimeError,
  CaseLookupError,
  CaseLoadError,
  // Back-compat aliases kept for existing consumers.
  SoftprobeRuntimeError,
  SoftprobeCaseLoadError,
  SoftprobeCaseLookupAmbiguityError,
} from '..';

describe('PD3.1a SDK error aliases', () => {
  it('exposes a SoftprobeError base class that extends Error', () => {
    const err = new SoftprobeError('boom');
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe('SoftprobeError');
    expect(err.message).toBe('boom');
  });

  it('RuntimeError extends SoftprobeError and carries status/body/url', () => {
    const err = new RuntimeError(500, 'internal', 'http://rt/v1/sessions');
    expect(err).toBeInstanceOf(SoftprobeError);
    expect(err).toBeInstanceOf(Error);
    expect(err.status).toBe(500);
    expect(err.body).toBe('internal');
    expect(err.url).toBe('http://rt/v1/sessions');
  });

  it('RuntimeError is the same constructor as the legacy SoftprobeRuntimeError', () => {
    // Legacy consumers still import SoftprobeRuntimeError; keeping them
    // identity-equal means `instanceof` checks on either class succeed for
    // every runtime error the SDK raises.
    expect(RuntimeError).toBe(SoftprobeRuntimeError);
  });

  it('CaseLookupError extends SoftprobeError and carries matches[]', () => {
    const matches = [{ spanId: 'a' }, { spanId: 'b' }] as const;
    const err = new CaseLookupError('ambiguous', matches as unknown as unknown[]);
    expect(err).toBeInstanceOf(SoftprobeError);
    expect(err.matches).toEqual(matches);
  });

  it('CaseLookupError aliases the legacy ambiguity class', () => {
    expect(CaseLookupError).toBe(SoftprobeCaseLookupAmbiguityError);
  });

  it('CaseLoadError extends SoftprobeError and carries path + cause', () => {
    const cause = new Error('ENOENT');
    const err = new CaseLoadError('failed to load case', '/tmp/x.case.json', cause);
    expect(err).toBeInstanceOf(SoftprobeError);
    expect(err.path).toBe('/tmp/x.case.json');
    expect(err.cause).toBe(cause);
  });

  it('CaseLoadError aliases the legacy class', () => {
    expect(CaseLoadError).toBe(SoftprobeCaseLoadError);
  });
});
