#!/usr/bin/env bash
# Humo de EDISYS contra el borde (http://localhost:4700). Usa cookie, como el navegador.
# WhatsApp está en simulado: no sale ningún mensaje real.
set -u
BASE="${BASE:-http://localhost:4700}"
JAR="$(mktemp)"; trap 'rm -f "$JAR"' EXIT
fallos=0
ok()  { printf '  OK   %s\n' "$1"; }
mal() { printf '  FALLA %s → %s\n' "$1" "$2"; fallos=$((fallos+1)); }
revisar() { # nombre, estado esperado, estado real, cuerpo, patrón
  if [ "$3" = "$2" ] && echo "$4" | grep -q -- "$5"; then ok "$1"; else mal "$1" "$3 $(echo "$4" | head -c 300)"; fi
}
pedir() { # método ruta [json]
  local m=$1 r=$2 d=${3:-}
  if [ -n "$d" ]; then
    curl -s -o /tmp/edisys_smoke_body -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$m" -H 'Content-Type: application/json' -H 'X-EDISYS: 1' -d "$d" "$BASE$r"
  else
    curl -s -o /tmp/edisys_smoke_body -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$m" -H 'X-EDISYS: 1' "$BASE$r"
  fi
}
echo "EDISYS · humo contra $BASE"
st=$(pedir GET /api/health); revisar "health" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"ok":true'
st=$(pedir POST /api/v1/auth/login '{"correo":"admin@demo.pe","clave":"Demo2026!"}'); revisar "login admin (cookie)" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"destino"'
grep -q edisys_at "$JAR" && ok "cookie edisys_at HttpOnly" || mal "cookie" "no llegó edisys_at"
st=$(pedir GET /api/v1/yo); revisar "/yo" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"rol":"administrador"'
st=$(pedir GET '/api/v1/edificios/1/dashboard?periodo=2026-09'); revisar "dashboard KPIs" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"saldo_cts":51000'
st=$(pedir GET '/api/v1/edificios/1/balance?periodo=2026-09'); revisar "balance (banco 34.120)" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"banco_cts":3412000'
st=$(pedir GET '/api/v1/edificios/1/lecturas?periodo=2026-09'); revisar "lecturas" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"avance"'
st=$(pedir POST '/api/v1/edificios/1/periodos/2026-09/reparto-medidores/calcular'); revisar "reparto 5.000/4.800/200" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"diferencia_cts":20000'
st=$(pedir GET '/api/v1/edificios/1/morosidad?periodo=2026-09'); revisar "morosidad del mes e histórica" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"historica":{'
st=$(pedir GET '/api/v1/edificios/1/ajustes?pendientes=1'); revisar "ajustes de lecturas corregidas" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"datos"'
st=$(curl -s -o /tmp/edisys_smoke_pdf -w '%{http_code}' -b "$JAR" "$BASE/api/v1/edificios/1/balance/2026-09.pdf"); [ "$st" = 200 ] && head -c 5 /tmp/edisys_smoke_pdf | grep -q '%PDF-' && ok "PDF del balance" || mal "PDF del balance" "$st"
st=$(curl -s -o /tmp/edisys_smoke_pdf -w '%{http_code}' -b "$JAR" "$BASE/api/v1/edificios/1/balance/2026-09/informe-junta.pdf"); [ "$st" = 200 ] && head -c 5 /tmp/edisys_smoke_pdf | grep -q '%PDF-' && ok "PDF del informe a la junta" || mal "PDF informe junta" "$st"
st=$(pedir GET '/api/v1/edificios/1/correo/mensajes'); revisar "bandeja de correo" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"modo"'
st=$(curl -s -o /dev/null -w '%{http_code}' "${MAILPIT:-http://localhost:4726}/api/v1/info"); [ "$st" = 200 ] && ok "Mailpit (4726)" || mal "Mailpit" "$st"
st=$(pedir GET '/api/v1/edificios/1/conciliacion?periodo=2026-09'); revisar "conciliación bancaria" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"saldo_sistema_cts"'
st=$(pedir GET '/api/v1/analitica/resumen?desde=2026-04&hasta=2026-09'); revisar "analítica" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"cobranza_mensual"'
st=$(pedir POST /api/v1/chatbot/mensaje '{"telefono":"51900000201","texto":"¿cuánto debo?"}'); revisar "chatbot «cuánto debo» (201)" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"intencion":"saldo"'
st=$(pedir POST /api/v1/whatsapp/enviar '{"telefono":"51900000201","plantilla":"libre","variables":{"texto":"Prueba de humo"}}'); revisar "whatsapp simulado" 201 "$st" "$(cat /tmp/edisys_smoke_body)" '"estado":"simulado"'
st=$(pedir GET /api/v1/whatsapp/config); revisar "whatsapp config sin apikey" 200 "$st" "$(cat /tmp/edisys_smoke_body)" '"tiene_apikey"'
st=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/login/"); [ "$st" = 200 ] && ok "login Qwik /login/" || mal "login Qwik" "$st"
st=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/app/"); [ "$st" = 200 ] && ok "app /app/" || mal "app" "$st"
st=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/"); [ "$st" = 302 ] && ok "/ → /app/ (302)" || mal "raíz" "$st"
st=$(pedir POST /api/v1/auth/logout); [ "$st" = 204 ] && ok "logout" || mal "logout" "$st"
echo; [ $fallos -eq 0 ] && echo "Todo OK" || echo "$fallos fallas"
exit $fallos
