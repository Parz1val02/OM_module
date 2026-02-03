FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git
WORKDIR /app
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o metrics-tap

FROM alpine:latest

RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY --from=builder /app/metrics-tap /app/metrics-tap

EXPOSE 55555/udp

ENTRYPOINT ["/app/metrics-tap"]
