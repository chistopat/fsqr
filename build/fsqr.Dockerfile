# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.3

FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY api ./api
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/fsqr ./cmd/fsqr

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/fsqr /app/fsqr
COPY config ./config

ENV FSQR_ENV=prod

EXPOSE 3000

USER nonroot:nonroot

ENTRYPOINT ["/app/fsqr"]
