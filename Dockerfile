ARG GO_VERSION=1.25

# When performing a multi-platform build, leverage Go's built-in support for
# cross-compilation instead of relying on emulation (which is much slower).
# See: https://docs.docker.com/build/building/multi-platform/#cross-compiling-a-go-application
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build
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

# Expose port for sake of MCP server using streamable HTTP transport
EXPOSE 8080

# Copy binary to final image
COPY --from=build /bin/tiger /usr/local/bin/tiger

CMD ["tiger", "mcp", "start", "http", "--host=", "--port=8080"]
