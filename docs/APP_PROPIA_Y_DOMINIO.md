# Dominio propio, app propia y videollamadas (bloques I3, I4 y J2)

Los tres bloques tienen una parte que hace EDISYS y otra que depende de un tercero
(DNS, tiendas, servidor Jitsi). Este documento separa las dos: EDISYS **no** toca DNS,
**no** emite certificados por su cuenta, **no** publica en tiendas y **no** llama a Jitsi.

## I3 · Dominio propio por administradora

**Qué hace EDISYS**

- Tabla `administradora_dominio` (migración `0031`): un host pertenece a una sola administradora.
- La administración registra, activa/desactiva o quita el dominio en
  *Videollamadas → Dominio propio* (permiso `dominios.administrar`) o por API:
  `GET|POST /api/v1/edificios/{eid}/dominios`, `PATCH|DELETE /dominios/{id}`.
- `GET /api/v1/publico/dominio` resuelve la administradora por `Host` / `X-Forwarded-Host`
  (para que login y app muestren su nombre).
- `GET /api/v1/publico/dominio-permitido?domain=` responde 200 solo para hosts registrados y
  activos: es el `ask` del TLS bajo demanda de Caddy.
- En un dominio propio **solo inician sesión los usuarios de esa administradora** (y el superadmin).
  Un usuario de otra recibe el mismo 401 que una clave errada. En el dominio de la plataforma no
  cambia nada.

**Qué hace el cliente / operación**

1. El cliente crea un registro `A` (o `CNAME`) de su dominio hacia la IP del servidor.
2. Operación fusiona `edge/dominios.ejemplo.caddy` con `edge/Caddyfile` (instrucciones dentro del
   archivo) y recarga el borde. El certificado se emite al primer visitante, solo si el `ask` dice 200.

**Pendiente de decidir (§7 del plan):** despliegue por cliente vs. multi-tenant con marca. Este
bloque implementa multi-tenant (un borde, muchos hosts).

## I4 · App propia (PWA con marca, base para tiendas)

`scripts/app-propia.mjs` construye la PWA con nombre, iconos, colores e id de la administradora:

```bash
node scripts/app-propia.mjs --nombre "Torres Administración" --corto "Torres" \
  --paquete pe.torres.app --id torres --icono-512 ./marca/icono-512.png \
  --host intranet.torres.pe --salida build-apps/torres
```

- Valida los parámetros (`app/src/lib/appPropia.js`, con pruebas): nombre corto ≤ 12, colores
  `#RRGGBB`, paquete en dominio invertido, icono PNG de 512×512 (el de 192 se genera con `sips` o
  ImageMagick, o se pasa con `--icono-192`).
- `app/public` no se toca: los iconos van a una copia temporal (`EDISYS_PUBLIC_DIR`).
- Sale `web/` (estático), `marca.json` y, con `--host`, `twa-manifest.json` para
  [Bubblewrap](https://github.com/GoogleChromeLabs/bubblewrap).
- `--solo-validar` revisa todo sin construir.
- Sin variables `EDISYS_APP_*`, `astro build` produce exactamente el manifiesto de siempre.

**Publicar (fuera de EDISYS):** quien tenga la cuenta de Google Play corre
`bubblewrap init --manifest https://<host>/app/manifest.webmanifest` y `bubblewrap build`
con su llave de firma, y sube el `.aab`. Para el *Digital Asset Links* hay que servir
`/.well-known/assetlinks.json` en el dominio del cliente. iOS: la PWA se instala desde Safari;
una app de App Store exige un contenedor nativo (fuera de alcance).

## J2 · Videollamadas junta ↔ administración

- `JITSI_BASE_URL` (variable del API; vacía = `https://meet.jit.si`). Solo se aceptan URLs
  http(s); cualquier otra cae al valor por defecto.
- El enlace no se guarda: se arma como `JITSI_BASE_URL/<código>` en cada lectura, así cambiar
  a un Jitsi propio no deja enlaces viejos. El código lleva 80 bits aleatorios.
- Permisos: `videollamadas.ver` (administración y junta), `videollamadas.administrar`
  (convocar, cancelar, enlazar grabación). El propietario no recibe enlaces (403).
- La grabación es opcional y vive fuera (Jitsi/Dropbox/servidor propio): solo se guarda un enlace `https://`.
