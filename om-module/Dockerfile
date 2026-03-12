FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o om-module .

FROM alpine:latest

RUN apk --no-cache add ca-certificates docker-cli curl jq

WORKDIR /app

COPY --from=builder /app/om-module .

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
CMD wget -qO- http://localhost:8080/ping || exit 1

CMD ["./om-module"]
