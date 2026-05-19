FROM golang:1.25 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/agent ./cmd/agent/main.go

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /out/agent /app/agent

ENTRYPOINT ["/app/agent"]
