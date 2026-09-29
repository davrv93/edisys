# Contrato del motor conversacional (EDISYS)

Contrato HTTP y de archivos del servicio `motor`, fijado a partir de
`motor/app.py` (versión 1.1.0, con el WIP del 29-09-2026: caché de golden por
`(id, texto)`, desempate Jaccard y few-shot solo sin filas) y de su cliente
`api/internal/app/motor.go`. `motor-go/` lo cumple **byte a byte**; la
comprobación está en *Verificación* al final. **Excepción desde el 29-09-2026:**
`/v1/chat` verifica las cifras (ver *Verificación de cifras*): añade el campo
`verificacion`, dos partes al `system`, baja la temperatura a 0,1 y puede
llamar dos veces a llama. Python no lo hace.

Puerto `8080`. Sin autenticación: el API manda `Authorization: Bearer $MOTOR_TOKEN`
si está definido, pero el motor no lo valida (Python tampoco). Todas las
respuestas JSON salen como `JSONResponse` de Starlette: UTF-8 sin escapar
(`ensure_ascii=False`), separadores `,` y `:`, claves en el orden indicado.
El motor Go añade la cabecera `X-Motor: go` (única diferencia de cabeceras).

## Endpoints

### `GET /v1/salud`

Consulta `GET {LLAMA}/health` con 5 s de timeout. Siempre `200`.

```json
{"ok":true,"motor":"1.1.0","llama":{"estado":"ok"}}
{"ok":true,"motor":"1.1.0","llama":{"estado":"caido","error":"<≤120 caracteres>"}}
```

`estado` = `"ok"` si llama responde 200; `"http <código>"` para otro 2xx;
`"caido"` + `error` si no conecta o da no-2xx (`"HTTP Error 503: Service Unavailable"`).
El texto de `error` para fallos de red depende de la implementación (Python:
`<urlopen error …>`; Go: el error de `net/http`). El API solo mira el código 200.

### `POST /v1/chat`

Entrada:

| campo | tipo | defecto |
|---|---|---|
| `mensajes` | `[{"role": str, "content": str}]` | obligatorio |
| `pedir_sugerencias` | bool | `true` |
| `memoria` | bool | `true` |
| `datos` | objeto: `filas` (lo que sea, Go manda la lista de filas SQL) y/o `golden` (`[{"pregunta": …}]`) | `{}` |

Salida `200`:

```json
{"respuesta":"…","intencion":"golden|falla|conversa","sugerencias":["…"],
 "contexto_usado":[{"id":"faq_…","sim":0.84}],"tokens_generados":71,
 "verificacion":{"cifras":["402","S/ 1.420,00"],"ok":true,"reintento":false,
                 "seguro":false,"descartadas":[]}}
```

- `verificacion` (nuevo, solo motor Go; el API lo ignora porque decodifica
  en un struct con `respuesta`/`sugerencias`/`tokens_generados`):
  `cifras` = las de la respuesta final, todas presentes en las fuentes;
  `ok` = el LLM dio una respuesta verificada (a la primera o en el reintento);
  `reintento` = hubo segunda llamada; `seguro` = se respondió la versión segura
  (implica `ok=false`); `descartadas` = cifras inventadas que no salieron.

- `intencion`: `golden` si `datos.filas` es *truthy* (lista no vacía…), si no
  `falla` si hubo fragmentos de FAQ, si no `conversa`.
- `sim` = `round(x, 3)` (repr de Python: `1.0`, no `1`).
- `tokens_generados` = `usage.completion_tokens` de llama tal cual, o `null`.
- `sugerencias` = `[]` si `pedir_sugerencias` es falso.

Errores: `422 {"detail":"El último mensaje debe ser del usuario."}` (lista vacía o
último rol ≠ `user`), `422 {"detail":"Mensaje vacío."}` (tras `strip`),
`422 {"detail":"Mensaje demasiado largo (máximo 2000 caracteres)."}` (>2000
caracteres Unicode tras `strip`), `502 {"detail":"llama-server no respondió: …"}`,
`422 {"detail":[{type,loc,msg,input}]}` de validación (ver abajo).

