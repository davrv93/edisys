# EDISYS · motor conversacional (F0)

Carpeta del motor conversacional local del plan
[`PLAN_TRABAJO_EDISYS_MOTOR_CONVERSACIONAL.md`](../PLAN_TRABAJO_EDISYS_MOTOR_CONVERSACIONAL.md).
En **F0** solo vive aquí el **benchmark de modelos**: elegir con números medidos
el LLM que quepa en el EC2 de 3 GB antes de escribir una línea del servicio.

```
motor/
├── docker-compose.yml     # llama-server (ghcr.io/ggml-org/llama.cpp:server) + evaluador (perfil eval)
├── .env.example           # MODELO_RUTA, MEM_LLAMA, LLAMA_CTX, LLAMA_THREADS, LLAMA_PORT
├── benchmark/preguntas.jsonl  # 20 preguntas de calidad con las cifras reales de la semilla
├── scripts/
│   ├── ec2-preparar.sh        # docker + swap 2 GB + sysctl (correr con sudo, una vez)
│   ├── descargar-modelos.sh   # los 3 GGUF + e5-small ONNX, con reanudación
│   ├── benchmark.sh           # levanta cada candidato y corre el evaluador
│   └── evaluar.py             # calidad (cifras, mojibake, evasivas) + latencia + /metrics
└── resultados/            # <clave>.json + RESUMEN.md (no entra al repo)
```

## Cómo correr la F0 (EC2 de 3 GB, Ubuntu 24.04)

```bash
sudo ./scripts/ec2-preparar.sh     # docker, swap de 2 GB, swappiness
make descargar                     # ~2,9 GB de GGUF + 32 MB de ONNX (reanuda si se corta)
make benchmark-todos               # los 3 candidatos, uno por uno
cat resultados/RESUMEN.md          # tabla comparativa → decisión del modelo
```

Un solo candidato: `make benchmark MODELO=qwen3-1.7b`.

## Qué mide

- **Calidad (20 preguntas del seed de EDISYS):** las cifras esperadas deben
  aparecer en la respuesta (`13,1` de morosidad, `34.120` del banco, la deuda
  de `1.420` del 402, el Yape `987 654 321`, el aforo `20`). Se comparan los
  dígitos, no el formato, porque el modelo puede separar miles distinto.
- **Banderas:** mojibake, respuesta evasiva («no lo sé» a algo que el corpus
  sí sabe) y heurística de «no contestó en español».
- **Velocidad:** latencia media y p95 de cada pregunta + `tok/s` de
  `/metrics` de llama-server (si el build los publica; si no, «n/d»).
- **RAM:** RSS real del contenedor de llama-server contra el techo de §1.

**Criterio del plan (F0):** ≥ 8 tok/s en el EC2 y tasa de cifras correcta; si
el elegido no llega, se baja a Qwen2.5-0.5B (ya contemplado en §1 del plan).

## Los 3 candidatos

| clave | modelo | cuantización | archivo | repo |
|---|---|---|---|---|
| `qwen25-1.5b` | Qwen2.5-1.5B-Instruct | Q4_K_M (~1,0 GB) | `qwen2.5-1.5b-instruct-q4_k_m.gguf` | Qwen (oficial) |
| `qwen3-1.7b` | Qwen3-1.7B | Q3_K_M (~1,0 GB) | `Qwen_Qwen3-1.7B-Q3_K_M.gguf` | bartowski (el oficial solo publica Q8_0 ≈ 1,9 GB: no cabe) |
| `llama32-1b` | Llama-3.2-1B-Instruct | Q4_K_M (~0,8 GB) | `Llama-3.2-1B-Instruct-Q4_K_M.gguf` | bartowski |

Embeddings (se bajan ya, se usan en F3): `multilingual-e5-small` ONNX int8 de
Xenova (`model_quantized.onnx` + `tokenizer.json`).

## Notas

- El puerto queda en `127.0.0.1:8081`: en F1 lo publica caddy con TLS y `MOTOR_TOKEN`.
- `llama-server` corre sin `--mlock` a propósito: en 3 GB conviene que el núcleo
  pueda reclamar las páginas del modelo mmap antes que un OOM.
- Las respuestas no llevan system prompt: la F0 mide el modelo pelado; el tono
  y el formato finos llegan con el QLoRA (F4).
- Todo el evaluador va con la biblioteca estándar de Python: el contenedor
  `python:3.12-slim` no instala nada en cada corrida.
