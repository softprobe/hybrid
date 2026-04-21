/**
 * PD3.1d — public logger shim.
 *
 * The SDK is silent by default. Users can call `setLogger` to plug in any
 * logger, or set the `SOFTPROBE_LOG` env var to `debug` / `warn` / `info`
 * to get console output without modifying code. Library code calls
 * `getLogger()` lazily so setting the logger after import still takes
 * effect on subsequent log calls.
 *
 * Reference: docs-site/reference/sdk-typescript.md#logging.
 */

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface SoftprobeLogger {
  debug?(...args: unknown[]): void;
  info?(...args: unknown[]): void;
  warn?(...args: unknown[]): void;
  error?(...args: unknown[]): void;
}

const silentLogger: SoftprobeLogger = {};
let currentLogger: SoftprobeLogger | null = null;

/** Install (or reset) the global SDK logger. Pass `null` to silence. */
export function setLogger(logger: SoftprobeLogger | null): void {
  currentLogger = logger;
}

/** Return the active logger — user-installed, env-derived, or silent. */
export function getLogger(): SoftprobeLogger {
  if (currentLogger) return currentLogger;
  const fromEnv = loggerFromEnv();
  return fromEnv ?? silentLogger;
}

function loggerFromEnv(): SoftprobeLogger | null {
  const level = (process.env.SOFTPROBE_LOG ?? '').trim().toLowerCase() as LogLevel | '';
  if (!level) return null;
  return buildConsoleLogger(level);
}

/**
 * Build a logger that routes to `console.*` for levels at or above the
 * requested threshold. `debug=lowest`, `error=highest`. A level of `info`
 * also surfaces warn+error; a level of `error` surfaces only `.error`.
 */
export function buildConsoleLogger(threshold: LogLevel): SoftprobeLogger {
  const order: LogLevel[] = ['debug', 'info', 'warn', 'error'];
  const minIdx = Math.max(0, order.indexOf(threshold));
  const logger: SoftprobeLogger = {};
  if (minIdx <= 0) logger.debug = (...args) => console.debug('[softprobe]', ...args);
  if (minIdx <= 1) logger.info = (...args) => console.info('[softprobe]', ...args);
  if (minIdx <= 2) logger.warn = (...args) => console.warn('[softprobe]', ...args);
  if (minIdx <= 3) logger.error = (...args) => console.error('[softprobe]', ...args);
  return logger;
}
