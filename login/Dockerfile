# EDISYS · login (Qwik City, SSR en Node) — pantalla 01
# Construcción multi-stage: en el EC2 no se construye (§1.2); la imagen sale de CI.

FROM node:22-slim AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

FROM node:22-slim AS deps
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --omit=dev --no-audit --no-fund

FROM node:22-slim
ENV NODE_ENV=production \
    NODE_OPTIONS=--max-old-space-size=128 \
    PORT=3000 \
    HOST=0.0.0.0 \
    EDISYS_API_INTERNO=http://api:8080
WORKDIR /app
COPY --from=deps  /src/node_modules ./node_modules
COPY --from=build /src/server ./server
COPY --from=build /src/dist ./dist
COPY package.json ./
USER node
EXPOSE 3000
HEALTHCHECK --interval=60s --timeout=3s --start-period=10s --start-interval=5s \
  CMD node -e "fetch('http://127.0.0.1:3000/login/salud').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"
CMD ["node", "server/entry.node-server.js"]
