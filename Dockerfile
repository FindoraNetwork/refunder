FROM --platform=$TARGETPLATFORM docker.io/golang:1.18.4-alpine AS builder

WORKDIR /app
COPY go.mod go.sum .
RUN go mod download
COPY . .

ENV CGO_ENABLED=0
ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o refunder .

FROM docker.io/busybox:latest
COPY --from=builder /app/refunder /refunder
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

ENTRYPOINT ["/refunder"]
