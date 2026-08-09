# --- build ------------------------------------------------------------------
FROM golang:1.25-alpine AS build

WORKDIR /src

# Dependencies are resolved first so a source-only change reuses the layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO is not needed: the PostgreSQL drivers used here are pure Go, which keeps
# the runtime image free of a libc dependency.
RUN CGO_ENABLED=0 GOOS=linux go build \
	-trimpath \
	-ldflags="-s -w" \
	-o /out/api ./cmd/api

# --- runtime ----------------------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
	adduser -D -u 10001 sugar

COPY --from=build /out/api /usr/local/bin/api

USER sugar
EXPOSE 8080

# The migrations are embedded in the binary, so the image carries no SQL files
# and the container cannot drift from the code that expects them.
ENTRYPOINT ["/usr/local/bin/api"]
