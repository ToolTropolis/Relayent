# Relayent Relay — multi-stage build producing a tiny static image.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/relayent-relay ./relay

# The :nonroot tag runs as uid 65532 instead of root. The relay needs no
# privileges — it binds 8787 (>1024, so no CAP_NET_BIND_SERVICE), reads only
# env vars, and writes nothing. Running as root would mean any future RCE lands
# as uid 0, making a container escape far more valuable to an attacker; the
# plain distroless tag defaults to root, so this must be stated explicitly.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/relayent-relay /relayent-relay
# Redundant with the tag's default, but explicit: a base-image change must not
# silently promote this container back to root.
USER 65532:65532
EXPOSE 8787
ENTRYPOINT ["/relayent-relay"]
