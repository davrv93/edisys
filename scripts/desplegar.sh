#!/usr/bin/env bash
# Despliegue en el servidor (EC2): baja las imágenes publicadas, respalda la base, migra y levanta.
# Uso (en /opt/edisys, con su .env de producción):
#   EDISYS_REGISTRO=ghcr.io/<dueño>/edisys EDISYS_TAG=<sha> ./desplegar.sh
# No siembra datos (SEMBRAR=no) y no toca los volúmenes. Si algo falla, se detiene sin levantar a medias.
set -euo pipefail
: "${EDISYS_REGISTRO:?falta EDISYS_REGISTRO (p. ej. ghcr.io/dueño/edisys)}"
: "${EDISYS_TAG:?falta EDISYS_TAG (el sha publicado)}"
export EDISYS_REGISTRO EDISYS_TAG SEMBRAR=no
C="docker compose"
echo "== imágenes $EDISYS_REGISTRO:$EDISYS_TAG"
$C pull migrate login edge backup
echo "== respaldo previo"
$C up -d postgres s3
$C run --rm --no-deps backup respaldar.sh "predespliegue-$(date +%Y%m%d-%H%M)" || { echo "El respaldo previo falló: no se despliega."; exit 1; }
echo "== migraciones"
$C run --rm migrate preparar
echo "== servicios"
$C up -d --no-build --remove-orphans
for i in $(seq 1 40); do
  if curl -fs http://localhost:4700/api/health >/dev/null; then echo "== listo: $(curl -fs http://localhost:4700/api/health)"; exit 0; fi
  sleep 3
done
echo "El API no respondió a tiempo"; $C ps; exit 1
