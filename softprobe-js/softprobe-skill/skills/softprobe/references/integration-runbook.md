# Softprobe Integration Runbook (Canonical)

This runbook is the default integration path for external repositories.

## 0) Install Packages (Required)

Install OpenTelemetry + Softprobe dependencies:

```bash
npm i @opentelemetry/auto-instrumentations-node @opentelemetry/sdk-node @softprobe/softprobe-js@latest
```

Example `package.json` versions:

```json
{
  "dependencies": {
    "@opentelemetry/auto-instrumentations-node": "^0.71.0",
    "@opentelemetry/sdk-node": "^0.213.0",
    "@softprobe/softprobe-js": "latest"
  }
}
```

To run the `softprobe` CLI you can either:

- **Install globally**: `npm i -g @softprobe/softprobe-js@latest` then use `softprobe ...`
- **Install locally** (recommended for repos): keep it in `dependencies` and use `npx softprobe ...`

## 1) Bootstrap Pattern (Required)

Use a dedicated bootstrap file loaded before server startup:

```js
// instrumentation.js
require('@softprobe/softprobe-js/init');

const { NodeSDK } = require('@opentelemetry/sdk-node');
const { getNodeAutoInstrumentations } = require('@opentelemetry/auto-instrumentations-node');

const sdk = new NodeSDK({
  instrumentations: [getNodeAutoInstrumentations()],
});

sdk.start();
```

Add a `start` script to `package.json` and use it to launch the server:

```json
"scripts": {
  "start": "node -r ./instrumentation.js server.js"
}
```

```bash
npm start
```

## 2) Config Pattern (Required)

Create `.softprobe/config.yml`:

```yaml
mode: PASSTHROUGH
cassetteDirectory: ./cassettes
```

## 3) Preflight Checks (Required)

Run these checks before editing app code:

1. Verify bootstrap exists and loads Softprobe init:
   - `rg -n "@softprobe/softprobe-js/init|softprobe/init" instrumentation.*`
2. Verify bootstrap initializes OTel NodeSDK:
   - `rg -n "NodeSDK|getNodeAutoInstrumentations|sdk.start\(" instrumentation.*`
3. Verify start command loads bootstrap:
   - `rg -n "node -r .*instrumentation" package.json`
4. Verify cassette directory configured:
   - `rg -n "cassetteDirectory" .softprobe/config.yml`
5. Verify no deep import of internal dist files:
   - `rg -n "@softprobe/softprobe-js/dist/" .`

If check 5 returns matches, remove the deep import approach and switch to this runbook.

## 4) Capture Command

```bash
TRACE_ID=11111111111111111111111111111111
softprobe capture "http://127.0.0.1:3000/health" --trace-id "$TRACE_ID"
```

If Softprobe is installed locally in the project:

```bash
TRACE_ID=11111111111111111111111111111111
npx softprobe capture "http://127.0.0.1:3000/health" --trace-id "$TRACE_ID"
```

Verify cassette output:

```bash
ls -l "cassettes/$TRACE_ID.ndjson"
```

## 5) Replay Diff Command

```bash
softprobe diff "cassettes/$TRACE_ID.ndjson" "http://127.0.0.1:3000"
```

If Softprobe is installed locally in the project:

```bash
npx softprobe diff "cassettes/$TRACE_ID.ndjson" "http://127.0.0.1:3000"
```

## 6) Forbidden Fix Pattern

Do not "fix" missing exports by importing package internals:

- forbidden: `require('@softprobe/softprobe-js/dist/instrumentations/express')`
- forbidden: `require(path.join(__dirname, 'node_modules/.../dist/...'))`

If a public API is missing, report it and ask the user whether to add/export that API.
