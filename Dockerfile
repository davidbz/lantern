# syntax=docker/dockerfile:1

# Build: docker build --build-arg CMD=./cmd/lantern -t lantern . (or `make docker`)
FROM --platform=$BUILDPLATFORM golang:1.27-trixie AS build

WORKDIR /src

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=bind,source=go.sum,target=go.sum \
    go mod download

ARG CMD=./cmd/lantern
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/lantern "${CMD}"

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=build /out/lantern /lantern
USER nonroot:nonroot
ENTRYPOINT ["/lantern"]
