# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.3
ARG ONNXRUNTIME_VERSION=1.22.0

FROM golang:${GO_VERSION}-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY api ./api
COPY config ./config
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/hoppify ./cmd/hoppify

FROM debian:bookworm-slim AS artifacts

ARG TARGETARCH
ARG ONNXRUNTIME_VERSION
ARG MODEL_URL=https://huggingface.co/chistopat/sku110k-yolo11-object-detector/resolve/main/weights/sku110k-yolo11-s640.onnx
ARG MODEL_SHA256=e8bc019d4241cf9486c4b6aaf50d51b652b3a0d641fce1189fbba12136794cd1

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tar \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /out

RUN mkdir -p models \
    && curl -fsSL --retry 3 --retry-delay 2 -o models/sku110k-yolo11-s640.onnx "${MODEL_URL}" \
    && echo "${MODEL_SHA256}  models/sku110k-yolo11-s640.onnx" | sha256sum -c -

RUN case "${TARGETARCH}" in \
        amd64) ort_arch=x64 ;; \
        arm64) ort_arch=aarch64 ;; \
        *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && curl -fsSL --retry 3 --retry-delay 2 -o onnxruntime.tgz \
        "https://github.com/microsoft/onnxruntime/releases/download/v${ONNXRUNTIME_VERSION}/onnxruntime-linux-${ort_arch}-${ONNXRUNTIME_VERSION}.tgz" \
    && mkdir -p lib \
    && tar -xzf onnxruntime.tgz --strip-components=2 -C lib \
        "onnxruntime-linux-${ort_arch}-${ONNXRUNTIME_VERSION}/lib/libonnxruntime.so.${ONNXRUNTIME_VERSION}" \
    && rm onnxruntime.tgz

FROM gcr.io/distroless/cc-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/hoppify /app/hoppify
COPY --from=artifacts /out/models ./models
COPY --from=artifacts /out/lib ./lib
COPY config ./config

ENV HOPPIFY_ENV=prod
ENV HOPPIFY_DETECTOR_RUNTIME_LIBRARY_PATH=/app/lib/libonnxruntime.so.1.22.0

EXPOSE 3000
EXPOSE 3001

USER nonroot:nonroot

ENTRYPOINT ["/app/hoppify"]
