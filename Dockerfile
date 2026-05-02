FROM golang:1.26 AS build

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN go build ./cmd/...

FROM debian:13.4-slim

RUN DEBIAN_FRONTEND=noninteractive apt update \
    && apt install -y ca-certificates

# nobody
USER 65534:65534

COPY --from=build --chown=65534:65534 /build/bot /usr/bin/bot

ENTRYPOINT [ "/usr/bin/bot" ]