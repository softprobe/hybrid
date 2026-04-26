# CI integration

Softprobe runs unchanged in any CI that can run Node / Python / Java / Go and
reach `https://runtime.softprobe.dev`. This page shows copy-pasteable workflows
for the major CI systems.

## Prerequisites

Every CI example follows the same pattern:

1. **Set `SOFTPROBE_API_TOKEN`** from the CI secret store.
2. **Run `softprobe doctor`** against the hosted runtime.
3. **Start your app and proxy** if the tests exercise real HTTP traffic.
4. **Run your tests** against the hosted runtime.
5. **Upload artifacts** (JUnit XML, HTML report, captured case files).

## GitHub Actions

### Replay tests via `suite run`

```yaml
# .github/workflows/replay.yml
name: Replay suite

on:
  pull_request:
  push:
    branches: [main]

jobs:
  replay:
    runs-on: [self-hosted, Linux]
    steps:
      - uses: actions/checkout@v4

      - name: Install the CLI
        run: |
          curl -fsSL https://softprobe.dev/install/cli.sh | sh
          echo "$HOME/.local/bin" >> $GITHUB_PATH

      - name: Preflight
        env:
          SOFTPROBE_API_TOKEN: ${{ secrets.SOFTPROBE_API_TOKEN }}
        run: softprobe doctor

      - name: Start sample app and proxy (your project's docker-compose)
        run: docker compose up -d --wait app proxy upstream

      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm

      - name: Install hook dependencies
        run: npm ci --prefix hooks

      - name: Run the suite
        env:
          SOFTPROBE_API_TOKEN: ${{ secrets.SOFTPROBE_API_TOKEN }}
          APP_URL: http://127.0.0.1:8082
          TEST_CARD: ${{ secrets.TEST_CARD }}
        run: |
          softprobe suite run suites/checkout.suite.yaml \
            --parallel 32 \
            --hooks hooks/*.ts \
            --junit out/junit.xml \
            --report out/report.html

      - name: Upload report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: softprobe-report
          path: out/
          retention-days: 30

      - name: Publish JUnit
        if: always()
        uses: dorny/test-reporter@v1
        with:
          name: Softprobe replay
          path: out/junit.xml
          reporter: jest-junit
```

### Replay tests via Jest

If you prefer running the same suite from Jest inside CI:

```yaml
# .github/workflows/jest-replay.yml
name: Jest replay

on: [push, pull_request]

jobs:
  test:
    runs-on: [self-hosted, Linux]
    steps:
      - uses: actions/checkout@v4

      - name: Boot app and proxy
        run: docker compose -f e2e/docker-compose.yaml up -d --wait

      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm

      - run: npm ci
      - run: npm test -- --ci --reporters=default --reporters=jest-junit
        env:
          SOFTPROBE_API_TOKEN: ${{ secrets.SOFTPROBE_API_TOKEN }}
          APP_URL: http://127.0.0.1:8082

      - uses: actions/upload-artifact@v4
        if: failure()
        with:
          name: jest-junit
          path: junit.xml

      - run: docker compose -f e2e/docker-compose.yaml logs > logs.txt
        if: failure()
      - uses: actions/upload-artifact@v4
        if: failure()
        with:
          name: compose-logs
          path: logs.txt
```

### Nightly captures from staging

Captures are usually run nightly against a staging environment; commits from the bot land on a branch so reviewers can approve.

```yaml
# .github/workflows/nightly-capture.yml
name: Nightly capture

on:
  schedule:
    - cron: '0 4 * * *'
  workflow_dispatch:

jobs:
  capture:
    runs-on: [self-hosted, Linux]
    environment: staging-capture
    permissions:
      contents: write
      pull-requests: write
    steps:
      - uses: actions/checkout@v4

      - name: Install the CLI
        run: curl -fsSL https://softprobe.dev/install/cli.sh | sh

      - name: Capture one session per scenario
        env:
          SOFTPROBE_API_TOKEN: ${{ secrets.SOFTPROBE_API_TOKEN }}
          STAGING_APP_URL: ${{ secrets.STAGING_APP_URL }}
        run: ./scripts/capture-scenarios.sh  # your own

      - name: Redact sensitive fields
        run: softprobe scrub cases/**/*.case.json

      - name: Open a pull request
        uses: peter-evans/create-pull-request@v5
        with:
          branch: capture/nightly-${{ github.run_id }}
          commit-message: "capture: nightly refresh"
          title: "Nightly capture refresh"
          body: "Automated capture refresh. Please review the diff before merging."
```

