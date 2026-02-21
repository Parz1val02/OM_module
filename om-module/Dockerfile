FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# Add go.sum file later
COPY go.mod ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o om-module ./cmd/om-module

FROM alpine:latest

# docker-cli curl jq: add package to install later
RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/om-module .

EXPOSE 8080

CMD ["./om-module"]
