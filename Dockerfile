# Build image for every PayGate Go service.
#
# One Dockerfile serves all of cmd/*: select the binary with the SERVICE build
# argument, e.g.
#
#   docker build --build-arg SERVICE=api-gateway -t paygate/api-gateway .
#   docker build --build-arg SERVICE=outbox-relay -t paygate/outbox-relay .

# ---------- build ----------
FROM golang:1.26-alpine AS build

WORKDIR /src

# Dependencies first so a source-only change does not re-download the module cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG SERVICE=api-gateway
ARG VERSION=dev
# CGO is off so the result is a static binary that runs on a distroless base.
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/service ./cmd/${SERVICE}

# ---------- runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# api-gateway bootstraps the event schema registry from these fixtures on
# startup and fails hard without them, so they must ship inside the image.
COPY --from=build /src/schemas ./schemas
COPY --from=build /out/service /app/service

USER nonroot:nonroot
EXPOSE 8090

ENTRYPOINT ["/app/service"]
