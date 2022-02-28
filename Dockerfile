FROM docker.io/golang:alpine AS builder

COPY . /app
WORKDIR /app

ENV CGO_ENABLED=0
RUN go build -ldflags="-s -w" -o refunder .

FROM docker.io/busybox:latest
COPY --from=builder /app/refunder /refunder

ENTRYPOINT ["/refunder"]