Algoritmo (orden de `app.py`):

1. `consulta = strip(último.content)`; `_indexar()` (ver *Archivos*).
2. `q = e5("query: " + consulta)`. Fragmentos = top **2** del FAQ con
   `dot(q, vector) ≥ 0.55`, orden descendente estable.
3. Si `memoria`: correcciones con `confirmada` *truthy*, top **1** con sim ≥ **0.80**.
4. Prompt (`_ensamblar`), `system` = partes unidas por `\n\n`:
   - `SYSTEM` literal;
   - `"Contexto del edificio (puede estar vacío o no servir):\n"` + textos unidos por `\n---\n`;
   - `"Correcciones aprendidas de la administración:\n"` + `P: <texto>\nR: <respuesta>` unidas por `\n`;
   - si `datos.filas` truthy: `"Datos reales ya calculados … no estén aquí):\n"` +
     `json.dumps(filas, ensure_ascii=False)[:1800]` (separadores `, ` y `: `, orden
     de claves de entrada, floats con `repr`; corte en caracteres);
   - si no, y `datos.golden` truthy: `"Ejemplos de preguntas que SÍ sabes responder (con su consulta interna):\n"` +
     `P: <pregunta>` por línea + `"\nSi la pregunta del usuario se parece a alguna, …"`.
   - Historial: mensajes anteriores (sin el último) de más reciente a más
     antiguo mientras `Σlen(partes) + Σlen(content) ≤ 6000` caracteres (sin
     contar los `\n\n` del join); se para en el primero que no cabe. El último
     mensaje va como `{"role":"user","content":<content sin strip>}`.
5. (Go) Al `system` se le añade `"\n\n" + ReglasCifras`: solo cifras textuales de
   las fuentes, copiadas tal cual, sin sumar ni restar; aviso de que el
   «S/ 4.800,00» del SYSTEM es solo formato; los `*_cts` de `datos.filas` ya
   convertidos a soles por fila (`- codigo 402: saldo = S/ 1.420,00`) y la lista
   «Cifras disponibles» (máx. 40).
   `POST {LLAMA}/v1/chat/completions` con
   `{"messages":…,"temperature":0.1,"max_tokens":280,"cache_prompt":true}`
   (Python: 0.3), timeout 120 s. Respuesta = `strip(choices[0].message.content)`.
   No-2xx, error de red o JSON sin ese campo → 502.
6. Filtro KTO-lite: si para la clave `"|".join(sorted(str(id)))` de los
   fragmentos hay más votos `-1` que `+1`, quita las líneas (`splitlines`) que
   contengan alguna palabra (`[\s,;:]+`, ≥6 caracteres, en minúsculas) del texto
   de los fragmentos, salvo las de <8 caracteres; si queda vacío, deja el original.
7. Evasión aprendida: si hubo fragmentos, `r = e5("passage: " + respuesta)`; si
   ≥ **2** fragmentos tienen `dot(r, vector) > 0.93`, la respuesta pasa a ser el
   texto del primero.
7.bis (Go) Verificación de cifras sobre el texto ya filtrado (ver abajo). Si
   alguna cifra no está en las fuentes: se repiten 5–7 con `temperature 0.0` y
   el `system` + «ATENCIÓN: … cifras que NO están … las ÚNICAS cifras que puedes
   escribir son: …». Si vuelve a fallar (o llama falla en el reintento: no es
   502), versión segura. Nunca sale una cifra no verificada.
8. Sugerencias (ver `/v1/sugerencias`) con `usadas` = contenidos de todos los
   mensajes `user`.
9. Registro en memoria (máx. 100): `{"cuando","pregunta","respuesta"[:400],"intencion","claves","tokens"}`;
   si se respondió la versión segura, además `"motivo":"cifra_no_verificada","descartadas":[…]`
   (y una línea `cifra_no_verificada:` en el log del contenedor).

