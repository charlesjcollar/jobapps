# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
# Regenerate the plugin import list so adding/removing a plugin folder is all it takes.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go run ./plugins/internal/gen && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bff .

# Static binary on an empty base image: just the binary plus the CA certificates
# plugins need to make HTTPS requests to company career sites.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /bff /bff
EXPOSE 8080
ENTRYPOINT ["/bff"]
