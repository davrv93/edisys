#!/usr/bin/env bash
# F4 · Pipeline completo: dataset → QLoRA → fusión → GGUF Q4_K_M.
# Corre en la Mac. NUNCA en el servidor: allí solo llega el GGUF final.
#   ./entrenar-qlora.sh                # dataset por defecto
#   ./entrenar-qlora.sh otro.jsonl    # otro dataset en formato chat (messages)
set -euo pipefail
cd "$(dirname "$0")"
DATASET="${1:-dataset/edisys_chat.jsonl}"
[ -s "$DATASET" ] || { echo "No está $DATASET — corre antes: python3 construir_dataset.py" >&2; exit 1; }

echo "· dependencias (torch, transformers, peft, datasets)"
PY="./.venv/bin/python"
[ -x "$PY" ] || PY=python3   # venv dedicado si existe; si no, el python del sistema
"$PY" - <<'EOF'
import importlib.util, sys
faltan = [m for m in ("torch", "transformers", "peft", "datasets", "gguf") if importlib.util.find_spec(m) is None]
if faltan:
    print("Instalando:", " ".join(faltan))
    import subprocess
    subprocess.check_call([sys.executable, "-m", "pip", "install", "-q", *faltan])
EOF

echo "· F2 → QLoRA con $DATASET"
"$PY" entrenar.py "$DATASET"

echo "· conversión a GGUF y cuantización Q4_K_M"
LLAMA_DIR="${LLAMA_CPP_DIR:-$PWD/.llama-cpp}"
if [ ! -f "$LLAMA_DIR/convert_hf_to_gguf.py" ]; then
  echo "  (clono llama.cpp en $LLAMA_DIR, una sola vez)"
  git clone --depth 1 https://github.com/ggml-org/llama.cpp "$LLAMA_DIR"
fi
mkdir -p gguf
"$PY" "$LLAMA_DIR/convert_hf_to_gguf.py" fusionado --outfile gguf/edisys-f16.gguf --outtype f16
if ! which llama-quantize >/dev/null 2>&1 && [ ! -x "$LLAMA_DIR/build/bin/llama-quantize" ]; then
  if which brew >/dev/null 2>&1; then
    echo "· instalo llama.cpp con brew (trae llama-quantize; una sola vez)"
    brew install -q llama.cpp
  fi
fi
if ! which llama-quantize >/dev/null 2>&1 && [ ! -x "$LLAMA_DIR/build/bin/llama-quantize" ]; then
  echo "Falta llama-quantize: compílalo (cmake -B build && cmake --build build) o: brew install llama.cpp" >&2
  echo "El f16 quedó en gguf/edisys-f16.gguf; cuantiza después con:" >&2
  echo "  llama-quantize gguf/edisys-f16.gguf gguf/edisys-motor-v1.gguf Q4_K_M" >&2
  exit 0
fi
QT=$(which llama-quantize || echo "$LLAMA_DIR/build/bin/llama-quantize")
"$QT" gguf/edisys-f16.gguf gguf/edisys-motor-v1.gguf Q4_K_M
rm -f gguf/edisys-f16.gguf

echo "· listo: gguf/edisys-motor-v1.gguf (~1,0 GB)"
echo "  Pruébalo contra el base: MODELO_RUTA=\$PWD/gguf/edisys-motor-v1.gguf make -C .. benchmark MODELO=qwen25-1.5b"
echo "  Para producción: cp gguf/edisys-motor-v1.gguf ../models/ y ajusta MODELO_RUTA en ../.env"
