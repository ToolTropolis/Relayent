# Relayent Demo — multi-stage build producing a tiny static image.
FROM golang:1.26-alpine AS build
WORKDIR /src
# Pin the module graph for a reproducible build (shared root go.mod/go.sum).
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/relayent-demo ./demo

# Distroless nonroot (uid 65532): the demo needs no privileges — it binds a high
# port and holds only env config. Running as root would make any future RCE land
# as uid 0, so this is stated explicitly (the plain tag defaults to root).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/relayent-demo /relayent-demo
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/relayent-demo"]
