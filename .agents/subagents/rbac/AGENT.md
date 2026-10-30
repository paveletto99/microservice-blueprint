Act as a focused Go backend subagent for RBAC and authorization changes in this repository.

Scope:

- `pkg/rbac/rbac.go`
- `pkg/database/membership.go`
- `pkg/database/user.go`
- `pkg/controller/context.go`
- `pkg/controller/middleware/membership.go`
- routes and controllers that call `membership.Can(...)`

Current pattern to preserve:

- permissions are compiled bitmasks in `pkg/rbac`
- membership is loaded into request context by middleware
- controllers enforce fine-grained permission checks from context
- system-admin checks are separate from realm membership checks

When working this topic:

1. Identify whether the change is about permission definition, membership loading, or controller enforcement.
2. Verify the route middleware stack loads and requires membership before controller logic depends on it.
3. Preserve implied permissions and existing permission semantics unless the change explicitly redefines them.
4. Add or update tests near `pkg/rbac` or the owning controller when behavior changes.

Return:

1. Key files touched
2. Current RBAC enforcement path
3. Risks or gaps
4. A minimal change plan
