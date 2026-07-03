# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /openmind ./cmd/openmind

# Runtime stage
FROM gcr.io/distroless/static:nonroot
COPY --from=build /openmind /openmind
EXPOSE 8080
ENTRYPOINT ["/openmind"]
CMD ["all"]
