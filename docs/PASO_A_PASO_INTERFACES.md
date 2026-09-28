# EDISYS — Guía paso a paso para implementar las interfaces

**Fecha:** 28-09-2026 · **Para:** el equipo que construye EDISYS · **Estado:** listo para empezar

**Qué es EDISYS:** software de administración de edificios para Perú que junta *property* (cuotas, recibos, morosidad, balance, reservas, junta) y *facility* (mantenimiento, medidores, operarios).

**Fuentes que mandan, en este orden:**
1. Esta guía, para el stack y el alcance del MVP de **133.33 h** (S/ 4,000 a S/ 30 la hora).
2. [`PLAN_TRABAJO_ADMINISTRACION_EDIFICIOS.md`](../../PLAN_TRABAJO_ADMINISTRACION_EDIFICIOS.md), para las fases y las notas del dueño (§0).
3. [`PLAN_REQUERIMIENTOS_ADMINISTRACION_EDIFICIOS.md`](../../PLAN_REQUERIMIENTOS_ADMINISTRACION_EDIFICIOS.md) y `~/Downloads/Requerimientos-Sistema-Edificios.md` (RF-01 a RF-20), para las reglas de negocio.
4. [`../design/DISENO.md`](../design/DISENO.md), para colores, tipografía y el lienzo de diseño.

> **Ojo, cambio frente al plan de trabajo.** El plan (§1, decisión 3, y §2) proponía montar EDISYS como un vertical de PjgFactSalud en Laravel. **Eso ya no vale.** El stack está fijado: backend en **Go**, login en **Qwik**, landing en **Astro**, app en **React + Tailwind**, todo en **docker compose** sobre un EC2 propio de 4 GB. De PjgFactSalud solo se copian ideas (el diálogo propio en vez de `alert`, los permisos por rol, Garage como S3), no código.

---

## Índice

