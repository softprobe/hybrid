/**
 * Boot tests for softprobe/init — runtime/session defaults and env guardrails.
 */

describe('softprobe/init boot', () => {
  const envBackup: Record<string, string | undefined> = {};

  function saveEnv(key: string): void {
    envBackup[key] = process.env[key];
  }

  function restoreEnv(key: string): void {
    if (envBackup[key] === undefined) delete process.env[key];
    else process.env[key] = envBackup[key];
  }

  beforeEach(() => {
    saveEnv('SOFTPROBE_MODE');
    saveEnv('SOFTPROBE_DATA_DIR');
    saveEnv('SOFTPROBE_STRICT_REPLAY');
    saveEnv('SOFTPROBE_STRICT_COMPARISON');
  });

  afterEach(() => {
    restoreEnv('SOFTPROBE_MODE');
    restoreEnv('SOFTPROBE_DATA_DIR');
    restoreEnv('SOFTPROBE_STRICT_REPLAY');
    restoreEnv('SOFTPROBE_STRICT_COMPARISON');
  });

  it('throws when SOFTPROBE_MODE or SOFTPROBE_DATA_DIR is set', () => {
    const initGlobal = jest.fn();
    jest.isolateModules(() => {
      process.env.SOFTPROBE_MODE = 'CAPTURE';
      process.env.SOFTPROBE_DATA_DIR = '/tmp/capture-cases';
      jest.doMock('../context', () => ({
        SoftprobeContext: { initGlobal },
      }));
      jest.doMock('../bootstrap/otel/mutator', () => ({ applyAutoInstrumentationMutator: jest.fn() }));
      jest.doMock('../bootstrap/otel/framework-mutator', () => ({ applyFrameworkMutators: jest.fn() }));
      jest.doMock('../instrumentations/fetch', () => ({ setupHttpReplayInterceptor: jest.fn() }));
      jest.doMock('../instrumentations/redis', () => ({ setupRedisReplay: jest.fn() }));
      jest.doMock('../instrumentations/postgres', () => ({}));
      expect(() => require('../init')).toThrow(
        'softprobe/init does not support SOFTPROBE_MODE or SOFTPROBE_DATA_DIR.'
      );
      expect(initGlobal).not.toHaveBeenCalled();
    });
  });

  it('always defaults to PASSTHROUGH with no cassetteDirectory when env vars are not set', () => {
    const initGlobal = jest.fn();
    jest.isolateModules(() => {
      delete process.env.SOFTPROBE_MODE;
      delete process.env.SOFTPROBE_DATA_DIR;
      jest.doMock('../context', () => ({
        SoftprobeContext: { initGlobal },
      }));
      jest.doMock('../bootstrap/otel/mutator', () => ({ applyAutoInstrumentationMutator: jest.fn() }));
      jest.doMock('../instrumentations/fetch', () => ({ setupHttpReplayInterceptor: jest.fn() }));
      jest.doMock('../instrumentations/redis', () => ({ setupRedisReplay: jest.fn() }));
      jest.doMock('../instrumentations/postgres', () => ({}));
      require('../init');
      expect(initGlobal).toHaveBeenCalledWith(
        expect.objectContaining({
          mode: 'PASSTHROUGH',
          cassetteDirectory: undefined,
        })
      );
    });
  });

  it('does not invoke applyFrameworkMutators from init (PD6.5f)', () => {
    const applyFrameworkMutators = jest.fn();
    jest.isolateModules(() => {
      delete process.env.SOFTPROBE_MODE;
      jest.doMock('../context', () => ({
        SoftprobeContext: { initGlobal: jest.fn() },
      }));
      jest.doMock('../bootstrap/otel/mutator', () => ({ applyAutoInstrumentationMutator: jest.fn() }));
      jest.doMock('../bootstrap/otel/framework-mutator', () => ({ applyFrameworkMutators }));
      jest.doMock('../instrumentations/fetch', () => ({ setupHttpReplayInterceptor: jest.fn() }));
      jest.doMock('../instrumentations/redis', () => ({ setupRedisReplay: jest.fn() }));
      jest.doMock('../instrumentations/postgres', () => ({}));
      require('../init');
      expect(applyFrameworkMutators).not.toHaveBeenCalled();
    });
  });
});
