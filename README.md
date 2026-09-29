# EDISYS

Software de administración de edificios (Perú): cuotas, recibos, balance por nodos, reservas, medidores, mantenimiento, WhatsApp y analítica.
Guía: [`docs/PASO_A_PASO_INTERFACES.md`](docs/PASO_A_PASO_INTERFACES.md) · datos de la demo: [`design/DISENO.md`](design/DISENO.md).

## Levantar en local

```bash
cp .env.example .env      # opcional: los valores por defecto sirven para local
make up                   # docker compose up -d --build
open http://localhost:4700/login/
```

Usuarios de demostración (clave **`Demo2026!`**): `admin@demo.pe` (administrador), `junta@demo.pe` (junta; también `junta2..5@demo.pe`),
`propietario201@demo.pe` (María Demo, Dpto 201, 51900000201; también entra con `201-40000201`), `inquilino@demo.pe` (Dpto 302),
`operario@demo.pe`, `tecnico@demo.pe`, `supervisor@demo.pe` (superadmin).

| Servicio | Contenedor | Puerto host | Qué es |
|---|---|---|---|
| edge | `edisys_edge` | **4700** | Caddy (sin TLS en local): `/` → 302 `/app/`, `/app/*` estático, `/login*` → login, `/api/*` → api |
| api | `edisys_api` | 4710 | Go (`edisys serve`) |
| login | `edisys_login` | 4730 | Qwik City (pantalla 01) |
| postgres | `edisys_postgres` | 4754 | PostgreSQL 16 |
| s3 | `edisys_s3` | 4790 (S3) · 4791 (admin) | Garage v2 de un nodo |
| migrate | `edisys_migrate` | — | `edisys preparar`: migra y siembra si la base está vacía; termina |
| mailpit | `edisys_mailpit` | 4725 (SMTP) · **4726** (web) | Buzón local: atrapa todo el correo del API |

S3: MinIO ya no publica imágenes libres, así que en local se usa **Garage** (lo mismo que el EC2). El API lo inicializa solo
(layout, clave y cubo privado) por el API de administración de Garage. Los archivos se sirven con URL firmada de 10 min en
`/api/v1/archivos/{id}`; el cubo nunca se publica.

## Comandos

```bash
make seed        # borra y vuelve a sembrar el Edificio Demo (6 meses: abril–setiembre 2026)
make seed-demo   # igual, pero deja 2 lecturas de setiembre pendientes para tomarlas en vivo
make test        # go test ./... (puras + integración contra la base edisys_test del compose)
make smoke       # humo con curl contra http://localhost:4700
make logs / make down
```

Binario: `edisys serve | migrate | seed [--pendientes=N] | preparar | salud`.

## WhatsApp

`WHATSAPP_MODO=simulado` (por defecto): nada sale; los mensajes quedan en `whatsapp_mensaje` con estado `simulado`.
Solo con `WHATSAPP_MODO=evolution` en el servidor **y** el edificio configurado en `evolution` se envía de verdad
(`POST {EVOLUTION_URL}/message/sendText/{instancia}` con `apikey`). La clave nunca se devuelve por el API.
Webhook de entrada: `POST /api/v1/whatsapp/webhook` (con `?token=` si defines `WHATSAPP_WEBHOOK_TOKEN`).

## Correo

`CORREO_MODO=smtp` en el compose local: el API entrega a **Mailpit** (`SMTP_HOST=mailpit`, puerto 1025 interno / 4725 en el host)
y los correos se ven en **http://localhost:4726**. Nada sale a terceros. Con `CORREO_MODO=simulado` (por defecto fuera del compose)
no se envía nada: los mensajes quedan en la bandeja `correo_mensaje` con estado `simulado`. `SMTP_USUARIO`/`SMTP_CLAVE` solo
para un SMTP real; la clave nunca se registra ni se devuelve.

- `GET /edificios/{eid}/balance/{AAAA-MM}.pdf` y `…/balance/{AAAA-MM}/informe-junta.pdf`
- `POST /edificios/{eid}/recibos/{AAAA-MM}/enviar-correo`: cada propietario recibe su recibo en PDF.
- `POST /edificios/{eid}/balance/{AAAA-MM}/enviar-correo {destinatarios: todos|junta|propietarios}`: la junta recibe balance e informe; los propietarios, el balance.
- `GET /edificios/{eid}/correo/mensajes`: la bandeja.

## API

Todo bajo `/api/v1`, JSON en snake_case, dinero en céntimos, errores `{ "error": { "codigo", "mensaje", "campos"? } }`.
Con cookie, toda escritura exige la cabecera `X-EDISYS: 1` (CSRF); con `Authorization: Bearer` no.
Los módulos nuevos (`/whatsapp/*`, `/chatbot/mensaje`, `/analitica/resumen`, `/mantenimiento/incidencias`) usan el primer
edificio del usuario (o `?edificio_id=`), y también existen bajo `/edificios/{eid}/…`.
