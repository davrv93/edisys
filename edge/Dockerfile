# EDISYS · borde (Caddy). Multi-stage: construye la app (Astro, base /app) y copia el dist.
# Contexto de build: la RAÍZ del monorepo (la app lee ../packages/tokens).
# Si la app no compila, la imagen sale igual con una página de reserva (no bloquea el resto).

FROM node:22-slim AS app
WORKDIR /src
COPY packages ./packages
COPY app/package.json app/package-lock.json* ./app/
WORKDIR /src/app
RUN npm ci --no-audit --no-fund || npm install --no-audit --no-fund || true
COPY app ./
COPY edge/reserva /reserva
RUN mkdir -p /out/app && \
    ( npm run build && cp -r dist/. /out/app/ && echo "app: construida" ) || \
    ( echo "app: NO compiló; se usa la página de reserva" && cp /reserva/index.html /out/app/index.html )

FROM caddy:2-alpine
COPY edge/Caddyfile /etc/caddy/Caddyfile
COPY --from=app /out/app /srv/app
EXPOSE 80
