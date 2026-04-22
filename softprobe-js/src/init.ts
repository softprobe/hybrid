/**
 * Boot entry: softprobe/init.
 * Runtime/session-aligned boot: does not accept mode/data-dir env overrides.
 *
 * For Express/Fastify auto-patching, import `@softprobe/softprobe-js/legacy` after this
 * module and call `applyLegacyFrameworkPatches()` (PD6.5f).
 */

const { SoftprobeContext } = require('./context');

if (process.env.SOFTPROBE_MODE !== undefined || process.env.SOFTPROBE_DATA_DIR !== undefined) {
  throw new Error(
    'softprobe/init does not support SOFTPROBE_MODE or SOFTPROBE_DATA_DIR. ' +
      'Use runtime sessions (Softprobe/SoftprobeSession) or explicit SoftprobeContext.initGlobal in tests.'
  );
}

SoftprobeContext.initGlobal({
  mode: 'PASSTHROUGH',
  cassetteDirectory: undefined,
  storage: undefined,
  strictReplay: process.env.SOFTPROBE_STRICT_REPLAY === '1',
  strictComparison: process.env.SOFTPROBE_STRICT_COMPARISON === '1',
});

const { applyAutoInstrumentationMutator } = require('./bootstrap/otel/mutator');
require('./instrumentations/postgres');
const { setupRedisReplay } = require('./instrumentations/redis');
const { setupHttpReplayInterceptor } = require('./instrumentations/fetch');

applyAutoInstrumentationMutator();
setupRedisReplay();
setupHttpReplayInterceptor();
