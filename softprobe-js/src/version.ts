/**
 * PD3.1e — single source of truth for the SDK version string.
 *
 * The value is kept in lock-step with `package.json#version` at build time.
 * The string is exported so `softprobe doctor` and user-facing telemetry
 * can report "ran against @softprobe/softprobe-js X.Y.Z" without re-reading
 * package.json at runtime.
 */
export const VERSION = '2.0.10';
