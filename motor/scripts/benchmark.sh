#!/usr/bin/env bash
# F0 · Corre el benchmark de calidad + velocidad de los candidatos en el EC2 real.
#   ./scripts/benchmark.sh qwen25-1.5b     # uno solo
#   ./scripts/benchmark.sh todos           # los 3, uno por uno
# Deja resultados/<clave>.json y acumula resultados/RESUMEN.md.
set -euo pipefail
cd "$(dirname "$0")/.."

PUERTO="${LLAMA_PORT:-8081}"
BASE="http://127.0.0.1:${PUERTO}"

run() { # clave gguf mem_limit
  local clave=$1 gguf=$2 mem=$3
  local ruta="/models/$gguf"
  [ -s "models/$gguf" ] || { echo "Falta models/$gguf — corre: make descargar" >&2; exit 1; }

  echo "══ $clave ($gguf, mem_limit $mem) ══"
  MODELO_RUTA="$ruta" MEM_LLAMA="$mem" docker compose up -d --force-recreate llama >/dev/null

  echo "· esperando a que el modelo cargue (hasta 3 min)…"
  local i=0
  until curl -sf "$BASE/health" >/dev/null 2>&1; do
    i=$((i + 1)); [ "$i" -gt 90 ] && { echo "  el modelo no cargó en 180 s; revisa docker compose logs llama" >&2; exit 1; }
    sleep 2
  done

  docker compose --profile eval run --rm --no-deps evaluador \
    python3 /scripts/evaluar.py \
      --base "$BASE" --clave "$clave" --gguf "$gguf" \
      --rss "$(docker stats --no-stream --format '{{.MemUsage}}' edisys_motor_llama | awk '{print $1}')" \
      --salida "/resultados/${clave}.json" \
      --resumen /resultados/RESUMEN.md
  echo
}

case "${1:-}" in
  qwen25-1.5b) run qwen25-1.5b  qwen2.5-1.5b-instruct-q4_k_m.gguf 1450m ;;
  qwen3-1.7b)  run qwen3-1.7b   Qwen_Qwen3-1.7B-Q3_K_M.gguf     1350m ;;
  llama32-1b)  run llama32-1b   Llama-3.2-1B-Instruct-Q4_K_M.gguf 1250m ;;
  todos)
    run qwen25-1.5b qwen2.5-1.5b-instruct-q4_k_m.gguf    1450m
    run qwen3-1.7b  Qwen_Qwen3-1.7B-Q3_K_M.gguf          1350m
    run llama32-1b  Llama-3.2-1B-Instruct-Q4_K_M.gguf    1250m
    echo "· apagando llama-server (decisión del modelo es cosa tuya, no del servidor)"
    docker compose stop llama >/dev/null
    ;;
  *) echo "uso: $0 {qwen25-1.5b|qwen3-1.7b|llama32-1b|todos}" >&2; exit 1 ;;
esac
