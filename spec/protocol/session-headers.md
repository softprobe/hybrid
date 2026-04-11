# Softprobe Session Headers

This document defines the initial shared request headers.

## Required header

- `x-softprobe-session-id`

## Optional related headers

- `x-softprobe-mode`
- `x-softprobe-case-id`
- `x-softprobe-test-name`

## Rules

- test framework helpers should attach `x-softprobe-session-id` to requests they initiate
- proxy may propagate the session header for internal service-to-service traffic within a controlled test session
- matching must not depend on ad hoc private headers outside this contract
