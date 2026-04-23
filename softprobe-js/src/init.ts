/**
 * Boot entry: softprobe/init.
 * Runtime/session-aligned boot: does not accept mode/data-dir env overrides.
 *
 * For Express/Fastify auto-patching, import `@softprobe/softprobe-js/legacy` after this
 * module and call `applyLegacyFrameworkPatches()` (PD6.5f).
 */

const otelApi = require('@opentelemetry/api');
try {
  const { AsyncHooksContextManager } = require('@opentelemetry/context-async-hooks');
  const mgr = new AsyncHooksContextManager();
  mgr.enable();
  otelApi.context.setGlobalContextManager(mgr);
} catch { /* context-async-hooks not installed; context.with() won't propagate across await */ }

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

const { setupHttpReplayInterceptor } = require('./instrumentations/fetch');

// postgres, redis, and auto-instrumentation mutator are optional — only active when
// the corresponding packages are installed. HTTP fetch interception works without them.
try { require('./instrumentations/postgres'); } catch { /* pg not installed */ }
try {
  const { setupRedisReplay } = require('./instrumentations/redis');
  setupRedisReplay();
} catch { /* @redis/client not installed */ }
try {
  const { applyAutoInstrumentationMutator } = require('./bootstrap/otel/mutator');
  applyAutoInstrumentationMutator();
} catch { /* @opentelemetry/auto-instrumentations-node not installed */ }

setupHttpReplayInterceptor();
