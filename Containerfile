# servly, packaged as a container image: one process serving the API and the
# web frontend. Data (SQLite database, uploaded assets) lives in /data.
#
# Build: podman build -t servly:dev -f Containerfile .
# Run:   podman run --rm -p 8080:8080 -v servly-data:/data servly:dev

FROM --platform=$BUILDPLATFORM docker.io/library/node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.26-alpine AS build
WORKDIR /src

ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
      -ldflags "-s -w -X github.com/servly-io/servly/internal/cli.Version=${VERSION}" \
      -o /out/servly ./cmd/servly && \
    mkdir -p /out/data

# distroless/static: CA certificates and a nonroot user, nothing else. The
# binary embeds tzdata and the frontend and needs no libc (CGO_ENABLED=0).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/servly /usr/local/bin/servly
# Owned by nonroot so a fresh named volume mounted here is writable.
COPY --from=build --chown=65532:65532 /out/data /data
ENV SERVLY_DATA_DIR=/data \
    SERVLY_LISTEN_ADDR=:8080
VOLUME ["/data"]
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/usr/local/bin/servly"]
CMD ["serve"]
