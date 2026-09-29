#!/usr/bin/env bash
# F0 · Baja los 3 candidatos GGUF + e5-small a models/. Nombres verificados contra
# la API de Hugging Face (28-09-2026). Reanuda descargas cortadas (curl -C -).
#   qwen25-1.5b  Qwen2.5-1.5B-Instruct  Q4_K_M  ≈ 1,0 GB  (repo oficial Qwen)
#   qwen3-1.7b   Qwen3-1.7B             Q3_K_M  ≈ 1,0 GB  (bartowski; el oficial solo publica Q8_0 ≈ 1,9 GB, no cabe)
#   llama32-1b   Llama-3.2-1B-Instruct  Q4_K_M  ≈ 0,8 GB  (bartowski)
#   e5-small     multilingual-e5-small ONNX int8 (model_quantized.onnx) + tokenizer
set -euo pipefail
DEST="${1:-models}"
mkdir -p "$DEST"

bajar() { # destino url
  local d=$1 u=$2
  if [ -s "$d" ]; then echo "  ya está: $(basename "$d")"; return; fi
  echo "  ↓ $(basename "$d")"
  curl -fL --retry 3 -C - -o "$d.part" "$u" && mv "$d.part" "$d"
}

echo "· qwen25-1.5b (Q4_K_M, repo oficial Qwen)"
bajar "$DEST/qwen2.5-1.5b-instruct-q4_k_m.gguf" \
  "https://huggingface.co/Qwen/Qwen2.5-1.5B-Instruct-GGUF/resolve/main/qwen2.5-1.5b-instruct-q4_k_m.gguf"

echo "· qwen3-1.7b (Q3_K_M, bartowski)"
bajar "$DEST/Qwen_Qwen3-1.7B-Q3_K_M.gguf" \
  "https://huggingface.co/bartowski/Qwen_Qwen3-1.7B-GGUF/resolve/main/Qwen_Qwen3-1.7B-Q3_K_M.gguf"

echo "· llama32-1b (Q4_K_M, bartowski)"
bajar "$DEST/Llama-3.2-1B-Instruct-Q4_K_M.gguf" \
  "https://huggingface.co/bartowski/Llama-3.2-1B-Instruct-GGUF/resolve/main/Llama-3.2-1B-Instruct-Q4_K_M.gguf"

echo "· e5-small ONNX int8 + tokenizer (para la F3, se baja ahora y no se vuelve a tocar)"
bajar "$DEST/e5-model_quantized.onnx" \
  "https://huggingface.co/Xenova/multilingual-e5-small/resolve/main/onnx/model_quantized.onnx"
bajar "$DEST/e5-tokenizer.json" \
  "https://huggingface.co/Xenova/multilingual-e5-small/resolve/main/tokenizer.json"

echo "· modelos en $DEST:"
ls -lh "$DEST" | awk 'NR>1 {printf "  %s  %s\n", $5, $9}'
