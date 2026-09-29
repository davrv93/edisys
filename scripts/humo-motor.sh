#!/usr/bin/env bash
# Humo del motor conversacional (F1): llama a la API con cookie de admin y prueba
# una pregunta que las reglas NO entienden → el motor debe responder (intencion=motor).
set -u
BASE="${BASE:-http://localhost:4700}"
JAR="$(mktemp)"; trap 'rm -f "$JAR"' EXIT
fallos=0
ok()  { printf '  OK   %s\n' "$1"; }
mal() { printf '  FALLA %s → %s\n' "$1" "$2"; fallos=$((fallos+1)); }

pedir() { # método ruta [json]
  local m=$1 r=$2 d=${3:-}
  if [ -n "$d" ]; then
    curl -s -o /tmp/edisys_motor_body -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$m" \
      -H 'Content-Type: application/json' -H 'X-EDISYS: 1' -d "$d" "$BASE$r"
  else
    curl -s -o /tmp/edisys_motor_body -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$m" -H 'X-EDISYS: 1' "$BASE$r"
  fi
}

st=$(pedir POST /api/v1/auth/login '{"correo":"admin@demo.pe","clave":"Demo2026!"}')
[ "$st" = 200 ] && ok "login admin" || mal "login admin" "$st $(head -c 200 /tmp/edisys_motor_body)"

st=$(pedir POST /api/v1/motor/consulta '{"pregunta":"Explícame el reparto de medidores"}')
cuerpo="$(cat /tmp/edisys_motor_body)"
case "$st" in
  200) ok "motor/consulta respondió" ;;
  503) ok "motor apagado (MOTOR_URL vacío): respuesta 503 MOTOR_NO_DISPONIBLE" ;;
  *)   mal "motor/consulta" "$st $(echo "$cuerpo" | head -c 300)" ;;
esac

echo; [ $fallos -eq 0 ] && echo "Humo del motor OK" || echo "$fallos fallas"
exit $fallos
