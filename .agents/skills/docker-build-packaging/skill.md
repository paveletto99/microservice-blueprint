# Docker Build And Packaging

Use this skill when a service needs a repeatable binary build plus container packaging flow.

## Arguments

- `service`: binary name under `cmd/`
- `image`: target image reference
- `tag`: image tag or release identifier
- `builder`: local `docker` build only or remote Cloud Build wrapper

## Instructions

Follow the repo's binary-first pattern. Build the Linux binary into `bin/` first, then package it with a thin Dockerfile that only copies the prebuilt artifact. This keeps the runtime image small and keeps toolchains out of the final container.

For local builds, prefer:

`mkdir -p bin && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o ./bin/${service} ./cmd/${service}`

For packaging, keep `builders/service.dockerfile` generic and pass the service name through `--build-arg SERVICE=...`. Use a non-root runtime user and copy CA certificates into the final image.

If the project uses remote builds, keep a small `scripts/build` wrapper around `gcloud builds submit` and move the actual steps into `builders/build.yaml`.

Validate the slice in this order: the binary builds, the Docker image builds, then the wrapper command points at the right service, image, and tag.
