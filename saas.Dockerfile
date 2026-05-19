FROM node:20-alpine AS frontend

WORKDIR /src/web-frontend

COPY web-frontend/package.json web-frontend/package-lock.json ./
RUN npm ci

COPY web-frontend/ ./
RUN npm run build

FROM golang:1.25 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/saas ./cmd/saas/main.go

FROM alpine:3.20

RUN apk add --no-cache tzdata

WORKDIR /app

COPY --from=builder /out/saas /app/saas
COPY config.yaml /app/config.yaml
COPY --from=frontend /src/web-frontend/dist /app/web-frontend/dist

EXPOSE 8080

ENTRYPOINT ["/app/saas"]
CMD ["-config", "/app/config.yaml"]
