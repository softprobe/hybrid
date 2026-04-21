# CI integration

Softprobe runs unchanged in any CI that can spin up a Docker container and run Node / Python / Java / Go. This page shows copy-pasteable workflows for the major CI systems.

## Prerequisites

Every CI example follows the same pattern:

1. **Start the Softprobe runtime** as a sidecar service.
2. **(Optional) start a capture** — most CI runs are replay-only; capture usually runs in a staging-cron job that commits updated cases.
3. **Run your tests** against the runtime.
4. **Upload artifacts** (JUnit XML, HTML report, captured case files).

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
    runs-on: ubuntu-latest
    services:
      softprobe-runtime:
        image: ghcr.io/softprobe/softprobe-runtime:v0.5
        ports:
          - 8080:8080
        options: >-
          --health-cmd "wget -qO- http://127.0.0.1:8080/health || exit 1"
          --health-interval 5s --health-retries 10

    steps:
      - uses: actions/checkout@v4

      - name: Install the CLI
        run: |
          curl -fsSL https://softprobe.dev/install/cli.sh | sh
          echo "$HOME/.local/bin" >> $GITHUB_PATH

      - name: Preflight
        run: softprobe doctor --runtime-url http://127.0.0.1:8080

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
          SOFTPROBE_RUNTIME_URL: http://127.0.0.1:8080
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
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Boot Softprobe stack
        run: docker compose -f e2e/docker-compose.yaml up -d --wait

      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm

      - run: npm ci
      - run: npm test -- --ci --reporters=default --reporters=jest-junit
        env:
          SOFTPROBE_RUNTIME_URL: http://127.0.0.1:8080
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
    runs-on: ubuntu-latest
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
          SOFTPROBE_RUNTIME_URL: ${{ secrets.STAGING_RUNTIME_URL }}
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
  SOFTPROBE_RUNTIME_URL: http://softprobe-runtime:8080

replay:
  stage: test
  image: node:20
  services:
    - name: ghcr.io/softprobe/softprobe-runtime:v0.5
      alias: softprobe-runtime
  before_script:
    - curl -fsSL https://softprobe.dev/install/cli.sh | sh
    - softprobe doctor --runtime-url $SOFTPROBE_RUNTIME_URL
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
      - image: ghcr.io/softprobe/softprobe-runtime:v0.5
        name: softprobe-runtime
    environment:
      SOFTPROBE_RUNTIME_URL: http://softprobe-runtime:8080
    steps:
      - checkout
      - run:
          name: Install CLI
          command: curl -fsSL https://softprobe.dev/install/cli.sh | sh
      - run: softprobe doctor --runtime-url $SOFTPROBE_RUNTIME_URL
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
    SOFTPROBE_RUNTIME_URL = 'http://localhost:8080'
  }

  stages {
    stage('Start runtime') {
      steps {
        sh 'docker run -d --name softprobe-runtime -p 8080:8080 ghcr.io/softprobe/softprobe-runtime:v0.5'
        sh 'until curl -sf http://localhost:8080/health; do sleep 1; done'
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
  post {
    always { sh 'docker rm -f softprobe-runtime || true' }
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

**Pin the runtime image tag.** `:v0.5` pulls once and stays cached; `:latest` re-pulls on every run.

**Install the CLI from the binary, not npm.** The binary installer is a single 8 MB download; the npm wrapper adds Node startup overhead.

**Run suites in parallel across cases, not jobs.** One CI job running `--parallel 32` is faster than 32 jobs each running one case.

**Collect logs on failure only.** `docker compose logs` on every run doubles your CI artifact cost.

## Next

- [Troubleshooting](/guides/troubleshooting) — what to do when a CI run fails unexpectedly.
- [Run a suite at scale](/guides/run-a-suite-at-scale) — if you haven't yet, start with the local version.
- [Deployment: Kubernetes](/deployment/kubernetes) — for running the replay stack in a persistent CI environment.
