#!/usr/bin/env bash
# restaurar.sh FECHA|ultimo — restaura el respaldo en una base TEMPORAL (edisys_restauracion por defecto) y
# compara el conteo de filas de cada tabla con el del respaldo. No toca la base en uso.
# Para restaurar SOBRE la base en uso: RESTAURAR_EN=edisys CONFIRMAR=si (detén antes el API).
set -euo pipefail
: "${DATABASE_URL:?falta DATABASE_URL}"
FECHA="${1:-ultimo}"
DIR="$RESPALDOS/$FECHA"
[ -f "$DIR/edisys.dump" ] || { echo "No hay respaldo en $DIR"; ls "$RESPALDOS"; exit 1; }
(cd "$DIR" && sha256sum -c --quiet SHA256SUMS) || { echo "Las sumas SHA-256 no cuadran: respaldo dañado"; exit 1; }
DESTINO="${RESTAURAR_EN:-edisys_restauracion}"
BASE_URL="${DATABASE_URL%/*}"
QUERY=""; case "$DATABASE_URL" in *\?*) QUERY="?${DATABASE_URL#*\?}";; esac
DB_DEST="$BASE_URL/$DESTINO$QUERY"
ADMIN="$BASE_URL/postgres$QUERY"
if [ "$DESTINO" = "$(basename "${DATABASE_URL%%\?*}")" ] && [ "${CONFIRMAR:-}" != "si" ]; then
  echo "Restaurar sobre la base en uso exige CONFIRMAR=si"; exit 1
fi
echo "Restaurando $FECHA en la base «$DESTINO»…"
psql "$ADMIN" -q -c "DROP DATABASE IF EXISTS \"$DESTINO\" WITH (FORCE)" -c "CREATE DATABASE \"$DESTINO\""
pg_restore --no-owner --exit-on-error --dbname="$DB_DEST" "$DIR/edisys.dump"
fallas=0; tablas=0
while read -r tabla n; do
  [ -z "$tabla" ] && continue
  tablas=$((tablas+1))
  m=$(psql "$DB_DEST" -At -c "SELECT count(*) FROM \"$tabla\"")
  if [ "$m" != "$n" ]; then echo "  DIFERENTE $tabla: respaldo $n, restaurado $m"; fallas=$((fallas+1)); fi
done < "$DIR/conteos.txt"
if [ "$fallas" -eq 0 ]; then
  echo "Restauración OK: $tablas tablas con el mismo conteo de filas ($(awk '{s+=$2} END {print s}' "$DIR/conteos.txt") filas)."
else
  echo "Restauración con $fallas diferencias"; exit 1
fi
[ "${CONSERVAR:-no}" = "si" ] || [ "$DESTINO" != "edisys_restauracion" ] || psql "$ADMIN" -q -c "DROP DATABASE \"$DESTINO\" WITH (FORCE)"
