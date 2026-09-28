# EDISYS · API en Go. Binario estático sobre distroless (sin shell ni curl: el healthcheck es `edisys salud`).
FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/edisys ./cmd/edisys

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/edisys /edisys
ENV PORT=8080 GOMEMLIMIT=200MiB
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/edisys"]
CMD ["serve"]
