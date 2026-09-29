#!/usr/bin/env bash
# Programa el respaldo diario (hora de Lima, TZ=America/Lima) y deja crond en primer plano.
set -euo pipefail
HORA="${RESPALDO_CRON:-0 2 * * *}"
echo "$HORA /usr/local/bin/respaldar.sh > /proc/1/fd/1 2>&1" > /etc/crontabs/root
echo "EDISYS backup: respaldo programado «$HORA» ($(date +%Z)); copia externa: ${EXTERNO_REMOTO:-desactivada}"
exec crond -f -l 8
