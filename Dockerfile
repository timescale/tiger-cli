ARG GO_VERSION=1.25
ARG BINARY_SOURCE=builder

# When performing a multi-platform build, leverage Go's built-in support for
# cross-compilation instead of relying on emulation (which is much slower).
# See: https://docs.docker.com/build/building/multi-platform/#cross-compiling-a-go-application
FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION} AS builder
ARG TARGETOS
ARG TARGETARCH

# Download dependencies to local module cache
WORKDIR /src
RUN --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download -x

# Build static executable
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 go build -o /bin/tiger ./cmd/tiger

# When building Docker images via GoReleaser, the binaries are built externally
# and copied in. See: https://goreleaser.com/customization/dockers_v2/
FROM scratch AS release
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/tiger /bin/tiger

# Either the 'builder' or 'release' stage, depending on whether we're building
# the binaries in Docker or outside via GoReleaser.
FROM ${BINARY_SOURCE} AS binary_source

# Create final alpine image
FROM alpine:3.23 AS final

# Install psql for sake of `tiger db connect`
RUN apk add --update --no-cache postgresql-client

# Create non-root user/group
RUN addgroup -g 1000 tiger && adduser -u 1000 -G tiger -D tiger
USER tiger
WORKDIR /home/tiger

# Set env vars to control default Tiger CLI behavior
ENV TIGER_PASSWORD_STORAGE=pgpass
ENV TIGER_CONFIG_DIR=/home/tiger/.config/tiger

# Declare config file mount points
VOLUME /home/tiger/.config/tiger
VOLUME /home/tiger/.pgpass

# Copy binary to final image
COPY --from=binary_source /bin/tiger /usr/local/bin/tiger

ENTRYPOINT ["tiger"]
CMD ["mcp", "start"]
