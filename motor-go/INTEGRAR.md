# Integrar el motor Go en el compose (pendiente de decisión del dueño)

Nada de esto está aplicado: el `docker-compose.yml` sigue construyendo `./motor`
(Python). El índice (`motor_indice`) es compatible en los dos sentidos, así que
el corte y la vuelta atrás son cambiar de imagen, sin migrar datos.

## Qué cambia en `docker-compose.yml`

Solo el servicio `motor` (el `llama` no se toca):

```diff
   motor:
     profiles: [motor]
-    build: ./motor
-    image: ${EDISYS_REGISTRO:-edisys}/motor:${EDISYS_TAG:-local}
+    build:
+      context: ./motor-go
+      additional_contexts:
+        indice: ./motor/indice      # semilla faq.json para volúmenes nuevos
+    image: ${EDISYS_REGISTRO:-edisys}/motor-go:${EDISYS_TAG:-local}
     container_name: edisys_motor
     <<: *logs
     environment:
       E5_RUTA: /models/e5-model_quantized.onnx
       E5_TOK: /models/e5-tokenizer.json
-      PYTHONUNBUFFERED: "1"
+      LLAMA_URL: http://llama:8080
     volumes:
       - ./motor/models:/models:ro
       - motor_indice:/motor/indice
-    mem_limit: 400m
+    mem_limit: 320m      # medido: 267 MB de pico, 185 MB en uso (Python: 442 / 384)
     depends_on: [llama]
     restart: unless-stopped
```

- `additional_contexts` pide Compose ≥ 2.17 (aquí hay 2.35). Sin él:
  `docker build -t edisys/motor-go --build-context indice=motor/indice motor-go`.
- Se usa otro nombre de imagen (`motor-go`) para que la Python siga en el disco
  y la vuelta atrás no requiera reconstruir.
- `mem_limit` puede quedarse en 400m; 320m deja ~50 MB sobre el pico medido.
- `MOTOR_URL` del API no cambia (`http://motor:8080`): mismo alias y puerto.
- El healthcheck va dentro de la imagen (`motor -salud`, cada 60 s).

## Corte

Cuando la otra sesión haya cerrado sus cambios en `motor/app.py` (si toca la
lógica, hay que portarla antes: ver «Si cambia app.py»):

```bash
docker compose --profile motor build motor               # compila y corre go vet + go test
docker run --rm --entrypoint sh edisys/motor:local -c true   # (solo comprueba que la Python sigue ahí)
docker compose --profile motor up -d motor               # recrea edisys_motor con la imagen Go
docker compose ps motor                                  # healthy en ~5 s (e5 carga en <1 s)
docker exec edisys_motor /usr/local/bin/motor -salud && echo ok
make humo-motor
```

Se pierde lo que Python tenga en memoria (registro, filtro 👍/👎, propuestas),
igual que en cualquier reinicio. `faq.json` y `memoria.json` siguen valiendo.

Cómo saber qué motor responde: la cabecera `X-Motor: go` de cualquier respuesta.

## Vuelta atrás

```bash
git checkout docker-compose.yml                          # o revertir el diff de arriba
docker compose --profile motor up -d motor               # vuelve la imagen Python (edisys/motor:local)
```

Las correcciones que el admin haya confirmado con el motor Go están en
`memoria.json` con el mismo formato y vectores idénticos: Python las lee tal cual.

## Si cambia `app.py`

El motor Go replica `app.py` tal como estaba el 29-09-2026 (con el WIP de esa
fecha). Si la otra sesión cambia la lógica (umbrales, prompt, golden…), hay que
llevar el cambio a `internal/motor/logica.go` y regenerar las referencias:

- `testdata/py_ref.json`, `e5_ref.json`, `tokens_ref.json`: se regeneran con los
  scripts de `verificacion/` (ver `verificacion/LEEME.txt`), en un contenedor
  aparte de la imagen Python — **nunca** con `docker exec` en `edisys_motor`:
  cargar otra copia del modelo ahí lo lleva al OOM (pasó el 29-09).
- Repetir la prueba lado a lado con el llama de eco (`verificacion/paridad.py`).

## Pruebas

```bash
cd motor-go
go test ./...                                            # lógica, contrato, pyjson (sin modelos)
docker build --target test -t edisys/motor-go:test --build-context indice=../motor/indice .
docker run --rm -v "$PWD/../motor/models:/models:ro" edisys/motor-go:test   # + e5 bit a bit y tokenizador
```

En macOS, las de e5 necesitan `ORT_LIB` apuntando a `libonnxruntime.1.19.2.dylib`
(`onnxruntime-osx-arm64-1.19.2.tgz` de las releases de Microsoft) y
`E5_RUTA`/`E5_TOK` con rutas absolutas. Los vectores solo son bit a bit iguales
en Linux (ORT usa otros núcleos en macOS: coseno ≈ 0,997).

## Versiones fijadas (no subir sin repetir la verificación)

- **onnxruntime 1.19.2** (la del Python) con `github.com/yalue/onnxruntime_go v1.12.1`.
  Con onnxruntime 1.29 los vectores ya no coinciden (coseno 0,998–0,9999 contra
  el mismo texto) y el `faq.json` existente quedaría ligeramente desalineado.
- Tokenizador propio en Go (`internal/e5/unigram.go`), no la librería de Rust:
  la de Rust (`daulet/tokenizers`) ocupaba ~300 MB y no cabía en 400 MB junto al
  modelo. Si se cambia de `e5-tokenizer.json`, `CargarUnigram` rechaza lo que no
  sabe replicar (otro normalizador, byte_fallback…) en vez de tokenizar distinto.
