// Softprobe suite-run hook sidecar.
//
// Spawned by `softprobe suite run` to invoke user-provided TypeScript /
// JavaScript hook files. Protocol: newline-delimited JSON on stdin /
// stdout.
//
// Request shape (stdin):
//   {"id":"42","hook":"rewriteDep","kind":"mock_response","payload":{...}}
//
// Response shape (stdout):
//   {"id":"42","result":{...}}      // success
//   {"id":"42","error":"..."}       // failure (exception or missing hook)
//
// One extra line is emitted right after hook files finish loading so the
// Go side can block on startup until every hook file parses:
//   {"ready":true}                  // or {"ready":true,"error":"..."}
//
// See softprobe-runtime/cmd/softprobe/suite_hooks.go for the Go side and
// docs-site/guides/run-a-suite-at-scale.md for the user-facing contract.

import { createInterface } from 'node:readline';
import { pathToFileURL } from 'node:url';
import { resolve } from 'node:path';

const registry = new Map();

async function loadHookFiles(files) {
  for (const file of files) {
    const abs = resolve(process.cwd(), file);
    const mod = await import(pathToFileURL(abs).href);
    for (const [name, value] of Object.entries(mod)) {
      if (typeof value === 'function') {
        registry.set(name, value);
      }
    }
  }
}

function reply(obj) {
  process.stdout.write(JSON.stringify(obj) + '\n');
}

async function invoke(req) {
  const hook = registry.get(req.hook);
  if (!hook) {
    reply({ id: req.id, error: `hook ${JSON.stringify(req.hook)} not found in any --hooks file` });
    return;
  }
  try {
    const result = await Promise.resolve(hook(req.payload));
    // Undefined is meaningful: caller expects `null` on the wire, not
    // a missing field. JSON.stringify({result: undefined}) drops the
    // key, so we normalize to null.
    reply({ id: req.id, result: result === undefined ? null : result });
  } catch (err) {
    const message = err && err.message ? err.message : String(err);
    reply({ id: req.id, error: message });
  }
}

async function main() {
  const files = process.argv.slice(2);
  try {
    await loadHookFiles(files);
    reply({ ready: true });
  } catch (err) {
    const message = err && err.stack ? err.stack : String(err);
    reply({ ready: true, error: message });
    // Keep the process alive long enough for the parent to read our
    // ready error, then exit. Exiting immediately can race the reader.
    setTimeout(() => process.exit(1), 100);
    return;
  }

  const rl = createInterface({ input: process.stdin });
  rl.on('line', (line) => {
    if (!line.trim()) return;
    let req;
    try {
      req = JSON.parse(line);
    } catch (err) {
      reply({ error: `invalid json from parent: ${err.message}` });
      return;
    }
    // Fire and forget — invocations can run in parallel, reply ordering
    // doesn't matter because each response carries its id.
    void invoke(req);
  });
  rl.on('close', () => {
    process.exit(0);
  });
}

void main();
