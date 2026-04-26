# Softprobe Documentation Map

**Normative workflow summary:** load `https://docs.softprobe.dev/ai-context.md` first (or `docs-site/public/ai-context.md` in the monorepo). Use this map to pick the smallest **additional** official page for the user's task.

Prefer the online URL. If working inside the Softprobe monorepo, the local file listed beside each URL has the same source content.

## Start Here

| Need | Online docs | Local source |
|---|---|---|
| End-to-end first run | https://docs.softprobe.dev/quickstart | `docs-site/quickstart.md` |
| Install CLI, SDK, runtime, or proxy | https://docs.softprobe.dev/installation | `docs-site/installation.md` |
| Download published artifacts | https://docs.softprobe.dev/downloads | `docs-site/downloads.md` |
| Core terms | https://docs.softprobe.dev/glossary | `docs-site/glossary.md` |

## Writing Tests

| Need | Online docs | Local source |
|---|---|---|
| Write a Jest replay test | https://docs.softprobe.dev/guides/replay-in-jest | `docs-site/guides/replay-in-jest.md` |
| Write a pytest replay test | https://docs.softprobe.dev/guides/replay-in-pytest | `docs-site/guides/replay-in-pytest.md` |
| Write a JUnit replay test | https://docs.softprobe.dev/guides/replay-in-junit | `docs-site/guides/replay-in-junit.md` |
| Write a Go replay test | https://docs.softprobe.dev/guides/replay-in-go | `docs-site/guides/replay-in-go.md` |
| Run many cases as a suite | https://docs.softprobe.dev/guides/run-a-suite-at-scale | `docs-site/guides/run-a-suite-at-scale.md` |
| Write reusable hooks | https://docs.softprobe.dev/guides/write-a-hook | `docs-site/guides/write-a-hook.md` |

## Capturing And Replaying Cases

| Need | Online docs | Local source |
|---|---|---|
| Capture a first case file | https://docs.softprobe.dev/guides/capture-your-first-session | `docs-site/guides/capture-your-first-session.md` |
| Understand sessions and case files | https://docs.softprobe.dev/concepts/sessions-and-cases | `docs-site/concepts/sessions-and-cases.md` |
| Understand capture and replay | https://docs.softprobe.dev/concepts/capture-and-replay | `docs-site/concepts/capture-and-replay.md` |
| Mock one external dependency | https://docs.softprobe.dev/guides/mock-external-dependency | `docs-site/guides/mock-external-dependency.md` |
| Ship rules with a case | https://docs.softprobe.dev/guides/ship-rules-with-a-case | `docs-site/guides/ship-rules-with-a-case.md` |

## API And Schema Reference

| Need | Online docs | Local source |
|---|---|---|
| CLI flags, exit codes, JSON output | https://docs.softprobe.dev/reference/cli | `docs-site/reference/cli.md` |
| TypeScript SDK API | https://docs.softprobe.dev/reference/sdk-typescript | `docs-site/reference/sdk-typescript.md` |
| Python SDK API | https://docs.softprobe.dev/reference/sdk-python | `docs-site/reference/sdk-python.md` |
| Java SDK API | https://docs.softprobe.dev/reference/sdk-java | `docs-site/reference/sdk-java.md` |
| Go SDK API | https://docs.softprobe.dev/reference/sdk-go | `docs-site/reference/sdk-go.md` |
| Session header rules | https://docs.softprobe.dev/reference/session-headers | `docs-site/reference/session-headers.md` |
| Suite YAML | https://docs.softprobe.dev/reference/suite-yaml | `docs-site/reference/suite-yaml.md` |
| Case file schema | https://docs.softprobe.dev/reference/case-schema | `docs-site/reference/case-schema.md` |
| Rule schema | https://docs.softprobe.dev/reference/rule-schema | `docs-site/reference/rule-schema.md` |
| HTTP control API contract | https://docs.softprobe.dev/reference/http-control-api | `docs-site/reference/http-control-api.md` |
| Proxy OTLP API contract | https://docs.softprobe.dev/reference/proxy-otel-api | `docs-site/reference/proxy-otel-api.md` |

## Debugging And Deployment

| Need | Online docs | Local source |
|---|---|---|
| First troubleshooting pass | https://docs.softprobe.dev/guides/troubleshooting | `docs-site/guides/troubleshooting.md` |
| Debug strict-policy misses | https://docs.softprobe.dev/guides/debug-strict-miss | `docs-site/guides/debug-strict-miss.md` |
| CI integration | https://docs.softprobe.dev/guides/ci-integration | `docs-site/guides/ci-integration.md` |
| Choose proxy vs language instrumentation | https://docs.softprobe.dev/guides/proxy-vs-language-instrumentation | `docs-site/guides/proxy-vs-language-instrumentation.md` |
| Local Docker Compose deployment | https://docs.softprobe.dev/deployment/local | `docs-site/deployment/local.md` |
| Kubernetes deployment | https://docs.softprobe.dev/deployment/kubernetes | `docs-site/deployment/kubernetes.md` |
| Hosted runtime | https://docs.softprobe.dev/deployment/hosted | `docs-site/deployment/hosted.md` |

## Lookup Rules

- For code examples, use the language-specific guide first, then the SDK reference.
- For CLI behavior, use the CLI reference rather than guessing flags.
- For replay misses, use troubleshooting plus session headers.
- For generated Jest helpers, use the generator guide rather than the hand-written Jest guide.
- For raw protocol work, use HTTP control API only for CLI/SDK/test control-plane tasks and Proxy OTLP API only for proxy/runtime integration tasks.
