ARG GOOSE_VERSION=v3.27.1

FROM golang:1.25-bookworm AS build
ARG GOOSE_VERSION

RUN CGO_ENABLED=0 go install "github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION}"

FROM debian:bookworm-slim

COPY --from=build /go/bin/goose /usr/local/bin/goose

ENTRYPOINT ["goose"]
