# Build
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/cash-cannon .

# Run
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H app
USER app
WORKDIR /app
COPY --from=build /out/cash-cannon /app/cash-cannon
ENV GIN_MODE=release PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/cash-cannon"]
