# Golang Patterns And Optimization

Use this skill when adding or refactoring Go code so new features follow this repo's package boundaries and performance habits.

## Arguments

- `boundary`: `cmd`, `internal`, or `pkg`
- `dependency`: concrete type or interface introduced by the change
- `hot_path`: request, render, cache, DB, or background path affected by the change
- `validation`: focused tests or compile checks for the touched slice

## Instructions

Preserve the current dependency direction. Put executable wiring in `cmd/`, private routing and assembly logic in `internal/`, and reusable helpers or adapters in `pkg/`. Keep controllers thin and use constructor injection for dependencies.

Prefer small interfaces at provider boundaries only. If a dependency has one implementation and one caller, start with a concrete type. Add an interface only when it removes a real coupling or enables testing.

Thread `context.Context` through operations that can block or need request-scoped data. Wrap errors with `%w` so failures keep local context.

Reuse existing optimization patterns before inventing new ones. `pkg/render/renderer.go` already shows the preferred shape for hot code: cache reusable state, guard shared mutable data with narrow locks, and use `sync.Pool` when allocation churn is real.

Avoid global mutable state, giant interfaces, and cross-package utility dumping. Validate the narrowest affected slice immediately after the first code change.
