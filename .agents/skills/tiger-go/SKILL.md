---
name: tiger-go
description: Implement reliability-critical Go with bounded concurrency,
  explicit errors, checked arithmetic, strict decoding and reproducible tests.
---

# Tiger Go

## Before implementation

Identify:
- errors versus invariant failures
- goroutine ownership
- channel capacities
- collection limits
- context lifetime
- arithmetic ranges
- decode limits
- shutdown behaviour

## Errors

Return errors for expected failures.

Always decide deliberately whether an error may be ignored.
Blank assignment of an error requires an explanatory reason.

## Goroutines

Before starting a goroutine, answer:
- who owns it?
- how does it terminate?
- can external input create unlimited instances?
- how are failures observed?
- what happens during shutdown?

Prefer fixed worker pools/semaphores when work is externally driven.

## Channels

Prefer a bounded channel with a documented overload policy.

Capacity full must cause one of:
- backpressure
- rejection
- shedding
- controlled degradation

Never turn overload into unconstrained goroutine creation.

## Arithmetic

Do not assume overflow traps.

Use:
- precondition comparisons
- math/bits for carry/borrow where useful
- wider/domain-specific representations where appropriate

## Decoding

Apply byte limits first.

For strict JSON:
- decode into a concrete boundary DTO
- reject unknown fields where contract requires
- reject trailing values
- validate domain constraints
- then construct trusted domain objects

## Determinism

Never depend on Go map iteration order.

Inject time/randomness where tests need reproducibility.

## Testing

Unit:
table-driven tests for expected and edge cases.

Property:
Rapid where shrinking/state-machine testing adds value.

Fuzz:
native fuzz targets for parsers and state transitions.

Concurrency:
go test -race.
Use testing/synctest for time/goroutine scheduling logic where suitable.

Performance:
Go benchmarks and statistical comparison for claimed regressions/improvements.
