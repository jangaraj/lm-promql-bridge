FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/lm-promql-bridge cmd/bridge/main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /bin/lm-promql-bridge .

EXPOSE 8080
CMD ["./lm-promql-bridge"]
