# Proxy integration posture (Softprobe WASM + customer OpenTelemetry)

**Status:** design decision record  
**Audience:** Product, field engineers, customers evaluating Istio + Softprobe  
**Normative platform design:** [design.md](./design.md), [proxy-otel-api.md](../spec/protocol/proxy-otel-api.md)

---

## 1. Executive summary

The Envoy / Istio **Softprobe WASM** plugin sends **full HTTP capture** (headers and bodies) to **`softprobe-runtime`** over the **proxy OTLP API** (`POST /v1/inject`, `POST /v1/traces`) as configured by **`sp_backend_url`** (for example `https://o.softprobe.ai`). That stream is **out-of-band (OOB)** with respect to the customer’s **existing** OpenTelemetry export path (Datadog, Honeycomb, New Relic, Sentry, an in-house collector, and so on).

We **do not** position the plugin as “drop spans into your existing APM unchanged.” Customer backends **truncate** large span attributes, enforce **payload size limits** (for example HTTP 413), and bill by **ingested span volume**. Full request/response bodies belong in **Softprobe’s** store and UI, not as unbounded tags on the customer’s production traces.

---

## 2. Market sizing (Istio + OpenTelemetry together)

Precise intersection counts are not available, but directional evidence is enough for product planning:

- **Service mesh** adoption is concentrated in mature cloud-native orgs; **Istio** is one of several options (ambient mesh, Cilium, Linkerd, and others compete for the same budgets).
- **OpenTelemetry** adoption is much broader than Istio; many teams export **directly** from the SDK to a vendor **without** running a central OpenTelemetry Collector.
- The subset “**Istio in production** + **willing to add a new WASM filter** + **comfortable with egress to Softprobe**” is a **credible enterprise** channel but **not** the only onboarding path. CLI-first capture, language SDKs, and self-hosted `softprobe-runtime` remain essential.

Treat **WasmPlugin + `sp_backend_url`** as a **high-signal** integration for accounts that already run a mesh, not as the sole definition of the product.

---

## 3. Why not share the customer’s OTLP pipeline for bodies?

Typical customer pipelines assume **small** span attributes. OpenTelemetry SDKs support **`OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT`**; vendors add **max tag length** and reject oversize payloads. Softprobe’s proxy-generated spans intentionally carry **large** `http.request.body` / `http.response.body` attributes (see `softprobe-proxy/src/otel.rs`).

If we merged that stream into the customer’s collector without filtering:

- Their **cost** and **cardinality** would spike.
- Their **dashboards** would show **truncated or dropped** data, not trustworthy capture.

A **supported** shared-pipeline mode would require the customer to run an **OpenTelemetry Collector** with a **filter** that strips Softprobe-heavy spans before the vendor exporter, **plus** a **second exporter** to Softprobe. That is valid for sophisticated platform teams but is **not** the default story we document for install.

**Default posture:** OOB OTLP/HTTP to **`sp_backend_url`** only for the heavy capture path.

---

## 4. Diagram: default vs optional shared pipeline

```mermaid
flowchart LR
  subgraph default [Default - OOB]
    AppD[App with OTel SDK]
    EnvoyD[Envoy + Softprobe WASM]
    VendorD[Customer APM or collector]
    SPD[softprobe-runtime at sp_backend_url]
    AppD --> VendorD
    EnvoyD --> SPD
  end

  subgraph optional [Optional - advanced]
    AppO[App with OTel SDK]
    EnvoyO[Envoy + Softprobe WASM]
    ColO[Customer OTel Collector]
    VendorO[Customer APM]
    SPO[softprobe-runtime]
    AppO --> ColO --> VendorO
    EnvoyO --> SPO
    note1[Some teams tee duplicate OTLP; requires filter + second exporter]
  end
```

In the **default** diagram, the customer’s traces and Softprobe’s capture **never** share the same export batch for body-sized fields.

---

## 5. Duplicate span vs enriching Istio’s span

The WASM filter may emit spans that **correlate** with Istio’s HTTP spans (same W3C `trace_id` / `traceparent` propagation). That does **not** mean we should push **full bodies** onto the **active** Istio span in the customer’s backend.

**Deferred option (not shipped):** Envoy can set **small string tags** on the active span via the proxy-wasm property path **`trace_span_tag`** (backed by `envoy_set_active_span_tag`). That is suitable for **correlation only** (for example session id, trace id hex, a short URL to the Softprobe UI). It is **not** a substitute for OOB capture of bodies.

Implementation of that enrichment is tracked in [`tasks.md`](../tasks.md) parking lot; see also [language-instrumentation.md](./language-instrumentation.md) for the same truncation argument on the **language** plane.

---

## 6. Documentation rules

- Do **not** imply that installing the **WasmPlugin** alone causes **full-body** traffic to appear in **Datadog**, **Honeycomb**, or similar **without** a separate Softprobe destination and consent to egress.
- Do say that **`sp_backend_url`** is the **Softprobe runtime** (hosted or self-hosted) and that the customer’s **existing** OTel pipeline remains **unchanged** unless they explicitly opt into a **tee + filter** architecture.

---

## 7. Related links

- [design.md §2.5 Instrumentation planes](./design.md#25-instrumentation-planes-proxy-vs-language) (proxy vs language; links here)
- [language-instrumentation.md](./language-instrumentation.md)
- [session-headers.md](../spec/protocol/session-headers.md)
- [proxy-otel-api.md](../spec/protocol/proxy-otel-api.md)
