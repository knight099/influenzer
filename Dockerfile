FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o server cmd/api/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/server .
COPY .env .env
# Optional: COPY config config if needed

EXPOSE 8080
CMD ["./server"]
