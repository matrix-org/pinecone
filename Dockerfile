FROM docker.io/golang:alpine AS base

RUN apk --update --no-cache add bash build-base git

WORKDIR /build

COPY . /build

RUN mkdir -p bin
RUN go build -trimpath -o bin/ ./cmd/pinecone

FROM alpine:latest
LABEL org.opencontainers.image.title="Pinecone"
LABEL org.opencontainers.image.description="Standalone Pinecone router"
LABEL org.opencontainers.image.source="https://github.com/matrix-org/pinecone"
LABEL org.opencontainers.image.licenses="Apache-2.0"

COPY --from=base /build/bin/* /usr/bin/

EXPOSE 65432/tcp
EXPOSE 65433/tcp

ENTRYPOINT ["/usr/bin/pinecone", "-listenws=:65433", "-listen=:65432"]