#### Verificación de cifras (Go, 29-09-2026)

- **Extractor** (`ExtraerCifras`): montos `S/ 1.420,00`, `S/1420`, `S/. 3.500`,
  `1,420.00`, `80 soles`, `4,8 mil`; porcentajes `13,1 %`; horas `8:00`;
  fechas `29/09`, `29/09/26`, `2026-09-29`; números sueltos. Ignora números
  pegados a letras (`e5`, `F5`) y viñetas (`1. `, `2) `). Rangos
  (`12:00-17:00`, `8-12`) y correlativos se parten por `-`. Un número con un
  solo separador seguido de 3 cifras (`1.500`, `1,420`) es ambiguo y se lee de
  las dos formas (miles o decimal).
- **Fuentes**: `datos.filas` (los campos `*_cts` cuentan **solo** divididos
  entre 100: `142000` no autoriza «S/ 142.000»), `datos.golden[].pregunta`,
  fragmentos del FAQ, memoria (P y R) y los mensajes `user` (identificadores de
  la pregunta: Dpto 402, 29/09). **No** cuentan el `SYSTEM` (su «S/ 4.800,00») ni
  los mensajes del asistente.
- **Comparación**: un número escrito con *d* decimales vale si alguna fuente
  redondeada a *d* decimales da lo mismo (`S/ 1.420` ← 1420,00; `4,8 mil` ←
  4 812,50; `13 %` ← 13,1). Horas: misma hh:mm, o `h:00` si la fuente trae `h`.
  Fechas: mismo día y mes (y año si ambos lo traen). No distingue tipos: un
  monto que coincida con un conteo de la fuente pasa.
- **Versión segura**: (1) si la única cifra mala es el único monto del texto y
  `datos.filas` trae un único monto → se sustituye por él (`S/ 1.420,00`);
  (2) si hay filas (≤ 8) → se narran sin LLM: «Esto es lo que tengo en los
  datos del edificio:\n- codigo: 402 · saldo: S/ 1.420,00»; (3) si no, se quitan
  las frases con cifras malas y se añade «No tengo ese monto a mano; revísalo en
  Recibos.» (o «No tengo ese dato a mano; escríbele a la administración.» si
  ninguna era un monto).

### `POST /v1/feedback`

Entrada: `pregunta` str, `respuesta` str, `respondio_bien` bool (obligatorios),
`admin` bool (defecto `false`). Salida `200 {"ok":true,"memoria":<estado>}`:

- `admin && respondio_bien && pregunta != "" && respuesta != ""` → guarda la
  corrección en `memoria.json` (reemplaza la de misma pregunta por
  `strip().lower()`, añade al final `{"texto","respuesta","confirmada":true,"cuando"}`
  con `strip`, recorta a las 50 últimas) y la vectoriza → `"confirmada"`.
- `!respondio_bien` → busca la última interacción con `pregunta` idéntica; si
  tenía fragmentos, voto `-1` a su clave y una propuesta
  `{"pregunta","respuesta","cuando"}` (máx. 50) → `"propuesta_pendiente"`
  (también si no la encuentra).
- resto → voto `+1` a la clave `""` → `"reforzado"`.

### `POST /v1/recuperar`

Entrada: `consulta` str, `candidatos` `[{"id": int, "pregunta": str, "sql": str = ""}]`
(obligatorios; `id` acepta entero, float entero o cadena numérica, como pydantic).
Salida `200 {"golden":[{"id":2,"pregunta":"…","sql":"…","sim":0.9}]}`:
`q = e5("query: " + strip(consulta))`; cada golden se vectoriza como
`"passage: " + pregunta` (caché en memoria por `(id, pregunta)`); orden
descendente estable por `sim + 0.03·jaccard(consulta, pregunta)`; top **3**;
de esos, solo los de `sim ≥ 0.45`. `jaccard` usa tokens `[a-z0-9]+` sobre
`lower()` menos una lista de stopwords (las vocales con tilde parten la palabra).

