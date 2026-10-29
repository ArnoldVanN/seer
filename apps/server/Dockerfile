# Build from the repo root: docker build -f apps/server/Dockerfile .
FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS build
WORKDIR /src

RUN go install github.com/bufbuild/buf/cmd/buf@v1.51.0 \
 && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
 && go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.20.0

COPY packages/protos/ packages/protos/
RUN cd packages/protos && rm -rf gen && buf generate --template buf.gen.go.yaml

COPY apps/server/go.mod apps/server/go.sum apps/server/
RUN cd apps/server && go mod download

COPY apps/server/ apps/server/
ARG VERSION=dev
ARG TARGETOS TARGETARCH
RUN cd apps/server && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" -o /out/seer ./cmd/seer

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/seer /seer
EXPOSE 8080 8081 9090
ENTRYPOINT ["/seer"]
CMD ["all", "-config", "/config/seer.yaml"]
