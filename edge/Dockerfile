# EDISYS · borde (Caddy). Multi-stage: construye la app (Astro) y copia el dist.
# Contexto de build: la RAÍZ del monorepo (la app lee ../packages/tokens).
# Si la app no compila, la imagen sale igual con una página de reserva (no bloquea el resto).
#
# EDISYS_PREFIJO: prefijo de URL pública. Vacío en local; «/edisys» bajo el alias de boticalima.

FROM node:22-slim AS app
WORKDIR /src
ARG EDISYS_PREFIJO=""
COPY packages ./packages
COPY app/package.json app/package-lock.json* ./app/
WORKDIR /src/app
RUN npm ci --no-audit --no-fund || npm install --no-audit --no-fund || true
COPY app ./
COPY edge/reserva /reserva
ENV EDISYS_PREFIJO=${EDISYS_PREFIJO}
RUN mkdir -p /out/srv${EDISYS_PREFIJO}/app && \
    ( npm run build && cp -r dist/. /out/srv${EDISYS_PREFIJO}/app/ && echo "app: construida" ) || \
    ( echo "app: NO compiló; se usa la página de reserva" && cp /reserva/index.html /out/srv${EDISYS_PREFIJO}/app/index.html )

FROM caddy:2-alpine
ARG EDISYS_PREFIJO=""
COPY edge/Caddyfile /etc/caddy/Caddyfile.tpl
RUN sed -e "s|@@PREFIJO@@|${EDISYS_PREFIJO}|g" \
        -e "s|@@STRIP@@|${EDISYS_PREFIJO:+uri strip_prefix ${EDISYS_PREFIJO}}|g" \
        /etc/caddy/Caddyfile.tpl > /etc/caddy/Caddyfile
COPY --from=app /out/srv /srv
EXPOSE 80
