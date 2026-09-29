# EDISYS · motor conversacional en Go. Mismo contrato que motor/app.py (ver CONTRATO.md).
# Imagen final debian-slim: onnxruntime se distribuye enlazado contra glibc.
# Los modelos van por volumen (/models), no en la imagen.
#
#   docker build -t edisys/motor-go --build-context indice=../motor/indice .
#
# El contexto «indice» trae la semilla (faq.json sin vectores) que se copia a
# /motor/indice, como hacía la imagen Python: un volumen nuevo arranca con ella.
ARG ORT_VERSION=1.19.2

FROM golang:1.27-bookworm AS base
ARG TARGETARCH
ARG ORT_VERSION
RUN set -eux; \
    case "${TARGETARCH:-$(dpkg --print-architecture)}" in \
      amd64) ORT=x64 ;; \
      arm64) ORT=aarch64 ;; \
      *) echo "arquitectura no soportada"; exit 1 ;; \
    esac; \
    mkdir -p /opt/ort; \
    curl -fsSL "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-linux-${ORT}-${ORT_VERSION}.tgz" \
      | tar xz -C /opt/ort --strip-components=1; \
    cp -L /opt/ort/lib/libonnxruntime.so /opt/libonnxruntime.so
ENV CGO_ENABLED=1 ORT_LIB=/opt/libonnxruntime.so
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Pruebas: docker build --target test … (las de e5 piden los modelos por volumen:
# docker run --rm -v ../motor/models:/models:ro -e E5_RUTA=… <imagen-test>)
FROM base AS test
ENV E5_RUTA=/models/e5-model_quantized.onnx E5_TOK=/models/e5-tokenizer.json
CMD ["go", "test", "-count=1", "./..."]

FROM base AS build
RUN go vet ./... && go test -count=1 ./... && \
    go build -trimpath -ldflags="-s -w" -o /out/motor .

FROM debian:bookworm-slim
ARG ORT_VERSION
# uid 999 = el usuario «edisys» de la imagen Python: el volumen motor_indice
# ya existe con ese dueño y el motor Go tiene que poder reescribirlo.
RUN useradd --system --uid 999 --home /motor edisys && mkdir -p /motor/indice
COPY --from=base /opt/libonnxruntime.so /usr/local/lib/libonnxruntime.so
COPY --from=build /out/motor /usr/local/bin/motor
COPY --from=indice --chown=999:999 . /motor/indice/
RUN chown -R edisys:edisys /motor
ENV ORT_LIB=/usr/local/lib/libonnxruntime.so \
    E5_RUTA=/models/e5-model_quantized.onnx E5_TOK=/models/e5-tokenizer.json \
    INDICE=/motor/indice LLAMA_URL=http://llama:8080 PUERTO=8080 \
    GOMEMLIMIT=200MiB MALLOC_ARENA_MAX=2
WORKDIR /motor
USER edisys
EXPOSE 8080
HEALTHCHECK --interval=60s --timeout=5s --start-period=20s --retries=3 \
  CMD ["/usr/local/bin/motor", "-salud"]
ENTRYPOINT ["/usr/local/bin/motor"]
