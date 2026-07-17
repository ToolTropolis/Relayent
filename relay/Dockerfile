# Relayent Relay — multi-stage build producing a tiny static image.
FROM golang:1.26-alpine AS build
WORKDIR /src
# Copy go.mod AND go.sum so the module graph is pinned and the build is
# reproducible. go.sum exists once the relay has dependencies (bbolt, go-oidc).
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/relayent-relay ./relay

# The :nonroot tag runs as uid 65532 instead of root. The relay needs no
# privileges — it binds 8787 (>1024, so no CAP_NET_BIND_SERVICE) and reads only
# env vars. Running as root would mean any future RCE lands as uid 0, making a
# container escape far more valuable to an attacker; the plain distroless tag
# defaults to root, so this must be stated explicitly.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/relayent-relay /relayent-relay
# Redundant with the tag's default, but explicit: a base-image change must not
# silently promote this container back to root.
USER 65532:65532
# /data is where the control-plane store lives in multi-tenant mode. Declaring
# the volume documents the writable path and lets a bind/named volume mount here
# alongside a read-only rootfs. The image does NOT set RELAYENT_DATA_DIR: without
# it the relay is stateless (legacy single-key), exactly as before. To enable
# multi-tenant, set RELAYENT_DATA_DIR=/data and mount a volume (owned by 65532).
VOLUME ["/data"]
EXPOSE 8787
ENTRYPOINT ["/relayent-relay"]
