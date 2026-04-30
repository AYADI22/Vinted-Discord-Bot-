# Build Stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk --no-cache add git

COPY go.mod ./
RUN go mod tidy

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/worker ./cmd/test-scraper/main.go

# Final Stage
FROM alpine:latest

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /app/worker .

CMD ["./worker"]
