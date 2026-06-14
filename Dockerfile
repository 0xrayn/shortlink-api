# ---- Build stage ----
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o shortlink .

# ---- Run stage ----
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/shortlink .

EXPOSE 8080

CMD ["./shortlink"]
