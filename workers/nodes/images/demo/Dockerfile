# k8flare-demo: the v1 allowlisted Pod image for workers/nodes (Phase 7).
# distroless, not scratch -- spikes/s3-containers/FINDINGS.md measured that
# `FROM scratch` has no CA bundle (a TLS client inside the container fails
# x509 verification), so k8flare's own images use distroless-family bases
# per that finding, even though this particular demo never dials out.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY main.go go.mod ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /demo .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /demo /demo
EXPOSE 8080
ENTRYPOINT ["/demo"]
