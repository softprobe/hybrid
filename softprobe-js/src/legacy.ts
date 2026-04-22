/**
 * Opt-in legacy hooks removed from default `softprobe/init` (PD6.5f).
 * Import after `softprobe/init` when you still need require-based Express/Fastify patches.
 */

import { applyFrameworkMutators } from './bootstrap/otel/framework-mutator';

export function applyLegacyFrameworkPatches(): void {
  applyFrameworkMutators();
}
