# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /openmind ./cmd/openmind
RUN mkdir -p /data/assets

# Runtime stage
FROM gcr.io/distroless/static:nonroot
COPY --from=build /openmind /openmind
# Pre-create the assets mount point owned by the nonroot user (uid/gid 65532)
# so a freshly-initialised named volume at /data/assets inherits writable
# ownership instead of defaulting to root:root.
COPY --from=build --chown=65532:65532 /data/assets /data/assets
EXPOSE 8080
ENTRYPOINT ["/openmind"]
CMD ["all"]
