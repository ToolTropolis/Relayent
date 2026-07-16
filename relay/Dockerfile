# Relayent Relay — multi-stage build producing a tiny static image.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 go build -o /out/relayent-relay ./relay

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/relayent-relay /relayent-relay
EXPOSE 8787
ENTRYPOINT ["/relayent-relay"]
