ARG VERSION="dev"

FROM golang:1.23 AS build

# allow this step access to build arg
ARG VERSION

WORKDIR /usr/src/app

RUN go env -w GOMODCACHE=/root/.cache/go-build

# Install dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build go mod download && go mod verify


COPY main.go index.html .
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION} -X main.commit=$(git rev-parse HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -v -o /usr/local/bin/app ./...

# Make a stage to run the app
FROM gcr.io/distroless/base-debian12

COPY --from=build /usr/local/bin/app /usr/local/bin/

CMD ["/usr/local/bin/app", "-addr", "0.0.0.0"]
