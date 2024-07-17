# Builder image
FROM golang:1.22.1 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN --mount=type=cache,target="/root/.cache/go-build" CGO_ENABLED=0 GOOS=linux go build -v -o main .

# Build migrate
RUN CGO_ENABLED=0 GOOS=linux go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Final image
FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /root/

COPY --from=builder /app/main .
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /go/bin/migrate /usr/local/bin/migrate

CMD ["./main"]