0. [Reglas que no se negocian](#0-reglas-que-no-se-negocian)
1. [Monorepo y servicios del compose](#1-monorepo-y-servicios-del-compose)
2. [Base común](#2-base-común)
3. [Pantallas 01 a 11](#3-pantallas-01-a-11)
4. [Orden global y horas](#4-orden-global-de-implementación-y-horas)
5. [La demo de 8 horas](#5-la-demo-de-8-horas)

---

## 0. Reglas que no se negocian

1. **Dinero en céntimos enteros** (`bigint`), nunca `float`. S/ 5.000,00 se guarda como `500000`. El formato `S/ 5,000.00` se aplica solo al mostrar.
2. **Toda cifra económica tiene sustento:** una foto, un PDF, un voucher o un código de operación. Un egreso sin documento se puede guardar, pero sale marcado como «sin sustento» en el balance.
3. **El permiso se valida en Go.** Las guardas de React solo esconden botones. Si un endpoint no comprueba el rol, está mal aunque la pantalla no muestre el botón.
4. **Las reglas duras viven en la base:** la doble reserva y los votos duplicados se impiden con restricciones de PostgreSQL. La pantalla puede avisar antes, pero no es la que impide.
5. **Nada de `window.alert`, `window.confirm` ni `window.prompt`.** Usa el componente `Dialog` (§2.2).
6. **Fechas en UTC en la base (`timestamptz`)** y en hora de Lima (`America/Lima`) en pantalla. Los periodos se escriben `AAAA-MM` (ej. `2026-09`).
7. **Datos personales (Ley 29733):** las fotos y los vouchers van en un cubo **privado**; se sirven con URL firmada que caduca en 10 minutos. Los DNI no se muestran enteros en listados: `4512****`.
8. **Español de Perú en toda la interfaz**, con tuteo. Números con coma de miles y punto decimal (`S/ 4,800.00`), como en los recibos de Sedapal.

---

## 1. Monorepo y servicios del compose

### 1.1 Estructura de carpetas

```
edisys/
├── docker-compose.yml          # producción (EC2)
├── docker-compose.dev.yml      # sobrescritura para desarrollo local (hot reload, puertos abiertos)
├── .env.example                # todas las variables, sin valores secretos
├── Makefile                    # make dev | make test | make seed | make deploy
│
├── api/                        # Backend Go
│   ├── cmd/api/main.go         # servidor HTTP
│   ├── cmd/edisys/main.go      # CLI: migrate, seed, crear-admin, recalcular
│   ├── internal/
│   │   ├── auth/               # login, JWT, refresh, middleware de permisos
│   │   ├── edificio/           # ficha, unidades, personas, importación Excel
│   │   ├── cobranza/           # periodos, recibos, pagos, morosidad
│   │   ├── balance/            # árbol de nodos
│   │   ├── reserva/            # áreas, recursos, reservas
│   │   ├── medidor/            # medidores, lecturas, reparto
│   │   ├── mantenimiento/      # incidencias, trabajos, aprobaciones
│   │   ├── rol/                # roles, permisos, usuarios
│   │   ├── archivo/            # subida a S3 y URLs firmadas
│   │   └── plataforma/         # errores, paginación, dinero, fechas Lima
│   ├── migrations/             # SQL numerado (golang-migrate)
│   ├── seed/                   # datos de ejemplo de la demo (Edificio Los Olivos)
│   └── Dockerfile
│
├── login/                      # Qwik City (adaptador Node), solo /login
│   └── Dockerfile
├── landing/                    # Astro estático, solo la web pública
├── app/                        # React + Vite + Tailwind, PWA
│   ├── src/
│   │   ├── ui/                 # componentes compartidos (§2.2)
│   │   ├── layout/             # armazón responsivo, menú por rol
│   │   ├── lib/                # api.js, auth.js, dinero.js, fechas.js
│   │   └── pantallas/          # 03-dashboard/, 04-balance/, … 11-roles/
│   └── public/manifest.webmanifest
├── packages/
│   └── tokens/                 # preset de Tailwind compartido por app, login y landing
│       ├── tokens.css          # variables CSS (claro y oscuro)
│       └── tailwind-preset.js
├── edge/                       # Caddy: TLS + estáticos + proxy
│   ├── Caddyfile
│   └── Dockerfile              # multi-stage: construye landing y app, copia los dist
├── e2e/                        # Playwright: un spec por pantalla
├── design/                     # DISENO.md y exportes del lienzo
└── docs/                       # esta guía
```

**Por qué un solo repo:** los tokens de diseño, los tipos del API y los datos de la demo se comparten. Un cambio de color toca las tres apps en un solo commit.

### 1.2 Servicios del compose y memoria

El EC2 tiene **4 GB de RAM, 2 vCPU y 80 GB de disco**. Ubuntu y Docker se comen unos 500–600 MB. Los límites de abajo suman **1,6 GB**, así que queda más de 1,5 GB libre para picos, caché del sistema y el `pg_dump` de la noche.

| Servicio | Imagen | Qué sirve | Ruta pública | `mem_limit` | Ajustes que lo mantienen dentro |
|---|---|---|---|---|---|
| `edge` | Caddy 2 (con los `dist` dentro) | TLS automático, landing Astro, SPA React, proxy a `login` y `api` | `/`, `/app/*` | **64 MB** | Sin módulos extra; los estáticos van horneados en la imagen |
| `login` | Node 22 slim + Qwik City | Pantalla 01 con SSR | `/login/*` | **192 MB** | `NODE_OPTIONS=--max-old-space-size=128` |
| `api` | Go (binario estático, `distroless`) | API REST, PDF de recibos, tareas programadas | `/api/*` | **256 MB** | `GOMEMLIMIT=200MiB`; pool de BD de 15 conexiones |
| `postgres` | PostgreSQL 16 alpine | Base de datos | ninguna (red interna) | **768 MB** | `shared_buffers=192MB`, `work_mem=8MB`, `max_connections=40`, `effective_cache_size=1GB` |
| `garage` | Garage (S3 compatible) | Fotos de medidores, vouchers, informes, videos | ninguna (red interna) | **256 MB** | Un solo nodo, `replication_factor=1` |
| `backup` | Alpine + cron | `pg_dump` diario y copia del cubo a un S3 externo (R2) | ninguna | **64 MB** | Corre a las 02:00 de Lima; retención de 30 días |
| `migrate` | la misma imagen de `api` | `edisys migrate up` al desplegar y termina | ninguna | 128 MB | `restart: "no"`; no queda corriendo |

**Total en marcha: 1.600 MB** (sin contar `migrate`, que se apaga).

Reglas de operación para que quepa:
- **No se construyen imágenes en el EC2.** Un `vite build` pide casi 1 GB. Las imágenes se construyen en GitHub Actions, se suben a `ghcr.io` y el servidor solo hace `docker compose pull && docker compose up -d`.
- **Swap de 2 GB** (`/swapfile`) como colchón, no como memoria de trabajo.
- **Logs con tope:** `logging: { driver: json-file, options: { max-size: "10m", max-file: "3" } }` en todos los servicios.
- **Healthchecks cada 60 s** con `start_interval: 5s`. Cada 5 s gastan CPU de gusto en una máquina de 2 vCPU.
- **Disco:** con 100 unidades, 2 medidores y fotos de ~300 KB (la app las comprime antes de subir), las lecturas ocupan ~720 MB al año. Los 80 GB alcanzan para años; vigila solo los videos de mantenimiento (tope de 50 MB por video).

### 1.3 Rutas del Caddyfile

```
edisys.pe            → landing (Astro, estático)
edisys.pe/login/*    → login:3000 (Qwik)
edisys.pe/app/*      → SPA React (try_files → /app/index.html)
edisys.pe/api/*      → api:8080
```

Todo bajo **un mismo dominio**. Así la cookie de sesión la leen el login y el API sin CORS ni cookies de terceros. Garage **no** se publica: las subidas pasan por el API y las lecturas usan URLs firmadas que el API genera sobre `/api/v1/archivos/{clave}` (redirige a Garage por la red interna a través de `edge`).

### 1.4 Paso a paso del arranque (la «fase B»)

1. Crea el repo con la estructura de §1.1 y un `README` de 20 líneas: cómo levantar, cómo sembrar datos y cómo correr pruebas.
2. Escribe `docker-compose.yml` con los siete servicios y `docker-compose.dev.yml` que abra los puertos 5432, 8080, 3000 y 5173 y monte el código.
3. `api`: esqueleto con `chi` (o `net/http` 1.22+), `pgx`, `golang-migrate`, `slog`. Endpoint `GET /api/health` → `{ "ok": true, "version": "…", "db": true, "s3": true }`.
4. Primera migración: `administradora`, `edificio`, `usuario`, `rol`, `permiso`, `rol_permiso`, `usuario_edificio_rol`, `sesion_refresh`, `auditoria`.
5. `packages/tokens` con el preset de Tailwind (§2.1) y `app/` con Vite + React + Tailwind + `vite-plugin-pwa`.
6. `edge/Dockerfile` multi-stage: `landing` → `astro build`; `app` → `vite build --base=/app/`; copia ambos `dist` a Caddy.
7. Workflow de GitHub Actions: `go test ./...`, `vitest`, `esbuild` de las pantallas, construcción de imágenes y `ssh` al EC2 con `docker compose pull && up -d`.
8. **Listo cuando:** `make dev` levanta todo en local, `curl https://edisys.pe/api/health` devuelve `ok: true` en el EC2, y `docker stats` muestra cada contenedor por debajo de su límite.

**Horas del bloque B (base común, §1.4 y §2): 27** (Arquitectura/Docker 10 + Backend Go 6 + PostgreSQL 6 + Almacenamiento/backups 5). Ver la matriz de §4.1.

---

## 2. Base común

### 2.1 Tokens de diseño

Los tokens **se toman de [`EDISYS/design/DISENO.md`](../design/DISENO.md)**, que deja el equipo de diseño con la paleta, la tipografía y la URL del lienzo. Cuando se escribió esta guía ese archivo aún no existía; **no inventes colores**: copia los valores de DISENO.md a `packages/tokens/tokens.css` apenas esté.

Cómo se implementan, sea cual sea la paleta:

1. **Variables CSS semánticas en `:root`**, no colores sueltos en las clases. Nombres fijos que todo el equipo usa:

   | Token | Uso |
   |---|---|
   | `--color-fondo`, `--color-superficie`, `--color-borde` | Fondo de página, tarjetas, divisores |
   | `--color-texto`, `--color-texto-suave` | Texto principal y secundario |
   | `--color-primario`, `--color-primario-texto` | Botón principal, enlaces, foco |
   | `--color-exito`, `--color-alerta`, `--color-peligro`, `--color-info` | Estados: pagado, por vencer, moroso o crítico, informativo |
   | `--color-ingreso`, `--color-egreso` | Balance: siempre el mismo color para ingresos y para egresos |
   | `--radio`, `--sombra`, `--fuente-base`, `--fuente-titulos`, `--fuente-numeros` | Forma y tipografía (números en tabular) |

2. **Modo oscuro** redefiniendo las mismas variables bajo `@media (prefers-color-scheme: dark)` y bajo `[data-tema="oscuro"]`.
3. **Preset de Tailwind** (`packages/tokens/tailwind-preset.js`) que mapea cada variable: `bg-superficie`, `text-texto-suave`, `border-borde`, `bg-primario`… Así ninguna pantalla escribe `bg-blue-600`.
4. **Los tres front (Qwik, Astro, React) importan el mismo preset.** El login y la landing se ven de la misma familia que la app.
5. **Semáforo de estados** con un solo mapa en `app/src/ui/estados.js`: `pagado → exito`, `por_vencer → alerta`, `vencido → peligro`, etc. Ninguna pantalla decide colores de estado por su cuenta.
6. **Nunca un color como único portador del significado:** cada estado lleva también texto o icono (daltonismo, pantallas al sol en la azotea).

### 2.2 Componentes compartidos (`app/src/ui/`)

Constrúyelos **antes** de la primera pantalla. Cada uno con su historia mínima en una página `/app/_ui` (solo en desarrollo) para verlos juntos.

| Componente | Qué hace | Reglas |
|---|---|---|
| `Boton` | Variantes `primario`, `secundario`, `fantasma`, `peligro`; tamaños `sm`, `md`, `lg` | Estado `cargando` con spinner que **desactiva** el botón (evita doble envío de pagos y lecturas). Área táctil mínima de 44 × 44 px |
| `Campo` | Input con etiqueta, ayuda y error; variantes `texto`, `numero`, `dinero`, `fecha`, `select`, `textarea` | `dinero` escribe en soles y entrega céntimos enteros. `numero` usa `inputMode="decimal"` en móvil. El error sale debajo, en rojo y con texto |
| `Tabla` | Columnas, orden, paginación en el servidor, fila seleccionable | **En móvil (< 640 px) se convierte en lista de tarjetas**, no en tabla con scroll horizontal. Estados de carga (esqueleto), vacío y error incluidos |
| `TarjetaKPI` | Título, valor grande, variación vs. mes anterior, icono, enlace «ver detalle» | Números con fuente tabular; la variación dice si es buena o mala con texto, no solo con color |
| `NodoDesplegable` | Fila de árbol: nombre, total, porcentaje del padre, flecha | Carga perezosa de los hijos al abrir; accesible por teclado (`aria-expanded`, flechas); muestra «sin sustento» si el documento falta |
| `SubirFoto` | Abre la **cámara trasera** (`<input type="file" accept="image/*" capture="environment">`), previsualiza, comprime a JPEG ≤ 1600 px y ~300 KB, sube con barra de progreso | Reintenta 3 veces si falla la red; lee la hora de la toma (EXIF) si existe. (Etapa 2: guardar la foto pendiente en IndexedDB para no perderla al cerrar la app.) |
| `SubirArchivo` | PDF, imagen o video para vouchers e informes | Tope de 10 MB (PDF/imagen) y 50 MB (video); valida el tipo antes de subir |
| `Calendario` | Vista mes y semana; celdas con franjas ocupadas; selección de franja | En móvil, vista de **día con lista de franjas**. Recibe la disponibilidad del servidor, no la calcula |
| `Dialog` | `useDialog()` con `alert`, `confirm` y `prompt`, que devuelven promesa | Foco atrapado, cierre con Escape, apilable, `danger: true` pinta el botón de confirmar en rojo. **Reemplaza todo diálogo nativo** |
| `Toast` | `useToast()` para avisos que no piden respuesta | Se va solo a los 4 s; los errores se quedan hasta cerrarlos |
| `Insignia` | Pastilla de estado (`pagado`, `vencido`, `crítico`…) | Usa el mapa de `estados.js` |
| `Vacio` / `ErrorCarga` / `SinPermiso` | Los tres estados de pantalla estándar | `ErrorCarga` trae botón «Reintentar»; `SinPermiso` explica qué rol hace falta y a quién pedirlo |
| `SelectorPeriodo` | Mes y año (`2026-09`) con flechas | Por defecto, el periodo abierto del edificio |
| `SelectorEdificio` | Cambia de edificio en la cabecera | Solo aparece si el usuario tiene más de un edificio |

Ejemplo de uso del diálogo (así se confirma todo en EDISYS):

```jsx
const { dialog, dialogEl } = useDialog();
const ok = await dialog.confirm({
  title: '¿Emitir 10 recibos de setiembre?',
  text: 'Después de emitirlos ya no se editan; solo se anulan.',
});
if (!ok) return;
```

### 2.3 Layout responsivo (móvil primero)

- **Se diseña a 360 px y se agranda**, nunca al revés. Puntos de quiebre de Tailwind: `sm 640`, `md 768`, `lg 1024`, `xl 1280`.
- **Móvil (< 768 px):** cabecera con nombre del edificio y menú de usuario; **barra inferior de 4 o 5 pestañas** según el rol; contenido a una columna; acciones principales en un botón fijo abajo (por encima de la barra).
- **Escritorio (≥ 1024 px):** menú lateral plegable, cabecera con `SelectorEdificio` y `SelectorPeriodo`, contenido con ancho máximo de 1280 px.
- **Menú según rol**, generado desde `GET /api/v1/yo` (no hay menú escrito a mano por rol):

  | Rol | Pestañas en móvil |
  |---|---|
  | Administrador | Inicio (03) · Balance (04) · Recibos (05) · Mantenimiento (09) · Más |
  | Junta | Inicio · Balance · Aprobaciones (09) · Más |
  | Propietario / inquilino | Portal (10) · Recibos · Reservas (07) · Reportar (09) |
  | Operario | Lecturas (08) · Reportar (09) |
  | Técnico / proveedor | Mis trabajos (09) |

- **PWA:** `manifest.webmanifest` con nombre «EDISYS», iconos de 192 y 512 px, `display: standalone`, `start_url: /app/`. Service worker de `vite-plugin-pwa` en modo `generateSW` que cachea **solo los estáticos**; las respuestas del API no se cachean (un saldo viejo en pantalla es peor que un error). Aviso «Hay una versión nueva, toca para actualizar» con `Toast`.

### 2.4 Autenticación: Qwik → cookie/JWT → React

```
[Navegador] ──GET /login──► [login (Qwik)]
     │  POST /login (routeAction$ en el servidor de Qwik)
     │        └──► POST api:8080/api/v1/auth/login  {usuario, clave}
     │             ◄── {access_jwt (15 min), refresh (30 días), usuario}
     │  Qwik escribe dos cookies y redirige a /app/
     ▼
[Navegador] ──GET /app/──► [edge: SPA React] ──GET /api/v1/yo (con cookie)──► [api]
```

Paso a paso:

1. **Go — `POST /api/v1/auth/login`**: recibe `{ usuario, clave }`. El `usuario` es un correo **o** el código de la unidad más el DNI (RF: «cada departamento tiene su usuario y clave, o DNI»). Claves con `bcrypt` (costo 12). Devuelve el `access_jwt` (15 min), un `refresh` opaco (30 días, guardado con hash en `sesion_refresh`) y los datos del usuario.
2. **Límite de intentos:** 5 fallos en 15 min por usuario y por IP → `429` con `codigo: "DEMASIADOS_INTENTOS"` y los minutos de espera.
3. **Qwik escribe las cookies** en su `routeAction$` (del lado del servidor, así la clave nunca toca JavaScript del navegador):
   - `edisys_at` = JWT · `HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=900`
   - `edisys_rt` = refresh · `HttpOnly; Secure; SameSite=Strict; Path=/api/v1/auth; Max-Age=2592000`
   - Redirige a `/app/` o al `?volver=` que traía (solo rutas internas que empiecen con `/app/`).
4. **Contenido del JWT** (firmado HS256 con `JWT_SECRET` de 32+ bytes, o EdDSA si luego hay más servicios): `sub` (usuario_id), `adm` (administradora_id), `edf` (edificios permitidos), `ver` (versión de permisos), `exp`. **Los permisos no van en el token**: se cargan en `/yo` y en el middleware (con caché de 60 s por `ver`), así un cambio de rol surte efecto sin esperar 15 min.
5. **Go acepta el token** desde la cookie `edisys_at` o desde `Authorization: Bearer` (para pruebas y futuras integraciones).
6. **CSRF:** `SameSite=Lax` + el API exige la cabecera `X-EDISYS: 1` en todo `POST/PUT/PATCH/DELETE`. `lib/api.js` la pone siempre; un formulario de otro sitio no puede ponerla.
7. **React — `lib/api.js`**: `fetch` con `credentials: 'include'`. Ante un `401` llama una sola vez a `POST /api/v1/auth/refresh` (que rota el refresh y reescribe ambas cookies) y reintenta. Si el refresh también falla → `location = '/login?volver=' + ruta`.
8. **React — arranque:** `GET /api/v1/yo` → `{ usuario, roles_por_edificio, permisos, edificio_actual, menu }`. Se guarda en un contexto `SesionContext`. Mientras carga, pantalla de carga con el logo; nunca parpadea el menú equivocado.
9. **Cerrar sesión:** `POST /api/v1/auth/logout` invalida el refresh y borra las cookies; React redirige a `/login`.
10. **Edificio activo:** va en la ruta (`/app/e/:eid/...`) y el API lo recibe en la ruta (`/api/v1/edificios/{eid}/...`). El middleware de Go comprueba que `eid` está entre los edificios del usuario **y** de su administradora (aislamiento multi-administradora: una administradora nunca ve edificios de otra). Toda consulta SQL filtra por `edificio_id`.

### 2.5 Guardas por rol

**Roles (7):** `superadmin` (dueño de la plataforma SaaS), `administrador`, `junta`, `propietario`, `inquilino`, `operario` (conserje, limpieza, nocturno), `tecnico` (técnico o proveedor externo). Un usuario puede tener roles distintos en edificios distintos (`usuario_edificio_rol`).

**Permisos con nombre `modulo.accion`**, por ejemplo: `balance.ver`, `balance.ver_documentos`, `recibos.emitir`, `pagos.registrar`, `unidades.editar`, `unidades.importar`, `reservas.crear`, `reservas.administrar`, `lecturas.registrar`, `lecturas.aprobar_reparto`, `incidencias.reportar`, `incidencias.validar`, `trabajos.presupuestar`, `trabajos.aprobar`, `roles.administrar`.

- **En Go:** `r.With(requiere("recibos.emitir")).Post(...)`. Sin el permiso → `403` con `{ "error": { "codigo": "SIN_PERMISO", "permiso": "recibos.emitir" } }`.
- **Datos propios:** el propietario y el inquilino solo ven **sus** unidades. Eso no es un permiso, es un filtro: el repositorio agrega `AND unidad_id = ANY($unidades_del_usuario)` cuando el rol es `propietario` o `inquilino`. Prueba unitaria obligatoria: un propietario pidiendo el recibo de otra unidad recibe `404` (no `403`, para no confirmar que existe).
- **En React:** `<Guarda permiso="recibos.emitir">…</Guarda>` para botones y `<RutaProtegida permiso="balance.ver">` para pantallas. Sin permiso en una ruta → componente `SinPermiso`, nunca pantalla en blanco.

### 2.6 Formato de errores del API (igual en todos los endpoints)

```json
{ "error": { "codigo": "PARTICIPACION_NO_SUMA_100", "mensaje": "Las participaciones suman 99.50 %; deben sumar 100 %.", "campos": { "participacion_pct": "…" } } }
```

| HTTP | Cuándo | Qué hace la pantalla |
|---|---|---|
| `401` | Sin sesión o token vencido | Refresh automático; si falla, al login |
| `403` | Sin permiso, o regla que prohíbe (moroso) | `SinPermiso`, o `Dialog.alert` con el `mensaje` |
| `404` | No existe o no es suyo | `Vacio` con «No encontramos este recibo» |
| `409` | Conflicto: doble reserva, recibo ya emitido, voto repetido | `Dialog.alert` y recarga los datos |
| `422` | Validación | Errores debajo de cada `Campo` usando `campos` |
| `5xx` | Falla del servidor | `ErrorCarga` con «Reintentar»; se registra con `slog` y un id de petición que se muestra al usuario |

Listas paginadas: `?pagina=1&por_pagina=25` → `{ "datos": [...], "total": 132, "pagina": 1 }`.

---

## 3. Pantallas 01 a 11

Formato de cada pantalla: **objetivo · roles · dispositivo · endpoints · estados · reglas · criterios de aceptación · orden de construcción · horas.** Todas las rutas del API empiezan con `/api/v1`; `{eid}` es el id del edificio.

---

### 01 · Login (Qwik)

**Objetivo:** entrar rápido desde el celular o la PC y aterrizar en la pantalla que corresponde al rol.
**Roles:** todos. **Dispositivo:** móvil primero; también escritorio.
**Ruta:** `/login` (servicio `login`).

**Endpoints Go**

| Método | Ruta | Devuelve |
|---|---|---|
| `POST` | `/auth/login` | `200 { access_jwt, refresh, usuario: { id, nombre, roles_por_edificio } }` · `401 CREDENCIALES` · `423 CUENTA_BLOQUEADA` · `429 DEMASIADOS_INTENTOS` |
| `POST` | `/auth/refresh` | `200` con cookies nuevas · `401` |
| `POST` | `/auth/logout` | `204` |
| `POST` | `/auth/olvide-clave` | `202` **siempre** (no revela si el correo existe); envía un enlace que vence en 30 min |
| `POST` | `/auth/restablecer-clave` | `204` · `422 CLAVE_DEBIL` · `410 ENLACE_VENCIDO` |

**Estados:** formulario vacío con foco en «Usuario»; enviando (botón con spinner, campos bloqueados); error de credenciales («Usuario o clave incorrectos», sin decir cuál de los dos); bloqueado por intentos (cuenta regresiva en minutos); sin conexión («Revisa tu conexión e intenta otra vez»).

**Reglas y validaciones**
- Usuario: correo válido **o** formato `CODIGO-UNIDAD` + DNI de 8 dígitos (o RUC de 11).
- Clave: mínimo 8 caracteres al crearla; al entrar no se valida el largo (solo contra la base).
- Botón «Ver clave» (ojo) para el celular.
- Si el usuario tiene varios edificios, tras entrar va a `/app/` y React muestra un selector de edificio; el último elegido se recuerda en `localStorage` (con `try/catch`).
- Destino por rol: administrador y junta → 03; propietario e inquilino → 10; operario → 08; técnico → 09 (mis trabajos).
- Funciona **sin JavaScript** para el envío (formulario HTML que procesa la acción de Qwik): útil en celulares viejos.

**Criterios de aceptación**
- [ ] Con `admin@losolivos.pe / demo1234` aterrizas en el dashboard (03) en menos de 2 s en 4G.
- [ ] Con `101 + DNI` aterrizas en el portal del propietario (10).
- [ ] Al sexto intento fallido en 15 min recibes el aviso de espera y el API responde `429`.
- [ ] Las cookies salen `HttpOnly` y `Secure` (verificado en DevTools); `document.cookie` no muestra el token.
- [ ] Un `?volver=https://otro-sitio.com` se ignora y te manda a `/app/`.
- [ ] Lighthouse móvil ≥ 90 en rendimiento y accesibilidad.

**Recorte para el MVP de 133 h:** «Olvidé mi clave» (`/auth/olvide-clave` y `/auth/restablecer-clave`) pasa a la etapa 2; mientras tanto, el administrador reenvía la invitación desde 11 y el usuario fija una clave nueva con ese enlace.

**Orden de construcción:** (1) endpoint `/auth/login` con pruebas; (2) formulario Qwik con `routeAction$` y cookies; (3) `/auth/refresh` y `/auth/logout`; (4) límite de intentos.

**Horas: 6** (Login Qwik 4 + Backend Go/auth 2).

---

### 02 · Landing pública (Astro)

**Objetivo:** explicar EDISYS a administradoras y juntas, y captar contactos para demos.
**Roles:** público (sin sesión). **Dispositivo:** móvil y escritorio.
**Ruta:** `/` (estático servido por `edge`).

**Secciones:** portada con la propuesta («Property + facility en un solo sistema. Tu propietario ve en qué se gastó cada sol»); problema (Excel, capturas por WhatsApp, balances que nadie entiende); módulos (balance por nodos, recibos, reservas, medidores con foto, mantenimiento con aprobación de junta); cómo funciona en 3 pasos; precio de referencia (por confirmar con el dueño: ~S/ 100 al mes por edificio); formulario de contacto; pie con enlace a «Entrar» (`/login`), términos y política de datos personales.

**Endpoints Go**

| Método | Ruta | Devuelve |
|---|---|---|
| `POST` | `/publico/contacto` | `202` · `422` (campos) · `429` (más de 3 envíos por IP y hora) |

Cuerpo: `{ nombre, correo, celular, empresa, edificios_aprox, mensaje, sitio_web }`. `sitio_web` es un **campo trampa** oculto: si llega lleno, se responde `202` y se descarta.

**Estados:** envío en curso; enviado («Te escribimos en menos de 24 horas»); error con reintento. El formulario funciona con una isla de Astro mínima (o `<form>` puro con redirección).

**Reglas:** página 100 % estática (cero JS salvo la isla del formulario); imágenes en AVIF/WebP con `width` y `height`; metadatos Open Graph; `sitemap.xml` y `robots.txt`; sin rastreadores de terceros sin aviso de cookies.

**Criterios de aceptación**
- [ ] Lighthouse ≥ 95 en rendimiento, accesibilidad, buenas prácticas y SEO.
- [ ] Se ve bien a 360 px sin scroll horizontal.
- [ ] Un envío de contacto llega a la tabla `lead` y dispara un correo al dueño.
- [ ] Usa el mismo preset de tokens que la app (mismo primario, misma tipografía).

**Recorte para el MVP de 133 h:** una sola página con las secciones de arriba, sin blog ni páginas por módulo; los textos los pone el dueño.

**Orden:** (1) maqueta con los textos del dueño; (2) formulario y endpoint; (3) SEO y rendimiento. Va al final del MVP porque no bloquea a nadie.

**Horas: 3** (Astro, del módulo «Login Qwik + Astro»).

---

### 03 · Dashboard del administrador

**Objetivo:** en una sola pantalla, saber cómo está el edificio este mes y qué hay que hacer hoy.
**Roles:** administrador (completo), junta (solo lectura, sin accesos de acción). **Dispositivo:** escritorio primero, usable en móvil.
**Ruta:** `/app/e/:eid/inicio`.

**Contenido**
- **Fila de 4 KPIs** (los mismos 4 que verá el propietario en su vista ejecutiva): ingresos del mes, egresos del mes, saldo, índice de morosidad (% de unidades con deuda vencida y S/ adeudado).
- **Cobranza del periodo:** recibos emitidos, pagados, parciales y pendientes (barra apilada) y un enlace a 05.
- **Tareas de hoy:** lecturas pendientes del periodo (enlace a 08), vouchers por validar (05 y 07), incidencias por validar (09), trabajos esperando aprobación de la junta (09).
- **Mantenimiento del mes:** trabajos programados y su estado.
- **Ingresos por reservas** de parrillas y otras áreas (nota del dueño §0.1).
- **Próximas reservas** de los 7 días siguientes.

**Endpoints Go**

| Método | Ruta | Devuelve |
|---|---|---|
| `GET` | `/edificios/{eid}/dashboard?periodo=2026-09` | `{ kpis: { ingresos_cts, egresos_cts, saldo_cts, morosidad: { pct, unidades, monto_cts } }, variacion_vs_mes_anterior: {...}, cobranza: { emitidos, pagados, parciales, pendientes }, tareas: { lecturas_pendientes, vouchers_por_validar, incidencias_por_validar, aprobaciones_pendientes }, trabajos_mes: [...], ingresos_reservas_cts, proximas_reservas: [...] }` |

Un **solo endpoint** agregado (una ida al servidor en 4G). Las cifras salen de las mismas funciones que usa el balance (04): **prohibido** calcular el saldo dos veces con dos consultas distintas.

**Estados:** carga (esqueletos de las tarjetas); edificio nuevo sin datos («Aún no hay movimientos en setiembre. Empieza importando tus unidades» con botón a 06); error con reintento; sin permiso (roles distintos de admin y junta).

**Reglas**
- Cada KPI es un enlace: ingresos/egresos/saldo → 04 con el nodo abierto; morosidad → 05 filtrado por vencidos.
- La variación dice «+12 % vs. agosto» con flecha y texto; si no hay mes anterior, no se muestra.
- La junta ve las mismas cifras sin los botones de acción.

**Criterios de aceptación**
- [ ] Con los datos de la demo, el saldo del dashboard es **idéntico al céntimo** al del nodo raíz de 04 para el mismo periodo (prueba automática).
- [ ] Al cambiar el periodo se recargan todas las tarjetas con una sola petición.
- [ ] Respuesta del endpoint < 300 ms con 300 unidades y 12 meses de datos.
- [ ] En 360 px las tarjetas KPI se apilan de a 2 por fila y no hay scroll horizontal.

**Orden:** va **después** de 04, 05, 08 y 09 en el MVP, porque los resume. (1) endpoint agregado sobre las funciones de balance y cobranza; (2) tarjetas KPI; (3) bloque de tareas con contadores; (4) listas del mes. En la demo se hace una versión con KPIs y tareas sobre datos sembrados.

**Recorte para el MVP de 133 h:** entran los 4 KPIs, la cobranza del periodo y el bloque de tareas de hoy. «Mantenimiento del mes», «ingresos por reservas» y «próximas reservas» pasan a la etapa 2 (el administrador los ve en 04, 07 y 09).

**Horas: 4** (Balance por nodos 3 + PostgreSQL 1, por la consulta agregada).

---

### 04 · Balance por nodos (macro → micro)

**Objetivo:** que cualquier propietario entienda en qué se gastó cada sol, bajando de lo general al documento.
**Roles:** administrador, junta (todo); propietario (vista ejecutiva y nodos, documentos según configuración del edificio); inquilino (solo vista ejecutiva, si el propietario lo habilita). **Dispositivo:** móvil y escritorio.
**Ruta:** `/app/e/:eid/balance`.

**El árbol (nota del dueño §0.5)**

```
Edificio Los Olivos · setiembre 2026 ............. saldo S/ 3,150.00
├── Ingresos ...................................... S/ 21,450.00
│   ├── Cuotas de mantenimiento ................... S/ 18,000.00
│   │   ├── Recibo 2026-09-101 (pagado) .......... S/ 1,800.00  [voucher]
│   │   └── …
│   ├── Agua (reparto de medidores) .............. S/ 5,000.00 …
│   └── Alquiler de áreas (parrillas) ............ S/ 450.00
│       └── Reserva R-0412 Parrilla 2 ............ S/ 150.00   [voucher Yape]
└── Egresos ....................................... S/ 18,300.00
    ├── Administración ............................ S/ 12,000.00
    │   ├── Conserjería (proveedor X) ............ S/ 7,500.00
    │   │   └── Factura F001-2231 ................ [PDF]
    ├── Servicios básicos
    │   └── Sedapal setiembre .................... S/ 5,000.00 [foto del recibo]
    └── Mantenimiento
        └── Trabajo T-0031 cambio de bomba ....... S/ 1,300.00 [informe]
```

(Cifras solo ilustrativas; la demo usa las de §5.)

Niveles: **edificio → ingresos/egresos → rubro → concepto → documento**. Cada nodo muestra su total, su porcentaje dentro del padre y se despliega.

**Vista ejecutiva arriba:** las 4 `TarjetaKPI` de 03 (ingresos, egresos, saldo, morosidad). El árbol va debajo, **cerrado** por defecto con solo los dos primeros niveles abiertos. Resumen simple primero, detalle al desplegar (regla crítica del requerimiento).

**Endpoints Go**

| Método | Ruta | Devuelve |
|---|---|---|
| `GET` | `/edificios/{eid}/balance?periodo=2026-09` | Resumen: `{ kpis, raiz: { id, nombre, total_cts, hijos: [ {id, tipo:"ingresos", total_cts, tiene_hijos}, {…egresos} ] } }` |
| `GET` | `/edificios/{eid}/balance/nodos/{nodo_id}?periodo=2026-09` | Hijos de un nodo: `[{ id, tipo: rubro|concepto|documento, nombre, total_cts, pct_padre, tiene_hijos, sin_sustento }]` (carga perezosa) |
| `GET` | `/edificios/{eid}/balance/documentos/{doc_id}` | `{ tipo: foto|pdf|voucher, nombre, url_firmada (10 min), fecha, monto_cts, origen: recibo|reserva|trabajo|egreso }` |
| `POST` | `/edificios/{eid}/egresos` | Registrar egreso con rubro, concepto, monto y documento (multipart) → `201` |
| `GET` | `/edificios/{eid}/balance/exportar.pdf?periodo=2026-09` | PDF del balance con los niveles abiertos que pida `?nivel=3` |

Los `nodo_id` son estables y legibles (`ing`, `egr`, `egr.administracion`, `egr.administracion.conserjeria`) para poder compartir un enlace con un nodo abierto: `/app/e/1/balance?periodo=2026-09&abrir=egr.administracion`.

**Estados:** carga del resumen (esqueleto); carga de un nodo (spinner en la fila, el resto sigue usable); periodo sin movimientos (`Vacio`); nodo que falla (error **en la fila** con «Reintentar», no en toda la pantalla); documento sin permiso (el propietario ve «Documento disponible para la junta»).

**Reglas**
- **Invariante:** la suma de los hijos es igual al total del padre, al céntimo. Prueba unitaria sobre todos los nodos del periodo de la demo.
- Los ingresos cuentan lo **cobrado** (pagos), no lo emitido; lo emitido y no cobrado es morosidad. Díselo al usuario con una nota bajo el KPI.
- Egresos sin documento → etiqueta «sin sustento» en amarillo y cuentan en un indicador del dashboard.
- El PDF se genera en Go (librería de PDF nativa, p. ej. `maroto`), **no** con Chrome sin cabeza: no cabe en 4 GB junto a lo demás.
- La conciliación bancaria (subir extracto y cuadrar) queda **fuera del MVP** (ver §4.3); el árbol ya deja el campo `movimiento_banco_id` para cuando entre.

**Criterios de aceptación**
- [ ] Abrir «Egresos → Servicios básicos → Sedapal» muestra la foto del recibo de S/ 5,000.00 en un visor, sin salir de la pantalla.
- [ ] El total de «Ingresos» menos «Egresos» es igual al KPI «Saldo».
- [ ] Una reserva pagada aparece sola bajo «Alquiler de áreas» con su código.
- [ ] El enlace con `abrir=` abre directamente ese nodo.
- [ ] Todo el árbol se navega con teclado (Tab, flechas, Enter).
- [ ] Un propietario de la unidad 101 **no** ve los vouchers de otras unidades (sale el nodo con el total, sin documento).

**Recorte para el MVP de 133 h:** la exportación a PDF del balance (`/balance/exportar.pdf`) pasa a la etapa 2; el propietario lo ve en pantalla y el administrador puede imprimirlo desde el navegador.

**Orden:** (1) modelo `rubro`, `concepto`, `egreso`, `documento` y la función `ArbolBalance(eid, periodo)` con pruebas del invariante; (2) endpoints de resumen y de nodo; (3) `NodoDesplegable` y visor de documentos; (4) registro de egresos con documento.

**Horas: 11** (Balance por nodos 9 + PostgreSQL 2, por las consultas del árbol).

---

### 05 · Recibos y facturación por departamento

**Objetivo:** generar, emitir, enviar y cobrar el recibo mensual de cada unidad, con su desglose y su sustento.
**Roles:** administrador (todo); junta (ver); propietario e inquilino (ver y pagar **los suyos**, desde 10). **Dispositivo:** escritorio para el administrador; móvil para ver y pagar.
**Ruta:** `/app/e/:eid/recibos` y `/app/e/:eid/recibos/:rid`.

> **Recibo interno de mantenimiento, no comprobante SUNAT** (decisión 1 del plan). Las «boletas y facturas por departamento» del dueño se cubren así en el MVP. Si un edificio exige SUNAT, se añade después como integración (unas 20 h, fuera de las 133.33 h). El módulo «Facturación» de la cotización cubre el **documento por departamento**: correlativo, PDF, pagos con voucher, validación y envío por correo.

**Flujo del periodo**
1. **Abrir periodo** `2026-09` con la fecha de corte del edificio (1, 15 o 30).
2. **Presupuesto del mes** por rubro: administración, mantenimiento, servicios, fondo de contingencia.
3. **Generar borradores**: el motor de reparto calcula cada recibo (§ reglas).
4. **Revisar** la tabla: total por unidad, diferencias con el mes anterior resaltadas.
5. **Emitir**: numera `2026-09-101` / correlativo `R-000123`, congela los montos y genera el PDF.
6. **Enviar** por correo (WhatsApp queda para después; ver §4.3).
7. **Registrar pagos**: voucher + código de operación; pagos parciales permitidos.

**Líneas del recibo:** cuota de mantenimiento (por participación), fondo de contingencia, agua por consumo (desde 08, con **foto del medidor**), parte de áreas comunes (desde 08), reservas cargadas al recibo (desde 07), conceptos opcionales de la unidad (estacionamiento), multas, saldo anterior, total.

**Endpoints Go**

| Método | Ruta | Devuelve |
|---|---|---|
| `POST` | `/edificios/{eid}/periodos` | Abre periodo `{ periodo, fecha_corte }` → `201` · `409 PERIODO_EXISTE` |
| `PUT` | `/edificios/{eid}/periodos/{p}/presupuesto` | Guarda montos por rubro → `200` |
| `POST` | `/edificios/{eid}/periodos/{p}/recibos/generar` | Borradores: `{ recibos: [{ unidad, lineas, total_cts }], total_cts, advertencias }` |
| `POST` | `/edificios/{eid}/periodos/{p}/recibos/emitir` | `{ emitidos: 10 }` · `409 YA_EMITIDO` · `422 LECTURAS_PENDIENTES` (si el edificio cobra agua y faltan lecturas) |
| `GET` | `/edificios/{eid}/recibos?periodo=&estado=&unidad=&pagina=` | Lista paginada `{ numero, unidad, propietario, total_cts, pagado_cts, saldo_cts, estado, vence }` |
| `GET` | `/edificios/{eid}/recibos/{rid}` | Detalle con líneas, pagos y `foto_medidor_url` firmada |
| `GET` | `/edificios/{eid}/recibos/{rid}/pdf` | PDF |
| `POST` | `/edificios/{eid}/recibos/enviar` | `{ recibo_ids }` → `202 { en_cola: 10 }` |
| `POST` | `/edificios/{eid}/recibos/{rid}/pagos` | Multipart `{ monto_cts, medio: yape|transferencia|efectivo|deposito, codigo_operacion, fecha, voucher }` → `201` |
| `PATCH` | `/edificios/{eid}/pagos/{pid}` | Validar o rechazar un pago informado por el propietario `{ estado: validado|rechazado, motivo }` |
| `POST` | `/edificios/{eid}/recibos/{rid}/anular` | `{ motivo }` → `200` (solo sin pagos) |
| `GET` | `/edificios/{eid}/morosidad` | `{ indice_pct, monto_cts, unidades: [{ unidad, meses: [{ periodo, saldo_cts }], antiguedad_dias }] }` |

**Estados del recibo:** `borrador → emitido → pagado_parcial → pagado`; `anulado`. Estados de pantalla: sin periodo abierto (botón «Abrir setiembre»); generando (barra de progreso); borradores con advertencias (p. ej. «Unidad 204 sin propietario»); emitidos; error de envío por unidad (se reintenta solo esa).

**Reglas**
- **Motor de reparto** configurable por edificio: `participacion`, `partes_iguales`, `consumo`, `mixto` + conceptos opcionales por unidad. **Redondeo al céntimo con el método del mayor residuo**: la suma de los recibos es exactamente el presupuesto; el céntimo que sobra va a la unidad con mayor residuo, nunca «se pierde». Prueba unitaria contra el Excel real del piloto **cuando llegue** (decisión 9 del plan).
- Un recibo **emitido no se edita**: se anula (con motivo, si no tiene pagos) y se emite otro. Todo queda en `auditoria`.
- La deuda inicial se carga **mes por mes** (06) y aparece como «saldo anterior» con detalle desplegable, nunca como un número suelto.
- Un pago informado por el propietario con voucher entra como `pendiente_validacion` y **no** baja la deuda hasta que el administrador lo valida.
- El pago se aplica primero al periodo más antiguo (salvo que el administrador elija otro).
- Código de operación único por medio y fecha: repetirlo → `409 PAGO_DUPLICADO` (evita registrar dos veces el mismo Yape).
- **Moroso** = unidad con saldo de un recibo vencido hace más de `dias_gracia` (configurable, por defecto 15). Esta misma función la usan 07 (bloqueo de reservas) y 03.

**Criterios de aceptación**
- [ ] Con el presupuesto de la demo, la suma de los 10 recibos es igual al presupuesto al céntimo, incluso con participaciones que dejan residuos (prueba con participaciones 33.3333 % × 3).
- [ ] El recibo de la unidad 101 muestra la línea de agua **S/ 400.00**, la de agua común **S/ 18.00** y la foto de su medidor (caso de 08).
- [ ] Emitir dos veces el mismo periodo responde `409` y no duplica recibos.
- [ ] Un pago parcial de S/ 500 sobre S/ 1,800 deja el recibo en `pagado_parcial` con saldo S/ 1,300.00.
- [ ] El mismo código de operación registrado dos veces responde `409`.
- [ ] El PDF abre en el celular y cabe en una hoja A4.

**Orden:** (1) periodos y presupuesto; (2) motor de reparto con pruebas (lo más delicado: empieza aquí); (3) generar y emitir; (4) lista y detalle; (5) pagos con voucher y validación; (6) morosidad; (7) PDF y envío por correo.

**Horas: 25** (Cuotas, reparto y recibos 14 + Facturación 11). Las otras 2 h del módulo de cuotas van al portal (10), que muestra el recibo al propietario.

---

### 06 · Unidades y propietarios + importación Excel

**Objetivo:** cargar el edificio completo desde su Excel, sin registrar a nadie a mano, y mantener el historial de propietarios e inquilinos.
**Roles:** administrador (todo); junta (ver, sin DNI completo). **Dispositivo:** escritorio (la importación); móvil solo para consultar.
**Ruta:** `/app/e/:eid/unidades` e `/app/e/:eid/unidades/importar`.

**Contenido**
- Ficha del edificio (RF-01): nombre, dirección, distrito, fecha de corte, política de reparto, modo de cobro de reservas, días de gracia, manual de convivencia y reglamento (PDF o texto).
- Tabla de unidades: código, tipo (departamento, estacionamiento, depósito), participación %, propietario, inquilino, celular, estado de deuda.
- Detalle de unidad: personas con **historial** (desde/hasta), medidores asociados, conceptos opcionales, deuda inicial mes por mes.
- **Importación en 3 pasos:** descargar plantilla → subir Excel → revisar vista previa con errores por fila → confirmar.

**Plantilla Excel** (hoja `Padron`): `codigo`, `tipo`, `piso`, `participacion_pct`, `propietario_nombre`, `propietario_dni_ruc`, `propietario_correo`, `propietario_celular`, `alquilado (si/no)`, `inquilino_nombre`, `inquilino_dni`, `inquilino_celular`, `medidor_agua_serie`, `lectura_inicial_agua`. Hoja opcional `Deuda`: `codigo`, `periodo (AAAA-MM)`, `monto`.

**Endpoints Go**

| Método | Ruta | Devuelve |
|---|---|---|
| `GET` / `PUT` | `/edificios/{eid}` | Ficha del edificio |
| `GET` | `/edificios/{eid}/unidades?buscar=&pagina=` | Lista paginada |
| `POST` / `PUT` / `DELETE` | `/edificios/{eid}/unidades[/{uid}]` | CRUD (borrar solo si no tiene recibos; si tiene, se desactiva) |
| `GET` | `/edificios/{eid}/unidades/{uid}` | Detalle con `personas`, `historial`, `medidores`, `deuda_inicial` |
| `POST` | `/edificios/{eid}/unidades/{uid}/personas` | Asignar propietario o inquilino `{ persona, rol, desde }`; cierra al anterior con `hasta` |
| `GET` | `/edificios/{eid}/importaciones/plantilla.xlsx` | La plantilla con ejemplos y validaciones de Excel |
| `POST` | `/edificios/{eid}/importaciones` | Multipart con el `.xlsx` → `{ importacion_id, filas: 30, validas: 28, errores: [{ fila: 7, campo: "propietario_dni_ruc", mensaje: "DNI debe tener 8 dígitos" }], advertencias: [{ fila: 12, mensaje: "DNI repetido con la fila 3" }], suma_participacion_pct: "100.0000", vista_previa: [...] }` |
| `POST` | `/edificios/{eid}/importaciones/{iid}/confirmar` | `200 { unidades_creadas, personas_creadas, deudas_cargadas }` · `422` si hay errores bloqueantes |

La importación se procesa en el API con `excelize` **en streaming** (no carga todo el archivo en RAM) y se guarda en una tabla de staging; la confirmación aplica todo **en una sola transacción**: o entra el padrón completo o no entra nada.

**Estados:** edificio sin unidades (Vacio grande con los dos caminos: «Importar Excel» o «Agregar una unidad»); subiendo y validando; vista previa con errores (filas en rojo, el botón Confirmar desactivado y la razón escrita); importado (resumen y enlace a la tabla).

**Reglas y validaciones**
- **La participación suma 100 %** (con 4 decimales, tolerancia de 0.0001 %). Si no suma, es error **bloqueante** y el mensaje dice cuánto suma y cuánto falta.
- DNI: 8 dígitos; RUC: 11 dígitos que empiezan con 10 o 20. Celular peruano: 9 dígitos que empiezan con 9.
- **DNI repetido** → advertencia, no error (una persona puede tener dos departamentos).
- Código de unidad único por edificio.
- Correo con formato válido; si falta, el propietario entrará por código + DNI.
- Reimportar el mismo archivo no duplica: se identifica la unidad por `codigo` y se actualiza (con vista previa de lo que cambia).
- Deuda inicial: periodo `AAAA-MM` válido y no futuro; monto > 0.
- Cambiar de propietario cierra el anterior con fecha; **nunca se sobrescribe** (historial, RF-02).
- Al crear un propietario con correo se le crea usuario con rol `propietario` y se le envía una invitación (el envío real puede quedar para la entrega; en el MVP basta el enlace copiable).

**Criterios de aceptación**
- [ ] El Excel de ejemplo de 10 unidades entra completo en menos de 5 s y la tabla muestra las 10.
- [ ] Un Excel con participaciones que suman 99.5 % se rechaza con «Suman 99.5000 %, faltan 0.5000 %» y no crea nada.
- [ ] Un Excel de 300 filas se valida en menos de 10 s sin pasar el límite de 256 MB del API.
- [ ] La fila con DNI de 7 dígitos se marca en rojo con el mensaje exacto.
- [ ] Cambiar el propietario de la 101 deja visible al anterior en el historial con su fecha de salida.

**Recorte para el MVP de 133 h:** la ficha del edificio lleva el manual y el reglamento como PDF subido (sin editor de texto); la invitación al propietario es un enlace copiable, sin envío automático.

**Orden:** (1) ficha del edificio; (2) CRUD de unidades y personas; (3) plantilla; (4) validación con vista previa; (5) confirmación transaccional; (6) deuda inicial; (7) historial.

**Horas: 11** (Registro + Excel).

---

### 07 · Reservas (calendario admin + reserva del propietario con pago)

**Objetivo:** reservar parrillas, SUM, piscina, etc. sin dobles reservas, con cobro trazable que entra solo al balance.
**Roles:** administrador (configura áreas, ve el calendario completo, confirma pagos, reserva a nombre de cualquiera); propietario (reserva y paga); inquilino (reserva si el propietario lo habilita); operario (ve las reservas del día). **Dispositivo:** escritorio para el admin; **móvil** para el propietario.
**Rutas:** `/app/e/:eid/reservas` (calendario) y `/app/e/:eid/reservas/nueva`.

**Flujo del propietario (móvil)**
1. Elige el área (Parrillas) y el recurso (Parrilla 2), o «cualquiera libre».
2. Elige el día y la franja libre (el calendario solo muestra lo disponible).
3. Ve tarifa, qué incluye, horario límite y normas; marca «Acepto las normas».
4. Según el modo de cobro del edificio:
   - **Cargo al recibo:** la reserva queda `confirmada` y se suma al recibo del mes.
   - **Pago inmediato:** la reserva queda `pendiente_pago` y **retiene la franja 15 minutos**; ve el QR de Yape del edificio y sube el voucher + código de operación. El administrador valida el pago y la reserva pasa a `confirmada`.
5. Recibe su **código de reserva** (`R-0412`).

**Endpoints Go**

| Método | Ruta | Devuelve |
|---|---|---|
| `GET` / `POST` / `PUT` | `/edificios/{eid}/areas[/{aid}]` | Áreas con recursos, tarifa, horario, aforo, qué incluye, normas |
| `GET` | `/edificios/{eid}/disponibilidad?recurso=&desde=&hasta=` | `{ franjas: [{ recurso_id, inicio, fin, estado: libre|ocupada|retenida|fuera_de_horario }] }` |
| `POST` | `/edificios/{eid}/reservas` | `{ recurso_id, unidad_id, inicio, fin, acepta_normas }` → `201 { id, codigo, estado, total_cts, vence_retencion }` · `409 FRANJA_OCUPADA` · `403 MOROSO` (con monto y meses adeudados) · `422 FUERA_DE_HORARIO / AFORO` |
| `GET` | `/edificios/{eid}/reservas?desde=&hasta=&recurso=&unidad=` | Para el calendario |
| `POST` | `/edificios/{eid}/reservas/{rid}/pago` | Multipart `{ codigo_operacion, voucher }` → `201` |
| `PATCH` | `/edificios/{eid}/reservas/{rid}` | Admin: `{ estado: confirmada|cancelada|no_show, motivo }` |
| `DELETE` | `/edificios/{eid}/reservas/{rid}` | Cancela el propietario (según regla de anticipación) |

**Estados:** cargando calendario; sin áreas configuradas (admin: «Configura tu primera área»; propietario: «Tu edificio aún no tiene áreas reservables»); día sin franjas libres; **moroso** (pantalla que explica por qué no puede reservar, cuánto debe, y botón «Ver mi deuda» hacia 10 — no un error genérico); retención vencida («Se liberó la franja porque no llegó el pago»); conflicto `409` (Dialog «Alguien acaba de reservar esa franja» y recarga la disponibilidad).

**Reglas**
- **Doble reserva bloqueada en la base**, no en la pantalla:

  ```sql
  CREATE EXTENSION IF NOT EXISTS btree_gist;
  ALTER TABLE reserva ADD CONSTRAINT reserva_sin_cruce
    EXCLUDE USING gist (recurso_id WITH =, tstzrange(inicio, fin, '[)') WITH &&)
    WHERE (estado IN ('pendiente_pago', 'confirmada'));
  ```

  El API traduce la violación (`23P01`) a `409 FRANJA_OCUPADA`.
- **El moroso no reserva**: el API usa la misma función de morosidad de 05. El administrador puede forzar una reserva a nombre de un moroso con motivo (queda en `auditoria`).
- Las retenciones `pendiente_pago` vencidas se liberan con una tarea cada minuto dentro del `api` (`UPDATE … SET estado='vencida' WHERE vence_retencion < now()`).
- Horario, aforo y anticipación mínima/máxima (p. ej. hasta 30 días) configurables por área.
- La reserva pagada genera el ingreso en el balance (04) bajo «Alquiler de áreas → recurso», con el código y el voucher como documento. La cargada al recibo aparece como línea en 05.
- **Pasarela con tarjeta (Izipay/Niubiz) fuera del MVP**; el campo `medio` ya la contempla.

**Criterios de aceptación**
- [ ] **Prueba de concurrencia:** 20 peticiones simultáneas por la misma franja → exactamente 1 `201` y 19 `409`.
- [ ] Un propietario con deuda vencida recibe `403 MOROSO` y ve cuánto debe.
- [ ] Una reserva con voucher validado aparece en el balance del mes con su código, sin registrarla a mano.
- [ ] Una retención sin pago se libera a los 15 min y la franja vuelve a verse libre.
- [ ] En 360 px se puede reservar de principio a fin con el pulgar.

**Orden:** (1) tablas y restricción de exclusión con la prueba de concurrencia; (2) configuración de áreas y recursos; (3) disponibilidad; (4) crear reserva con reglas (moroso, horario); (5) calendario del admin; (6) flujo móvil del propietario; (7) pago con voucher y validación; (8) enlace al balance y al recibo.

**Recorte para el MVP de 133 h:** el calendario del administrador es una vista de semana en lista (no arrastrar y soltar); horario, aforo y anticipación usan valores por defecto editables en la ficha del área; la cancelación por el propietario pasa a la etapa 2 (la hace el administrador).

**Horas: 9** (Reservas 8 + PostgreSQL 1, por la restricción de exclusión y su prueba de concurrencia). Las otras 2 h del módulo de reservas van al portal (10).

---

### 08 · Lectura de medidores del operario

**Objetivo:** que el conserje tome la foto de cada medidor desde el celular en la fecha de corte y que el sistema reparta solo el recibo general.
**Roles:** operario (registra lecturas); administrador (registra el recibo general, revisa alertas, aprueba el reparto); propietario (ve su foto y su consumo en el recibo). **Dispositivo:** **celular del operario** (pantalla de un solo paso: foto y listo); escritorio para el reparto.
**Rutas:** `/app/e/:eid/lecturas` (operario) y `/app/e/:eid/lecturas/reparto` (admin).

**Pantalla del operario**
- Arriba: periodo y tipo (Agua / Energía) y el avance: «7 de 10 leídas».
- Lista de unidades en el orden de la ronda (piso por piso), con estado: pendiente, leída, con alerta.
- Al tocar una unidad: botón grande **«Tomar foto»** (abre la cámara trasera) → la foto aparece arriba y **debajo** el campo «Lectura (m³)» con teclado numérico, junto a la lectura anterior como referencia → «Guardar y siguiente».
- Sin escribir nada más. Nada de menús intermedios.

**Caso del dueño (prueba automática obligatoria)**

Datos de la demo, edificio Los Olivos, 10 departamentos, agua de setiembre 2026:

| Concepto | Valor |
|---|---|
| Recibo general de Sedapal | **S/ 5,000.00** por **1,000 m³** |
| Tarifa efectiva | 5,000 ÷ 1,000 = **S/ 5.00 por m³** |
| Consumo de los 10 departamentos (suma de sus medidores) | **960 m³ → S/ 4,800.00** |
| Diferencia (riego, limpieza) = agua común | 40 m³ → **S/ 200.00** |
| Reparto de los S/ 200 | **por participación** |

| Unidad | Participación | Consumo | Agua propia | Agua común | Total agua |
|---|---|---|---|---|---|
| 101 | 9 % | 80 m³ | 400.00 | 18.00 | **418.00** |
| 102 | 9 % | 85 m³ | 425.00 | 18.00 | 443.00 |
| 103 | 9 % | 90 m³ | 450.00 | 18.00 | 468.00 |
| 104 | 9 % | 95 m³ | 475.00 | 18.00 | 493.00 |
| 105 | 9 % | 70 m³ | 350.00 | 18.00 | 368.00 |
| 201 | 11 % | 100 m³ | 500.00 | 22.00 | 522.00 |
| 202 | 11 % | 110 m³ | 550.00 | 22.00 | 572.00 |
| 203 | 11 % | 105 m³ | 525.00 | 22.00 | 547.00 |
| 204 | 11 % | 115 m³ | 575.00 | 22.00 | 597.00 |
| 205 | 11 % | 110 m³ | 550.00 | 22.00 | 572.00 |
| **Total** | **100 %** | **960 m³** | **4,800.00** | **200.00** | **5,000.00** |

La misma lógica sirve para **energía** de áreas comunes (el módulo es de *medidores*, no solo de agua; nota §0.3). La luz y el gas propios de cada departamento no entran (RF-09).

**Endpoints Go**

| Método | Ruta | Devuelve |
|---|---|---|
| `GET` | `/edificios/{eid}/lecturas?periodo=2026-09&tipo=agua` | `{ avance: { leidas: 7, total: 10 }, medidores: [{ medidor_id, unidad, orden_ronda, lectura_anterior, lectura_actual, foto_url, estado: pendiente|leida|alerta }] }` |
| `POST` | `/edificios/{eid}/medidores/{mid}/lecturas` | Multipart `{ periodo, valor, foto (obligatoria), tomada_en }` → `201 { consumo, alerta: null|"NEGATIVO"|"PICO" }` · `422 FOTO_OBLIGATORIA` · `409 YA_LEIDO` (corregir usa `PUT`) |
| `PUT` | `/edificios/{eid}/lecturas/{lid}` | Corrección del administrador con motivo (queda en `auditoria`; la foto original no se borra) |
| `POST` | `/edificios/{eid}/periodos/{p}/recibo-general` | Multipart `{ tipo, monto_cts, consumo_total, foto_recibo }` → `201` |
| `POST` | `/edificios/{eid}/periodos/{p}/reparto-medidores/calcular?tipo=agua` | Vista previa: `{ tarifa_cts_x_1000, total_unidades_cts, diferencia_cts, lineas: [{ unidad, consumo, propio_cts, comun_cts, total_cts }], alertas }` |
| `POST` | `/edificios/{eid}/periodos/{p}/reparto-medidores/aprobar?tipo=agua` | Escribe las líneas en los recibos borrador de 05 → `200` · `422 DIFERENCIA_NEGATIVA` · `422 LECTURAS_PENDIENTES` |

**Estados:** lista cargando; ronda terminada («¡Listo! 10 de 10 leídas» y botón para salir); **sin señal** (en el MVP: mensaje claro «No se pudo subir, se reintentará» y 3 reintentos mientras la app siga abierta; la cola persistente en IndexedDB para sótanos sin señal pasa a la etapa 2); cámara sin permiso (instrucciones para habilitarla en Android e iOS); alerta de consumo (el campo se pinta ámbar y pide confirmar con `Dialog.confirm`, no bloquea).

**Reglas**
- **Foto obligatoria:** el botón Guardar está desactivado sin foto **y** el API responde `422` si no llega la foto. Las dos cosas.
- Consumo = actual − anterior. **Negativo** → alerta y requiere confirmación con motivo (cambio de medidor, vuelta de contador).
- **Pico:** consumo > 2 × la media de los últimos 3 meses de esa unidad → alerta ámbar (posible fuga), no bloquea.
- La **lectura inicial** se toma al incorporar el edificio (viene del Excel de 06 o de una primera ronda con foto).
- **Diferencia negativa** (los departamentos suman más que el general) → no se reparte nada negativo: `422 DIFERENCIA_NEGATIVA` y el administrador debe revisar lecturas o registrar un ajuste con motivo.
- **Redondeo:** tarifa efectiva con precisión de milésimas de céntimo; cada línea se redondea al céntimo y la diferencia de redondeo se asigna por mayor residuo para que la suma sea exactamente el recibo general.
- La foto de cada lectura se guarda con `operario_id`, hora de toma y hora de subida (sustento; RF-09).
- El reparto aprobado genera las líneas «Agua (consumo)» y «Agua común» en el recibo (05); la foto aparece en el recibo del propietario (10).
- La tarifa efectiva promedio (total ÷ m³) es el método por defecto. Si el piloto pide tramos como los de Sedapal, se agrega como política después.

**Criterios de aceptación**
- [ ] **Prueba unitaria con el caso del dueño:** 5,000 / 4,800 / 200 → la tabla de arriba exacta al céntimo, total 5,000.00.
- [ ] Prueba con participaciones 33.3333 % × 3 y diferencia de S/ 100: las tres partes suman exactamente 100.00.
- [ ] Intentar guardar sin foto: el botón está desactivado; un `curl` sin foto recibe `422 FOTO_OBLIGATORIA`.
- [ ] Una ronda de 10 unidades se hace en menos de 5 minutos en un celular Android de gama media.
- [ ] Si la subida falla, la lectura no se da por guardada y el operario ve el reintento; un reintento que sí llega no crea una lectura duplicada (idempotencia por `medidor_id + periodo`).
- [ ] El recibo de la 101 muestra su foto, 80 m³ y S/ 418.00 de agua.

**Orden:** (1) función de reparto pura en Go con las pruebas del caso del dueño (**antes que cualquier pantalla**); (2) medidores y lecturas con foto al API; (3) pantalla del operario con `SubirFoto`; (4) recibo general y vista previa del reparto; (5) aprobar → líneas del recibo.

**Recorte para el MVP de 133 h:** cola persistente sin señal (IndexedDB) y gráfico histórico de consumo por unidad pasan a la etapa 2. La energía común usa el mismo código que el agua, sin pantallas extra.

**Horas: 8** (Medidores). Son pocas porque la función de reparto es pura y pequeña; empieza por ella y por su prueba.

---

### 09 · Mantenimiento: reportar → validar → informe y costos → aprobación junta → estado

**Objetivo:** que ningún trabajo quede en el aire: se reporta, el administrador valida, publica informe y costos, la junta aprueba en la app y todos ven el estado (nota del dueño §0.2).
**Roles:** propietario, inquilino, operario y técnico (reportan); administrador (valida, presupuesta, ejecuta, cierra); junta (aprueba o rechaza); técnico (actualiza avance de los trabajos asignados). **Dispositivo:** móvil para reportar, aprobar y ver; escritorio para presupuestar.
**Rutas:** `/app/e/:eid/mantenimiento` (tablero), `/app/e/:eid/mantenimiento/reportar`, `/app/e/:eid/mantenimiento/:tid`.

**Estados del trabajo (la línea de tiempo que ven todos)**

```
reportado → validado → presupuestado → aprobado → en_ejecucion → terminado
    │           │                          │
    ├─ descartado (con motivo)             └─ rechazado → «pendiente no aprobado» en el informe mensual
    └─ unido a otro (duplicado)
```

**Pantallas**
- **Reportar (móvil, un paso):** foto (obligatoria al menos una; el video pasa a la etapa 2), ubicación (lista: «Piso 3 – pasadizo», «Cuarto de bombas»…), descripción corta. Botón «Enviar reporte». Nada más.
- **Tablero (admin):** columnas o pestañas por estado con conteo; filtro por criticidad; en móvil, lista con pestañas.
- **Detalle:** línea de tiempo con evidencia en cada paso (quién, cuándo, foto/documento), informe, presupuestos (uno o varios proveedores), votos de la junta, costo ejecutado.

**Endpoints Go**

| Método | Ruta | Devuelve |
|---|---|---|
| `POST` | `/edificios/{eid}/incidencias` | Multipart `{ descripcion, ubicacion, fotos[], video? }` → `201 { id, codigo: "INC-0045" }` |
| `GET` | `/edificios/{eid}/trabajos?estado=&criticidad=&mes=` | Tablero con conteos por estado |
| `GET` | `/edificios/{eid}/trabajos/{tid}` | Detalle con `linea_tiempo`, `evidencias`, `presupuestos`, `votos`, `puedo: [validar, presupuestar, votar…]` |
| `POST` | `/edificios/{eid}/incidencias/{iid}/validar` | `{ accion: aceptar|descartar|unir, criticidad: critica|media|baja, unir_con?, motivo? }` |
| `POST` | `/edificios/{eid}/trabajos/{tid}/informe` | Multipart `{ diagnostico, presupuestos: [{ proveedor, monto_cts, pdf }], elegido, rubro_id }` → pasa a `presupuestado` y avisa a la junta |
| `POST` | `/edificios/{eid}/trabajos/{tid}/votos` | `{ voto: aprueba|rechaza, comentario }` → `201 { resultado: pendiente|aprobado|rechazado }` · `409 YA_VOTASTE` |
| `POST` | `/edificios/{eid}/trabajos/{tid}/avance` | Multipart `{ estado: en_ejecucion|terminado, nota, evidencias[], costo_real_cts?, comprobante? }` |
| `GET` / `PUT` | `/edificios/{eid}/reglas-aprobacion` | `{ modo: presidente|mayoria, umbral_cts }` |

**Estados de pantalla:** tablero vacío («Sin trabajos este mes. Así da gusto»); reporte enviándose con progreso del video; reporte sin conexión (se guarda y se envía al volver, igual que 08); detalle sin permiso para votar (la junta ve el botón; el propietario ve «Esperando aprobación de la junta: 2 de 3 votos»).

**Reglas**
- **Umbral de aprobación** (decisión 8): si el presupuesto elegido es ≤ `umbral_cts`, el administrador lo aprueba solo; si lo supera, decide la junta según `modo` (`presidente`: basta su voto; `mayoria`: más de la mitad de los miembros activos).
- **Un voto por miembro:** `UNIQUE (trabajo_id, miembro_id)` en la base.
- Un trabajo **rechazado** queda como «pendiente no aprobado» con su criticidad y aparece en el informe mensual a la junta (RF-12: «no se hizo porque no lo aprobaste»).
- No se puede pasar a `en_ejecucion` sin aprobación, ni a `terminado` sin al menos una evidencia de cierre.
- **Cada cambio de estado avisa al que reportó** (notificación en la app en el MVP; correo si tiene; WhatsApp después).
- Al terminar, el **costo real** entra al balance (04) como egreso en el rubro elegido, con el comprobante como documento. Si difiere del presupuesto aprobado en más de 10 %, se marca para revisión de la junta.
- Criticidad con semáforo: crítica (rojo), media (ámbar), baja (verde); siempre con texto.
- Videos: tope de 50 MB y 60 s; se suben por partes para aguantar la red móvil.

**Criterios de aceptación**
- [ ] Un propietario reporta una fuga con foto desde el celular en menos de 30 s.
- [ ] Con umbral de S/ 1,000 y modo mayoría con 3 miembros: un presupuesto de S/ 800 lo aprueba el administrador; uno de S/ 1,300 necesita 2 votos a favor.
- [ ] Votar dos veces responde `409` y el conteo no cambia.
- [ ] Un trabajo rechazado aparece como pendiente no aprobado, con criticidad, en el resumen del mes.
- [ ] Al marcar `terminado` con costo real de S/ 1,300, el egreso aparece en el balance con el comprobante.
- [ ] El que reportó ve la línea de tiempo completa desde su portal (10).

**Orden:** (1) máquina de estados en Go con pruebas de transiciones permitidas y prohibidas; (2) reportar con foto; (3) validar; (4) informe y presupuestos; (5) votos y regla de umbral; (6) avance y cierre con egreso al balance; (7) tablero y línea de tiempo.

**Recorte para el MVP de 133 h:** solo fotos (sin video); un presupuesto por trabajo (no comparación de varios proveedores); «unir duplicados» pasa a la etapa 2 (se descarta el duplicado con motivo «igual a INC-…»); el aviso al que reportó es el estado visible en su portal, sin correo. En el MVP el endpoint de reporte acepta solo `fotos[]`.

**Horas: 10** (Mantenimiento). La otra hora del módulo va al portal (10), donde el propietario ve lo que reportó.

---

### 10 · Portal del propietario (móvil)

**Objetivo:** que el propietario, desde su celular, sepa cuánto debe, pague, vea en qué se gastó el dinero, reserve y reporte, sin llamar al administrador.
**Roles:** propietario (todo lo suyo); inquilino (lo que el propietario habilite: por defecto reservas, reportar incidencias y normas; decisión 7). **Dispositivo:** **móvil primero** (PWA instalable).
**Ruta:** `/app/e/:eid/portal`.

**Contenido (de arriba abajo)**
1. **Saludo y estado de cuenta:** «Hola, Rosa · Dpto. 101». Tarjeta grande: «Tu recibo de setiembre: S/ 1,800.00 · vence el 15/10» con estado (`Insignia`) y botón **«Pagar»**. Si debe meses anteriores, lo dice con el detalle por mes.
2. **Accesos rápidos:** Pagar · Reservar (07) · Reportar (09) · Normas.
3. **Mi recibo:** desglose con la **foto del medidor**, consumo y agua común; descargar PDF; historial de recibos y pagos.
4. **Transparencia:** los 4 KPIs del edificio y el árbol de balance (04) en solo lectura.
5. **Mantenimiento:** trabajos del mes y lo que reportó, con su estado.
6. **Mis reservas:** próximas y pasadas, con código.
7. **Normas:** manual de convivencia y reglamento de áreas.
8. **Si es propietario de una unidad alquilada:** interruptores para habilitar al inquilino (reservar, reportar, ver recibos).

**Endpoints Go** (todos filtrados a **sus** unidades)

| Método | Ruta | Devuelve |
|---|---|---|
| `GET` | `/edificios/{eid}/portal` | `{ unidades: [...], recibo_actual, deuda: { total_cts, meses: [...] }, kpis_edificio, trabajos_mes, mis_incidencias, proximas_reservas, normas_url }` (una sola petición para la portada) |
| `GET` | `/edificios/{eid}/recibos?mios=1` | Historial propio |
| `POST` | `/edificios/{eid}/recibos/{rid}/pagos` | El mismo de 05; entra como `pendiente_validacion` |
| `GET` / `PUT` | `/edificios/{eid}/unidades/{uid}/permisos-inquilino` | `{ reservar, reportar, ver_recibos }` |
| (reutiliza) | 04, 07, 09 | Balance, reservas e incidencias |

**Pagar con Yape (MVP):** la pantalla muestra el QR y el número de Yape del edificio, el monto exacto y el concepto a poner («Dpto 101 set-2026»); el propietario sube la captura del voucher y el código de operación. Se ve «Pago enviado, en revisión» hasta que el administrador lo valida.

**Estados:** cargando (esqueleto de la tarjeta de recibo); sin recibos aún («Tu primer recibo llega el 1 de octubre»); al día (tarjeta verde «Estás al día. ¡Gracias!»); moroso (tarjeta roja con total y meses, sin tono de reproche; el acceso a Reservar muestra el motivo del bloqueo); pago en revisión; inquilino sin permiso para una sección (la sección no aparece).

**Reglas**
- Con varias unidades, selector de unidad arriba; la deuda se muestra por unidad y total.
- El propietario **nunca** ve datos personales de otras unidades; en el balance ve montos, no nombres de morosos.
- Tamaño mínimo de texto 16 px; montos en 24 px o más; contraste AA.
- Instalable: banner propio «Instala EDISYS en tu celular» la segunda vez que entra (no en la primera).

**Criterios de aceptación**
- [ ] En un Android de gama media en 4G, la portada carga en menos de 2 s.
- [ ] Rosa (101) ve S/ 418.00 de agua con la foto de su medidor.
- [ ] Rosa sube un voucher de Yape y el administrador lo ve en «Vouchers por validar» del dashboard (03).
- [ ] Un inquilino sin permiso de recibos no ve la sección ni puede pedir `GET /recibos` (el API responde `403`).
- [ ] La app se instala en Android e iOS y abre en `/app/` sin barra del navegador.

**Orden:** (1) endpoint agregado del portal; (2) tarjeta de estado de cuenta y detalle del recibo; (3) pago con voucher; (4) accesos a 07 y 09; (5) transparencia (reutiliza 04); (6) permisos del inquilino; (7) instalación PWA.

**Recorte para el MVP de 133 h:** los interruptores del inquilino (`permisos-inquilino`) pasan a la etapa 2; el inquilino recibe el permiso por defecto de la decisión 7 (reservar, reportar y normas). El banner propio de instalación también pasa a la etapa 2: se usa el aviso nativo del navegador.

**Horas: 5** (Cuotas y recibos 2 + Reservas 2 + Mantenimiento 1). Es corta porque reutiliza los endpoints y componentes de 04, 05, 07 y 09.

---

### 11 · Roles y permisos

**Objetivo:** que el administrador decida quién entra, a qué edificio y con qué rol, y ajuste lo que ve cada rol sin tocar código.
**Roles:** administrador (su administradora y sus edificios); superadmin (todas las administradoras). **Dispositivo:** escritorio.
**Ruta:** `/app/e/:eid/ajustes/usuarios` y `/app/ajustes/roles`.

**Contenido**
- **Usuarios:** lista (nombre, correo o unidad, rol por edificio, último ingreso, estado); invitar; desactivar; restablecer clave (envía enlace; el administrador **nunca** ve ni fija claves).
- **Matriz de permisos:** filas = permisos agrupados por módulo, columnas = los 7 roles; casillas editables solo para los permisos **ajustables** (p. ej. ¿el propietario ve documentos de egresos?, ¿el inquilino ve el balance?). Los permisos peligrosos (emitir recibos, administrar roles) están fijos y se muestran con candado.
- **Junta:** quiénes son miembros, quién es presidente (lo usa la regla de 09).
- **Bitácora:** quién cambió qué permiso y cuándo.

**Endpoints Go**

| Método | Ruta | Devuelve |
|---|---|---|
| `GET` | `/yo` | Usuario, roles por edificio, permisos efectivos, menú |
| `GET` | `/edificios/{eid}/usuarios?pagina=` | Lista |
| `POST` | `/edificios/{eid}/usuarios/invitar` | `{ correo | unidad_id, rol }` → `201 { enlace_invitacion }` |
| `PATCH` | `/edificios/{eid}/usuarios/{uid}` | `{ rol?, activo? }` · `409 ULTIMO_ADMIN` |
| `GET` | `/roles` | Roles con sus permisos y cuáles son ajustables |
| `PUT` | `/edificios/{eid}/roles/{rol}/permisos` | `{ permisos: [...] }` (solo ajustables) → sube la `ver` de permisos del edificio |
| `GET` / `PUT` | `/edificios/{eid}/junta` | Miembros y presidente |
| `GET` | `/edificios/{eid}/auditoria?modulo=roles` | Bitácora |

**Estados:** lista vacía salvo el propio admin; invitación enviada (enlace copiable); intento de quitarse el último rol de administrador (Dialog que lo impide); guardando matriz (optimista, revierte si el API falla).

**Reglas**
- **No se puede dejar un edificio sin administrador** (`409 ULTIMO_ADMIN`).
- Nadie se sube a sí mismo de rol.
- Cambiar un permiso sube la versión `ver`; el middleware la compara y el cambio surte efecto en menos de 60 s, sin cerrar sesiones.
- Un administrador solo ve y asigna edificios de **su** administradora.
- Todo cambio va a `auditoria` con usuario, IP, antes y después.

**Criterios de aceptación**
- [ ] Quitar `balance.ver_documentos` al propietario hace que, en menos de 60 s, sus nodos de documento muestren «Documento disponible para la junta» y el API responda `403` al documento.
- [ ] Un administrador de la administradora A no puede listar usuarios de un edificio de la administradora B (`404`).
- [ ] Desactivar un usuario invalida sus refresh y en su siguiente petición recibe `401`.
- [ ] La matriz se usa en 1280 px sin scroll horizontal; en móvil se muestra por rol (un acordeón por rol).

**Recorte para el MVP de 133 h:** la **matriz editable** y la pantalla de bitácora pasan a la etapa 2. En el MVP los permisos de cada rol son fijos (semilla en la base), el administrador asigna roles, invita, desactiva y define la junta; la auditoría se graba en la tabla `auditoria` aunque aún no tenga pantalla. Los criterios de la matriz de arriba se verifican en la etapa 2.

**Orden:** (1) tablas y semilla de los 7 roles con permisos por defecto (va en la fase B, porque todo lo demás depende del middleware); (2) `/yo`; (3) usuarios e invitaciones; (4) junta.

**Horas: 5** (del módulo «Backend Go + roles»).

---

## 4. Orden global de implementación y horas

**Tarifa única: S/ 30 la hora** (demo e implementación).
- **Implementación del MVP: 133.33 h × S/ 30 = S/ 4,000.**
- **Demo: 8 h × S/ 30 = S/ 240**, aparte (§5).

Con 133 h el MVP es **ajustado**: cada pantalla trae su línea «Recorte para el MVP de 133 h», que dice qué de su descripción pasa a la etapa 2. Constrúyelo tal cual y anota lo que el piloto pida de más; no metas funciones fuera de la lista sin cotizarlas.

### 4.1 Horas por módulo de la cotización y por pantalla

La cotización reparte las horas por **módulo técnico**; esta guía las reparte por **pantalla**. La matriz de abajo es la misma cifra vista de los dos lados: cada fila suma lo que dice la cotización y cada columna suma las horas de la pantalla.

| Módulo (cotización) | Horas | B | 01 | 02 | 03 | 04 | 05 | 06 | 07 | 08 | 09 | 10 | 11 | Cierre |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Arquitectura / Docker | 10 | 10 | | | | | | | | | | | | |
| Backend Go + roles | 13 | 6 | 2 | | | | | | | | | | 5 | |
| Login Qwik + Astro | 7 | | 4 | 3 | | | | | | | | | | |
| PostgreSQL | 10 | 6 | | | 1 | 2 | | | 1 | | | | | |
| Registro + Excel | 11 | | | | | | | 11 | | | | | | |
| Cuotas, reparto y recibos | 16 | | | | | | 14 | | | | | 2 | | |
| Facturación | 11 | | | | | | 11 | | | | | | | |
| Balance por nodos | 12 | | | | 3 | 9 | | | | | | | | |
| Reservas | 10 | | | | | | | | 8 | | | 2 | | |
| Medidores | 8 | | | | | | | | | 8 | | | | |
| Mantenimiento | 11 | | | | | | | | | | 10 | 1 | | |
| Almacenamiento / backups | 5 | 5 | | | | | | | | | | | | |
| EC2 / SSL / despliegue | 4 | | | | | | | | | | | | | 4 |
| Pruebas | 5.33 | | | | | | | | | | | | | 5.33 |
| **Total** | **133.33** | **27** | **6** | **3** | **4** | **11** | **25** | **11** | **9** | **8** | **10** | **5** | **5** | **9.33** |

Qué hay en cada celda que no es obvia:
- **B · Backend Go 6:** esqueleto, errores, middleware de sesión y permisos, JWT, `/yo`.
- **B · PostgreSQL 6:** esquema, migraciones, índices y semilla de la demo.
- **B · Almacenamiento 5:** Garage, subida por el API, URLs firmadas, `pg_dump` diario y copia a R2.
- **B · Arquitectura 10:** monorepo, compose con límites, Caddy, CI, además de tokens, componentes compartidos, armazón y PWA.
- **Pruebas 5.33:** e2e de Playwright del flujo principal y la corrida completa final. Las **pruebas unitarias** (motor de reparto, caso 5,000 / 4,800 / 200, reservas, máquina de estados, permisos) van **dentro** de las horas de cada módulo; no se recortan.
- **EC2 4:** endurecimiento, TLS, despliegue final y restaurar un respaldo para probarlo.

### 4.2 Orden de construcción y correspondencia con las fases del plan

El orden sigue las fases del plan de trabajo, con una excepción: el **dashboard (03)** se construye después de las pantallas que resume.

| Paso | Fase del plan | Pantalla o bloque | Horas | Acumulado | Sale cuando… |
|---|---|---|---|---|---|
| 1 | 1 · Base | **B** Base común (§1.4 y §2) | 27 | 27 | `make dev` levanta todo y `/api/health` responde en el EC2 |
| 2 | 1 · Base | **01** Login (Qwik) | 6 | 33 | Entran admin y propietario, cada uno a su pantalla |
| 3 | 1 · Base | **11** Roles y permisos | 5 | 38 | Los 7 roles con guardas en Go y React |
| 4 | 1 · Base | **06** Unidades + importación Excel | 11 | 49 | El padrón del piloto entra desde su Excel |
| 5 | 2 · Cuotas y recibos | **05** Recibos y facturación | 25 | 74 | Los recibos cuadran al céntimo con el presupuesto |
| 6 | 3 · Balance | **04** Balance por nodos | 11 | 85 | Un propietario entiende el mes sin ayuda |
| 7 | 5 · Medidores | **08** Lectura de medidores | 8 | 93 | Caso 5,000 / 4,800 / 200 pasa y sale en el recibo |
| 8 | 4 · Reservas | **07** Reservas con pago | 9 | 102 | Una reserva pagada aparece sola en el balance |
| 9 | 6 · Mantenimiento | **09** Mantenimiento con aprobación | 10 | 112 | Una incidencia de punta a punta, visible para el propietario |
| 10 | 3 | **03** Dashboard del administrador | 4 | 116 | Sus cifras coinciden con 04 y 05 |
| 11 | 1 y 4 | **10** Portal del propietario | 5 | 121 | Rosa paga, reserva y reporta desde el celular |
| 12 | — | **02** Landing (Astro) | 3 | 124 | Lighthouse ≥ 95 y el formulario llega |
| 13 | — | **Cierre:** e2e completo, respaldo restaurado, TLS, despliegue final y carga del piloto | 9.33 | **133.33** | El piloto funciona en producción |

Por qué este orden:
- **08 va antes que 07** aunque el plan los numere al revés: 08 alimenta los recibos (05) y el caso del dueño es lo que más vende. Las reservas se pueden probar con cargo al recibo sin depender de nada nuevo.
- El dashboard (03) y el portal (10) son pantallas **de lectura** que agregan datos de las demás; hacerlas al final evita rehacerlas. Por eso cuestan tan poco: si las haces antes, cuestan el doble.
- La landing (02) no bloquea a nadie y se puede hacer en paralelo por otra persona.

### 4.3 Resumen por fase

| Fase del plan | Pantallas | Horas en este MVP | Horas en el plan original |
|---|---|---|---|
| 1 Base del edificio | B, 01, 11, 06 | 49 | 60 |
| 2 Cuotas, recibos y morosidad | 05 | 25 | 70 |
| 3 Balance y reportes por nodos | 04, 03 | 15 | 50 |
| 4 Reservas y pagos | 07, 10 | 14 | 50 |
| 5 Medidores | 08 | 8 | 35 |
| 6 Flujo de mantenimiento | 09 | 10 | 50 |
| Público y cierre | 02, cierre | 12.33 | — |
| **Total** | **11 pantallas** | **133.33** | **~330** (+16 de fase 0) |

La **fase 0** del plan (descubrimiento, 16 h) no está en las 133.33 h: la cubren la demo de 8 h (§5), el lienzo de diseño y las reuniones con el dueño, que van por cuenta aparte.

### 4.4 Qué queda fuera del MVP (etapa 2)

Para bajar de ~330 h a 133 h, además de los recortes de cada pantalla, esto queda **fuera del MVP** y se cotiza después (horas aproximadas a S/ 30):

| Fuera del MVP | Horas aprox. | Mientras tanto |
|---|---|---|
| Conciliación bancaria (subir extracto y cuadrar) | 20 | El balance se arma con pagos validados y egresos con sustento |
| Pasarela de tarjeta (Izipay/Niubiz) | 15 | Yape/transferencia con voucher validado por el admin |
| WhatsApp (recibos, reservas, avisos) con evolution-go | 12 | Correo y el estado visible dentro de la app |
| Informe mensual a la junta y PDF del balance | 12 | Balance en pantalla (04) y tablero de 09 |
| Cuotas extraordinarias atadas a aprobación | 8 | Línea manual en el recibo con referencia al trabajo |
| Multas y penalidades configurables | 8 | Línea manual «multa» en el recibo |
| Comprobante electrónico SUNAT | 20 | Recibo interno por departamento |
| Recortes de pantalla: olvidé mi clave, matriz de permisos editable, bitácora en pantalla, permisos del inquilino, video en incidencias, cola sin señal de medidores, cancelación por el propietario, bloques extra del dashboard | 30 | Ver la línea «Recorte» de cada pantalla |
| PAM, personal, service desk, inventario (fases 7–9 del plan) | 130 | — |

### 4.5 Riesgos del calendario

- **Margen casi nulo.** 133 h para 11 pantallas solo alcanza si nadie rehace trabajo: respeta el orden de §4.2, empieza cada bloque por la función pura y su prueba, y reutiliza los componentes de §2.2 en lugar de estilar cada pantalla a mano.
- **El Excel real de reparto** (decisión 9) no ha llegado. El motor de 05 se construye con las reglas conocidas; si el Excel trae fórmulas raras, cuesta entre 4 y 10 h extra, **fuera** de las 133.33 h.
- **Datos del piloto:** sin padrón real, los criterios «sale cuando…» se validan con la demo y quedan pendientes de confirmar con datos reales.
- **RAM:** los límites de §1.2 dejan margen, pero **no** construyas imágenes en el EC2.

---

## 5. La demo de 8 horas

**Costo:** 8 h × S/ 30 = **S/ 240**, aparte de la implementación.

**Objetivo:** en una reunión de 20 minutos, mostrar al dueño y a una administradora el login, el dashboard, el balance por nodos y la lectura de medidores con el caso S/ 5,000 / 4,800 / 200, funcionando en su propio celular y en `https://demo.edisys.pe`.

**Qué entra:** 01 Login, 03 Dashboard (solo KPIs y tareas), 04 Balance por nodos (sin exportar PDF ni registrar egresos), 08 Lectura de medidores (operario + vista previa del reparto).
**Qué no entra:** recibos emitidos (se siembran ya hechos), reservas, mantenimiento, roles editables, landing, refresh de token, límite de intentos, cola sin señal.

**Datos de ejemplo (semilla `api/seed/losolivos.sql`):**
- Administradora «Demo Administraciones SAC»; edificio **«Residencial Los Olivos»**, Jesús María, Lima; 10 departamentos (101–105 al 9 %, 201–205 al 11 %).
- Usuarios: `admin@losolivos.pe` (administrador), `conserje@losolivos.pe` (operario), `101` + DNI `45120001` (propietaria Rosa Quispe; datos inventados).
- Periodo setiembre 2026: presupuesto de S/ 18,000, pagos de 8 de 10 unidades (2 morosas), egresos con fotos de sustento, 2 reservas de parrilla pagadas, recibo de Sedapal de S/ 5,000 por 1,000 m³.
- Lecturas de agosto sembradas como «lectura anterior»; las de setiembre **se toman en vivo** durante la demo (se llevan fotos impresas de medidores con las lecturas que dan los consumos de la tabla de 08).

### Hora a hora

| Hora | Qué se hace | Entregable al terminar la hora |
|---|---|---|
| **1** | Monorepo mínimo (`api`, `login`, `app`, `edge`, `packages/tokens`); `docker-compose.yml` con `edge`, `login`, `api`, `postgres`, `garage`; migraciones de las tablas que usa la demo (edificio, unidad, usuario, rol, rubro, concepto, egreso, documento, pago, medidor, lectura, recibo_general); semilla de Los Olivos | `make dev` levanta todo; `psql` muestra las 10 unidades y los egresos |
| **2** | Go: `POST /auth/login` (bcrypt + JWT en cookie), middleware de sesión y de rol (sin refresh), `GET /yo`, `GET /edificios/{eid}/dashboard`, `GET /edificios/{eid}/balance` y `/balance/nodos/{id}` sobre la función `ArbolBalance` con su prueba del invariante | `curl` con la cookie devuelve el árbol y los KPIs correctos |
| **3** | Qwik: pantalla de login con `routeAction$` que llama al API y escribe la cookie; redirección por rol. Tokens de diseño desde DISENO.md en el preset de Tailwind compartido | Entras con el admin y caes en `/app/`; con el conserje, en `/app/…/lecturas` |
| **4** | React: armazón responsivo (menú lateral en escritorio, barra inferior en móvil), `SesionContext` con `/yo`, `TarjetaKPI`, `Dialog` y `Toast` mínimos; **pantalla 03** con los 4 KPIs y el bloque de tareas (lecturas pendientes: 10) | Dashboard visible en PC y en celular con cifras reales de la semilla |
| **5** | **Pantalla 04:** `NodoDesplegable` con carga perezosa, visor de documentos con URL firmada de Garage, enlace desde los KPIs del dashboard | Bajas de «Egresos» a «Sedapal» y ves la foto del recibo de S/ 5,000 |
| **6** | **Pantalla 08:** función de reparto en Go con la **prueba del caso del dueño** (5,000 / 4,800 / 200 → tabla exacta); `GET /lecturas`, `POST /medidores/{mid}/lecturas` con foto obligatoria a Garage; pantalla del operario con `SubirFoto` (cámara trasera, compresión) y «Guardar y siguiente» | El conserje registra lecturas con foto desde el celular; el contador sube a «10 de 10» |
| **7** | Vista previa del reparto para el admin (`/reparto-medidores/calcular`): tabla por unidad con agua propia, agua común y total, que cuadra en S/ 5,000.00. Despliegue en el EC2: imágenes construidas en GitHub Actions (o en la laptop con `docker buildx` y `docker save`), TLS con Caddy en `demo.edisys.pe` | La demo corre en `https://demo.edisys.pe` y abre en el celular |
| **8** | Ensayo completo con cronómetro y correcciones: orden del guion, textos, estados de carga y vacío visibles, prueba en un Android y un iPhone, `docker stats` bajo los límites; reinicio de datos con `make seed` para dejarla limpia | Guion de 20 minutos ensayado y demo reiniciada |

### Guion de la reunión (20 min)

1. **Login en el celular** del dueño con `101` + DNI: cae en su vista (2 min). Luego con el admin en la laptop.
2. **Dashboard:** «setiembre cerró con S/ X de saldo; 2 unidades morosas; 10 lecturas pendientes» (3 min).
3. **Balance por nodos:** de «Egresos» a la foto del recibo de Sedapal en tres toques (5 min). Frase clave: *«cada sol, con su sustento»*.
4. **Medidores en vivo:** el dueño toma con su celular la foto de 2 medidores impresos; el resto ya está cargado (5 min).
5. **El reparto:** S/ 5,000 → S/ 4,800 de departamentos + S/ 200 de agua común repartidos por participación; la 101 paga S/ 418.00 (3 min).
6. **Cierre:** lo que falta para el MVP (§4) y la cotización: 133.33 h a S/ 30 = S/ 4,000 (2 min).

**Lo que la demo deja para el MVP:** el esqueleto del monorepo, el compose, el login, la función `ArbolBalance`, la función de reparto con sus pruebas y los componentes `TarjetaKPI`, `NodoDesplegable` y `SubirFoto`. Se reaprovechan, pero **no se descuentan** de las 133.33 h: en la demo se escriben sin refresh, sin límites de intentos, sin cola sin señal y sin pruebas e2e, y todo eso se completa en el MVP.
