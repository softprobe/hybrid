# Softprobe Repo Layout

This document defines the target multi-repo strategy.

---

## 1) Target repos

The target platform layout is:

1. `softprobe-spec`
2. `softprobe-proxy`
3. `softprobe-js`
4. `softprobe-python`
5. `softprobe-java`

---

## 2) Responsibilities

### `softprobe-spec`

- canonical schemas
- protocol definitions
- compatibility fixtures

### `softprobe-proxy`

- HTTP interception and enforcement

### `softprobe-js`

- JS SDK
- JS CLI
- JS generator
- optional local runtime implementation

### `softprobe-python`

- Python SDK
- Pytest integration
- generator
- runtime client

### `softprobe-java`

- Java SDK
- JUnit integration
- generator
- runtime client

---

## 3) Dependency rules

Allowed:

- all implementation repos depend on `softprobe-spec`

Disallowed:

- language repos depending on each other
- proxy depending on any language repo
- spec depending on any implementation repo

---

## 4) Short-term transition

Current repos in the workspace are:

- `proxy`
- `softprobe-js`

Current transition plan:

- treat `proxy` as the future `softprobe-proxy`
- use `softprobe-spec` as the new canonical home for shared contracts
- keep `softprobe-js` focused on JS implementation concerns

---

## 5) Runtime placement

The runtime may stay in `softprobe-js` temporarily as a reference implementation, but the shared protocol and schemas must not stay there as the permanent source of truth.
