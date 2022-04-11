FROM docker.io/golang:alpine AS base

RUN apk --update --no-cache add bash build-base git

WORKDIR /build

COPY . /build

RUN mkdir -p bin
RUN go build -trimpath -o bin/ ./cmd/pineconeproxy

FROM alpine:latest
LABEL org.opencontainers.image.title="Pinecone Proxy"
LABEL org.opencontainers.image.description="HTTP to Pinecone proxy"
LABEL org.opencontainers.image.source="https://github.com/matrix-org/pinecone"
LABEL org.opencontainers.image.licenses="Apache-2.0"

COPY --from=base /build/bin/* /usr/bin/

EXPOSE 8228/tcp

ENTRYPOINT ["/usr/bin/pineconeproxy"]