## GitLab CI

```yaml
# .gitlab-ci.yml
stages: [build, test]

variables:
  SOFTPROBE_RUNTIME_URL: https://runtime.softprobe.dev
  # Configure SOFTPROBE_API_TOKEN as a masked CI/CD variable.

replay:
  stage: test
  image: node:20
  before_script:
    - curl -fsSL https://softprobe.dev/install/cli.sh | sh
    - softprobe doctor
    - npm ci
  script:
    - softprobe suite run suites/checkout.suite.yaml
        --parallel 32
        --hooks hooks/*.ts
        --junit out/junit.xml
  artifacts:
    when: always
    paths: [out/]
    reports:
      junit: out/junit.xml
```

## CircleCI

```yaml
# .circleci/config.yml
version: 2.1

jobs:
  replay:
    docker:
      - image: cimg/node:20.0
    environment:
      SOFTPROBE_RUNTIME_URL: https://runtime.softprobe.dev
      # Set SOFTPROBE_API_TOKEN in the CircleCI project or context.
    steps:
      - checkout
      - run:
          name: Install CLI
          command: curl -fsSL https://softprobe.dev/install/cli.sh | sh
      - run: softprobe doctor
      - run: npm ci
      - run:
          name: Run suite
          command: |
            softprobe suite run suites/checkout.suite.yaml \
              --parallel 32 \
              --hooks hooks/*.ts \
              --junit out/junit.xml
      - store_test_results:
          path: out/junit.xml
      - store_artifacts:
          path: out/

workflows:
  version: 2
  test:
    jobs:
      - replay
```

## Jenkins (declarative pipeline)

```groovy
pipeline {
  agent any
  environment {
    SOFTPROBE_RUNTIME_URL = 'https://runtime.softprobe.dev'
    SOFTPROBE_API_TOKEN = credentials('softprobe-api-token')
  }

  stages {
    stage('Preflight') {
      steps {
        sh 'softprobe doctor'
      }
    }
    stage('Suite') {
      steps {
        sh 'npm ci'
        sh 'softprobe suite run suites/checkout.suite.yaml --parallel 32 --junit out/junit.xml'
      }
      post {
        always { junit 'out/junit.xml' }
      }
    }
  }
}
```

## Caching the case files

Case files are essentially test data. If your cases directory grows large (100+ MB), cache it across CI runs:

```yaml
# GitHub Actions example
- name: Cache case files
  uses: actions/cache@v4
  with:
    path: cases/
    key: cases-${{ hashFiles('cases/**/*.case.json') }}
```

If cases are committed to git, this is unnecessary. If they are produced nightly to object storage, download them in a pre-step.

## Secrets and hooks

Hooks that need secrets (test tokens, HMAC keys) should read from environment variables, not config files:

```ts
export const sign: MockResponseHook = ({ capturedResponse, env }) => {
  if (!env.HMAC_KEY) throw new Error('HMAC_KEY required');
  // ...
};
```

Inject them via the CI's secret store (<code v-pre>${{ secrets.HMAC_KEY }}</code> in GitHub Actions, masked variables in GitLab, etc.).

## Speed tips

**Keep the runtime URL at the hosted default.** Set `SOFTPROBE_API_TOKEN` in the
CI secret store and leave `SOFTPROBE_RUNTIME_URL` unset unless Softprobe support
provides a different hosted endpoint.

**Install the CLI from the binary, not npm.** The binary installer is a single 8 MB download; the npm wrapper adds Node startup overhead.

**Run suites in parallel across cases, not jobs.** One CI job running `--parallel 32` is faster than 32 jobs each running one case.

**Collect app/proxy logs on failure only.** `docker compose logs` on every run doubles your CI artifact cost.

## Next

- [Troubleshooting](/guides/troubleshooting) — what to do when a CI run fails unexpectedly.
- [Run a suite at scale](/guides/run-a-suite-at-scale) — if you haven't yet, start with the hosted-runtime flow.
- [Deployment: Kubernetes](/deployment/kubernetes) — for running the proxy in a persistent CI environment.
