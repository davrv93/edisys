#!/usr/bin/env bash
# Respaldo completo: base (pg_dump -Fc), conteo de filas por tabla, copia del cubo de archivos (Garage) y
# sumas SHA-256. Rota lo que tenga más de RETENCION_DIAS. Si EXTERNO_REMOTO está definido (un remoto de rclone
# configurado por variables RCLONE_CONFIG_<NOMBRE>_*), sube una copia fuera del servidor. Nunca imprime secretos.
set -euo pipefail
: "${DATABASE_URL:?falta DATABASE_URL}"
FECHA="${1:-$(date +%Y%m%d-%H%M)}"
DIR="$RESPALDOS/$FECHA"
mkdir -p "$DIR"
echo "[$(date -Iseconds)] respaldo $FECHA → $DIR"

pg_dump --format=custom --no-owner --dbname="$DATABASE_URL" --file="$DIR/edisys.dump"
psql "$DATABASE_URL" -X -At -F ' ' -v ON_ERROR_STOP=1 > "$DIR/conteos.txt" <<'SQL'
SELECT format('SELECT %L, count(*) FROM public.%I', table_name, table_name)
FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY table_name \gexec
SQL

if [ -n "${S3_BUCKET:-}" ] && [ -n "${RCLONE_CONFIG_GARAGE_ENDPOINT:-}" ]; then
  rclone copy "garage:$S3_BUCKET" "$DIR/archivos" --quiet --transfers 4
  echo "  archivos: $(find "$DIR/archivos" -type f 2>/dev/null | wc -l | tr -d ' ')"
fi
(cd "$DIR" && find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS)
ln -sfn "$FECHA" "$RESPALDOS/ultimo"
echo "  base: $(du -h "$DIR/edisys.dump" | cut -f1) · tablas: $(wc -l < "$DIR/conteos.txt" | tr -d ' ')"

# Rotación.
find "$RESPALDOS" -mindepth 1 -maxdepth 1 -type d -mtime +"${RETENCION_DIAS:-30}" -print -exec rm -rf {} + | sed 's/^/  rotado: /'

# Copia externa opcional (S3/R2). Desactivada si no hay EXTERNO_REMOTO.
if [ -n "${EXTERNO_REMOTO:-}" ]; then
  rclone copy "$DIR" "$EXTERNO_REMOTO:${EXTERNO_RUTA:-edisys-respaldos}/$FECHA" --quiet
  echo "  copia externa: $EXTERNO_REMOTO:${EXTERNO_RUTA:-edisys-respaldos}/$FECHA"
fi
echo "[$(date -Iseconds)] respaldo $FECHA listo"
