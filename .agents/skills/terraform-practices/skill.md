# Terraform Best Practices

Use this skill when changing `terraform/` so infrastructure stays consistent with the repo's service and security model.

## Arguments

- `service`: Cloud Run service or shared infrastructure being added or changed
- `secrets`: Secret Manager, KMS, database, or cache values consumed by the service
- `networking`: load balancer, VPC, private IP, or public access behavior
- `overrides`: service-specific environment or annotation overrides

## Instructions

Keep shared infrastructure and shared configuration separate. Put reusable environment maps in locals, as done in `terraform/services.tf`, and keep service deployment details in `service_*.tf` files.

Give each service its own service account and grant only the IAM roles it needs. Model secrets as Secret Manager references and keep KMS, database, cache, and observability wiring explicit.

Merge shared environment first and service overrides last. Preserve the current pattern of `_all` overrides followed by per-service overrides so operators can change behavior without editing every resource.

Use explicit `depends_on` when a service relies on API enablement, IAM bindings, migrations, or secrets. Keep `lifecycle.ignore_changes` for deploy-managed Cloud Run fields so Terraform does not fight the delivery pipeline.

If a new service copies an existing `service_*.tf` file almost line-for-line, stop and consider extracting a reusable module instead of adding more duplication.