### `POST /v1/sugerencias`

Entrada: `consulta` str (`""`), `usadas` `[str]` (`[]`). Salida
`200 {"sugerencias":[…]}`. `consulta` vacía tras `strip` →
`["Hola","¿Cuánto debo?","Quiero reservar la parrilla"]`. Si no: el banco fijo de
10 preguntas ordenado por `dot(e5("passage: "+b), e5("query: "+consulta))`,
sin las que coincidan en `lower()` con alguna `usada` (`strip().lower()`), las 3 primeras.

### `GET /v1/registro`

`200 {"interacciones":[…],"propuestas":[…],"filtro":{"<clave>":{"+1":n,"-1":n}}}`
(las interacciones con versión segura llevan además `motivo` y `descartadas`)
(estado en memoria; se pierde al reiniciar, en las dos implementaciones).

### Rutas y errores genéricos

- Ruta desconocida: `404 {"detail":"Not Found"}`; método no admitido:
  `405 {"detail":"Method Not Allowed"}`; `/v1/salud/` → `307` a `/v1/salud`.
- Validación (pydantic v2, modo laxo): `422 {"detail":[{"type","loc","msg","input"}]}`
  con `loc` tipo `["body","mensajes",0,"content"]`. Tipos cubiertos: `missing`,
  `string_type`, `bool_parsing`, `list_type`, `dict_type`, `int_*`,
  `model_attributes_type`, `json_invalid`. Pydantic añade `ctx`/`url` en
  algunos casos que el motor Go no replica (el API no los lee).
- Excepción no prevista: `500 Internal Server Error` en texto plano.

## Archivos (`/motor/indice`)

- `faq.json`: lista de `{"id", "texto", "vector"?}`. Se aceptan claves extra y
  se conserva su orden.
- `memoria.json`: lista de `{"texto","respuesta","confirmada","cuando","vector"?}`.
- Formato de escritura: `json.dumps(x, ensure_ascii=False, indent=1)`, sin salto
  final; floats con `repr` de Python (`vector` = float32 convertidos a double).
- `_indexar()` (en cada `/v1/chat` y `/v1/feedback`): las entradas sin `vector`
  *truthy* de FAQ y memoria se vectorizan **en un solo lote** (FAQ primero) con
  `"passage: " + texto`, se les añade `vector` al final del objeto y se
  reescriben los dos archivos. Si no falta ninguno, no se escribe nada.
- `cuando`: `datetime.now(timezone.utc).isoformat(timespec="seconds")` →
  `2026-09-29T16:48:05+00:00`.

## Embeddings

`multilingual-e5-small` int8 (`/models/e5-model_quantized.onnx` +
`/models/e5-tokenizer.json`), entradas `input_ids`/`attention_mask`/`token_type_ids`
(ceros) → `last_hidden_state`, media con máscara y L2 en float32. Truncado a 128
tokens; en lote, relleno a la longitud mayor con `pad_id 0`. **El lote importa**:
la cuantización dinámica calcula la escala sobre todo el tensor, así que el
mismo texto da un vector algo distinto (coseno ≈ 0,997) en lote que suelto. El
motor Go usa los mismos lotes que Python.

## Verificación (29-09-2026)

- Tokenizador Go vs `tokenizers` 0.20 de Python: 3 126/3 126 textos con ids
  idénticos (`testdata/tokens_ref.json`).
- Vectores: bit a bit iguales a los de Python (5 referencias sueltas y en lote,
  y los 15 del `faq.json` real del volumen).
- `_ensamblar`, `_jaccard`, `_aplicar_filtro`: salida idéntica a Python
  (`testdata/py_ref.json`).
- Lado a lado con un llama de eco: 61/61 respuestas HTTP idénticas byte a byte
  (10 preguntas × recuperar/chat/chat+golden/sugerencias, historial largo con
  filas, memoria, 👎/👍, filtro, registro, 422/404/405), y `faq.json`/`memoria.json`
  resultantes idénticos — también reindexando desde la semilla sin vectores.
