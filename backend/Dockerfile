# ── build ──────────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# garante go.sum completo mesmo se ainda não tiver sido commitado
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

# ── runtime ────────────────────────────────────────────────────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget && adduser -D -H -u 10001 app
ENV TZ=America/Recife
COPY --from=build /out/api /api
COPY --from=build /out/seed /seed
USER app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/api"]
