# Licensing

Softprobe uses a **dual-license split** designed around a simple principle:

- **Server-side code** — the parts a competitor would need to build a product
  that competes with Softprobe — is under the
  [**Softprobe Source License 1.0**](./LICENSE) (SPDX:
  `LicenseRef-Softprobe-Source-License-1.0`). This is a source-available
  license derived from the [Functional Source License 1.1](https://fsl.software)
  with a **materially broader** non-compete clause: it prohibits using
  Softprobe's server code in *any* product or service that competes with
  Softprobe's commercial offerings — hosted, on-premises, bundled,
  rebranded, or otherwise — not just hosted SaaS redistribution. You may
  use it freely for your own internal business, research, and consulting
  work. After two years from each release date, that release automatically
  re-licenses to Apache-2.0 and the non-compete restriction lifts.

- **Client SDKs and protocol schemas** — the code you embed into your own
  applications — are under the standard permissive
  [**Apache License, Version 2.0**](./softprobe-js/LICENSE). Use them in
  proprietary products, commercial or otherwise, with no restrictions
  beyond normal Apache-2.0 attribution.

This split is modelled on the same pattern HashiCorp, Sentry, and other
source-available vendors use — but with a stricter server-side non-compete
than stock FSL 1.1. The libraries that customers actually link into their
own code stay permissive so enterprise legal review is never a blocker for
adoption.

## Path map

| Path | License | SPDX | Notes |
|------|---------|------|-------|
| `/LICENSE` (repo root) | Softprobe Source License 1.0 | `LicenseRef-Softprobe-Source-License-1.0` | Default for any file not inside a package below |
| `softprobe-runtime/` | Softprobe Source License 1.0 | `LicenseRef-Softprobe-Source-License-1.0` | Unified runtime and `softprobe` CLI |
| `softprobe-proxy/` | Softprobe Source License 1.0 | `LicenseRef-Softprobe-Source-License-1.0` | Envoy + WASM data plane |
| `softprobe-js/` | Apache-2.0 | `Apache-2.0` | TypeScript/Node SDK (published to npm) |
| `softprobe-python/` | Apache-2.0 | `Apache-2.0` | Python SDK |
| `softprobe-java/` | Apache-2.0 | `Apache-2.0` | Java SDK (published to Maven Central) |
| `softprobe-go/` | Apache-2.0 | `Apache-2.0` | Go SDK |
| `spec/` | Apache-2.0 | `Apache-2.0` | Protocol definitions, JSON schemas, example case files |
| `docs-site/` | follows repo root (SSL-1.0) | — | User-facing documentation content |
| `e2e/` | follows repo root (SSL-1.0) | — | End-to-end test harnesses |
| `docs/` | follows repo root (SSL-1.0) | — | Internal design docs |

When a subdirectory contains its own `LICENSE` file, that license governs
everything inside that subdirectory. When no per-directory `LICENSE` exists,
the root [`LICENSE`](./LICENSE) applies.

## What counts as a "Competing Use"

The binding definition is in [`LICENSE`](./LICENSE); this section is a
plain-English summary and is not itself legally binding. Read the license
text directly for any legal determination.

A **Competing Use** is using the server-side Software — or a derivative of
it — in any product, service, or offering that both:

1. is offered or made available to third parties, **and**
2. provides substantially the same functionality as a Softprobe commercial
   product or service (HTTP/RPC traffic capture, replay, session-based
   mocking, service virtualization, record-and-replay regression testing,
   topology-aware test orchestration, or any hosted / on-prem / bundled
   form thereof).

This is broader than the canonical Functional Source License 1.1, which
restricts only hosted/embedded-service redistribution. Under the Softprobe
Source License, you also **cannot**:

- rebrand the server code and sell it as an on-premises product;
- bundle the server code into a larger commercial product whose value
  proposition includes capture/replay/mocking;
- publish a fork on a cloud marketplace as an alternative to Softprobe's
  hosted offering;
- resell the CLI as a replay-testing tool under another name.

## What you definitely *can* do

You **can**:

- Run `softprobe-runtime`, `softprobe-proxy`, and the `softprobe` CLI
  inside your company or lab for any workload, including capturing and
  replaying your own production traffic — at any scale, in any industry,
  commercial or otherwise.
- Build internal tooling, CI pipelines, and test harnesses on top of them.
- Use them in consulting or professional-services engagements where the
  client is the licensee and is using the Software under this license
  (provided the engagement isn't itself structured as a Competing Use —
  e.g., running a "managed replay testing service" for the client's own
  customers would cross the line).
- Embed the Apache-2.0 client SDKs in your own commercial products,
  including products that happen to interact with captured traffic, *as
  long as* those products aren't themselves replay-testing platforms
  competing with Softprobe.
- Fork, modify, and redistribute — including running modified builds
  internally — as long as the distribution terms and non-compete
  restriction remain intact.
- Evaluate the Software for up to 60 days to decide whether you need a
  commercial license.

You **cannot**:

- Offer `softprobe-runtime` or `softprobe-proxy` as a hosted, managed, or
  embedded service through which third parties can record, replay, mock,
  or otherwise manage their own traffic or workloads.
- Package `softprobe-runtime` or `softprobe-proxy` into a standalone or
  bundled product sold to third parties where that product's primary
  value is replay testing, service virtualization, or traffic mocking.
- Use the code to become a Softprobe competitor by any mechanism,
  including white-label, OEM, or "powered by Softprobe" rebadging.

In short: build with Softprobe as much as you like; just don't become a
Softprobe competitor using Softprobe's own code.

## Automatic conversion to Apache-2.0

Every release of Softprobe under the Softprobe Source License re-licenses
to Apache-2.0 two years after that release's publication date (the "Change
Date"). So if we publish a release on 2026-04-21, that exact commit
automatically becomes Apache-2.0-licensed on 2028-04-21 — no action
required from anyone, no retroactive clawback possible, and the
Competing-Use restriction lifts completely for that release. Older
releases always grow more permissive over time.

## Why this is not stock FSL 1.1

The canonical [Functional Source License 1.1](https://fsl.software/) is
deliberately narrow — it only restricts hosted-or-embedded-service
redistribution, leaving all other forms of redistribution (on-premises
products, bundling, rebranding) permitted. That's the right tradeoff for
some vendors but it doesn't match Softprobe's intent.

The Softprobe Source License keeps everything else from FSL — the
two-year Apache-2.0 conversion, the patent grant structure, the
redistribution clause, the trademark clause — and widens only the
Competing Use definition. The license file opens with a preamble stating
exactly this, so license scanners, enterprise legal reviewers, and
contributors all see the deviation up front. SPDX identifies this as a
custom `LicenseRef-` rather than the stock `FSL-1.1-Apache-2.0`, because
an SPDX ID claim must be verbatim-accurate.

## Contributing

Contributions are accepted under the same license that governs the file
you're modifying. By opening a pull request you agree that your
contribution is licensed under that file's license. No CLA is required.

## Questions

Open an issue at https://github.com/softprobe/softprobe or email
`legal@softprobe.io`. For commercial terms (hosting, OEM, redistribution
outside the Softprobe Source License grant), contact `sales@softprobe.io`.
