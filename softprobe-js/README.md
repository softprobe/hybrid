# @softprobe/softprobe-js

TypeScript SDK for the **Softprobe hybrid** platform.

The canonical TypeScript flow is:

1. create a replay session on `softprobe-runtime`
2. load a `*.case.json` file
3. use `findInCase(...)` to select a captured span in memory
4. register an explicit mock with `mockOutbound(...)`
5. drive the SUT through the proxy with `x-softprobe-session-id`

This package is the TypeScript SDK for that flow. It is **not** the canonical
home of the product CLI or the product runtime.

## Install

```bash
npm install --save-dev @softprobe/softprobe-js
```

## Minimal replay example

```ts
import path from "path";
import { Softprobe } from "@softprobe/softprobe-js";

const softprobe = new Softprobe({
  baseUrl: process.env.SOFTPROBE_RUNTIME_URL ?? "http://127.0.0.1:8080",
});

const session = await softprobe.startSession({ mode: "replay" });
await session.loadCaseFromFile(
  path.resolve("spec/examples/cases/fragment-happy-path.case.json")
);

const hit = session.findInCase({
  direction: "outbound",
  method: "GET",
  path: "/fragment",
});

await session.mockOutbound({
  direction: "outbound",
  method: "GET",
  path: "/fragment",
  response: hit.response,
});

// Then send a request to the SUT through the proxy with:
// { "x-softprobe-session-id": session.id }

await session.close();
```

## Current public surface

The package currently exports the runtime client and the ergonomic session APIs
used by the hybrid flow:

- `Softprobe`
- `SoftprobeSession`
- `SoftprobeRuntimeClient`
- `findInCase(...)`
- `mockOutbound(...)`
- `clearRules()`
- `close()`

See:

- [`docs/design.md`](../docs/design.md)
- [`docs-site/reference/sdk-typescript.md`](../docs-site/reference/sdk-typescript.md)
- [`e2e/jest-replay/`](../e2e/jest-replay/)

## Canonical CLI

The canonical product CLI lives in [`softprobe-runtime/`](../softprobe-runtime/),
not in this package. If this package exposes older Node-oriented CLI entrypoints,
they are compatibility surfaces rather than the preferred product path.

## Legacy note

This repo still contains older Node-specific instrumentation, framework-patching,
and NDJSON cassette surfaces under migration. They remain for compatibility and
test coverage, but they are **not** the canonical Softprobe product direction.
The source of truth is the proxy-first hybrid design in [`docs/design.md`](../docs/design.md).

## Release note

This package is configured (`publishConfig` in `package.json`) to publish to
npm as `@softprobe/softprobe-js`, but **this monorepo does not currently run
an automated publish workflow**. Historical releases under that name exist on
npm, but new commits in this repo are not automatically released. Consume the
package from source when you need changes that are not yet on npm:

```bash
cd softprobe-js
npm install
npm run build
# then reference ../softprobe-js from your project (file: or workspaces).
```

The in-repo harnesses under [`e2e/jest-replay/`](../e2e/jest-replay/) import
this package by relative path rather than from npm.

## License

Apache-2.0. See [`LICENSE`](./LICENSE) and the monorepo [`LICENSING.md`](../LICENSING.md) for the full dual-license map (server components are under the Softprobe Source License 1.0).
