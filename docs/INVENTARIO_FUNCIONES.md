# Inventario de funciones — segunda pasada de interfaz

**Regla madre:** no se pierde ninguna función. Esta lista se sacó del código **antes** de tocar nada
(rama `feat/ui-pasada2`, commit `4e1acab`). Cada línea dice qué hay hoy y, al terminar la pasada,
**dónde vive ahora**. Si algo cambia de sitio (a un menú `⋯`, a un despliegue, a un atajo), se dice aquí.

Leyenda de la columna «Ahora»: ✅ sigue igual de sitio · ➜ cambió de sitio (se dice a dónde) · ✚ se añadió.

**Estado: cerrado el 28-09-2026** (rama `feat/ui-pasada2`). Cada línea se revisó contra el código final y
contra el recorrido `scripts/recorrido-ui.cjs` (3 roles, 18 vistas, 0 errores). Ninguna función se perdió.

---

## Armazón (todas las pantallas) — `layout/Armazon.jsx`, `lib/permisos.js`

| # | Función | Permiso / condición | Ahora |
|---|---|---|---|
| A1 | Enlace «Saltar al contenido» | — | ✅ |
| A2 | Menú lateral con las secciones del rol, filtrado por permisos | `menuPara(rol, tiene)` | ➜ lateral en 3 estados: abierto ≥ 1280 (220 px), solo iconos 1024–1279 (64 px, nombre en tooltip), cajón < 1024; plegar/abrir se recuerda (`edisys.menu`). Mismos ítems (`ITEMS` / `MENU_POR_ROL`, ahora exportados) |
| A3 | Ítem activo según página + query (`?reportar=1`) | — | ✅ (`esActivo`, sin cambios) |
| A4 | Logo que lleva al inicio | — | ✅ |
| A5 | Selector de edificio (solo si hay más de uno; si no, tarjeta con nombre y n.º de unidades) | — | ➜ en el lateral abierto y en el cajón (tablet/móvil); en modo iconos, el botón «Abrir el menú completo» lo muestra |
| A6 | Bloque de usuario (iniciales, nombre, rol) | — | ✅ |
| A7 | Cerrar sesión | — | ✅ (lateral y «Más») |
| A8 | Cambiar de rol en modo mock (`VITE_MOCK=1`) | `s.mock` | ✅ |
| A9 | Cabecera móvil con logo, edificio, rol e iniciales que abre «Más opciones» | — | ➜ misma cabecera; el botón de edificio/usuario y el nuevo botón ☰ abren el cajón |
| A10 | Barra inferior móvil con las pestañas del rol + «Más» | `menu.movil`, `menu.mas` | ✅ con los accesos del plan (admin: Resumen · Recibos · Mantenim. · WhatsApp · Más; «Más» siempre, también para operario y técnico). Balance pasó a «Más» en el admin |
| A11 | Hoja «Más opciones»: edificio, secciones que no caben, usuario, correo, salir | — | ➜ cajón a la izquierda (`Modal cajon`) con TODAS las secciones, edificio, modo mock, usuario, correo y Cerrar sesión |
| A12 | Modo tarea (`useModoTarea`): oculta cabecera y barra en pantallas de una tarea; modo `cabecera` en el portal | — | ✅ (misma API) |
| A13 | Aterrizaje por rol (`/app/` redirige si no tiene `dashboard.ver`) | `destinoPorRol` | ✅ |
| A14 | Pantalla sin permiso → `SinPermiso` con el permiso que falta | `RutaProtegida`, `Isla` | ✅ |
| A15 | Aviso de versión nueva (PWA) con «Toca para actualizar» | — | ✅ |
| A16 | Recordar edificio elegido (`localStorage`) | — | ✅ |
| A17 | Error al cargar `/yo` con Reintentar; sin edificio → «Entrar con otra cuenta» | — | ✅ |

## 01 · Login (`login/src/routes/index.tsx`)

| # | Función | Ahora |
|---|---|---|
| L1 | Formulario correo/DNI + contraseña, validación local | ✅ |
| L2 | Envío con JS al API; sin JS, `POST` al servidor Qwik que reenvía y copia cookies | ✅ |
| L3 | Mostrar/ocultar contraseña (botón con `aria-label`) | ✅ |
| L4 | «Recordar este dispositivo» (guarda el correo, nunca la clave) | ✅ |
| L5 | «¿La olvidaste?» → nota explicativa | ✅ |
| L6 | «Recibir código por WhatsApp» → nota de «siguiente etapa» | ✅ |
| L7 | Error del API con código para soporte; bloqueo 429 con cuenta regresiva | ✅ |
| L8 | Redirección a `next` seguro | ✅ |
| L9 | Panel de marca (escritorio) / cabecera con lema (móvil); aviso «Instala EDISYS» en móvil | ✅ (panel más estrecho) |
| L10 | Enlace «Pide tu acceso a la administración» | ✅ |

## 03 · Dashboard (`pantallas/03-dashboard/Dashboard.jsx`)

| # | Función | Permiso | Ahora |
|---|---|---|---|
| D1 | Cambiar de periodo (flechas; URL `?periodo=`) | — | ✅ encabezado |
| D2 | Imprimir | — | ➜ menú `⋯` del encabezado (en escritorio y móvil) |
| D3 | «Ir a recibos» del periodo | `recibos.emitir` | ✅ acción principal del encabezado |
| D4 | KPI Ingresos cobrados → balance `abrir=ing`, con nota y variación vs. mes anterior | — | ➜ franja compacta de KPI (mismo enlace, variación como flecha + texto) |
| D5 | KPI Egresos → balance `abrir=egr`, rubros/documentos y variación | — | ➜ franja compacta |
| D6 | KPI Saldo del mes → balance | — | ➜ franja compacta |
| D7 | KPI Saldo en banco (si el API lo manda) | — | ➜ franja compacta |
| D8 | KPI Morosidad → recibos vencidos (%, unidades, monto) | — | ➜ franja compacta, en grande y a la izquierda |
| D9 | KPI Ingresos por reservas → reservas | — | ➜ franja compacta |
| D10 | Egresos por rubro con barras, cada rubro enlaza a su nodo del balance | — | ✅ |
| D11 | Si no hay rubros: «Mantenimiento del mes» (trabajos con monto e insignia) | — | ✅ + cada trabajo enlaza al tablero filtrado por su código |
| D12 | Ingresos por concepto (si vienen) | — | ✅ |
| D13 | «Ver balance por nodos» | — | ✅ |
| D14 | Tareas de hoy (del API o armadas con contadores), cada una enlaza a su pantalla | — | ➜ lista con icono de estado |
| D15 | Morosidad por unidad (tabla unidad/meses/reservas bloqueadas/deuda) o resumen + «Ver recibos vencidos» | — | ➜ fusionada con «Cobranza» en una tarjeta: barra de avance y morosos en línea; enlace «Ver recibos vencidos» |
| D16 | Cobranza del mes: pagados/emitidos/parciales, % con barra, emitido/cobrado/por cobrar, nota del periodo | — | ➜ misma tarjeta fusionada |
| D17 | Estado vacío: «Importar unidades» | `unidades.importar` | ✅ |
| D18 | Error con Reintentar; esqueletos de carga | — | ✅ |

## 04 · Balance por nodos (`pantallas/04-balance/Balance.jsx`, `ui/NodoDesplegable.jsx`)

| # | Función | Permiso | Ahora |
|---|---|---|---|
| B1 | Cambiar de periodo | — | ✅ |
| B2 | Expandir todo | — | ➜ botón secundario del encabezado (sigue visible) |
| B3 | Imprimir | — | ➜ menú `⋯` |
| B4 | Registrar egreso (modal: rubro, concepto, monto, fecha, documento; aviso «sin sustento») | `egresos.registrar` | ✅ acción principal |
| B5 | 4 KPI: ingresos, egresos (+banco), saldo, morosidad (→ recibos vencidos si `recibos.ver`) | — | ➜ franja compacta |
| B6 | Árbol perezoso: raíz + 2 niveles abiertos; carga de hijos al abrir; error por fila con Reintentar | — | ✅ |
| B7 | `?abrir=` abre los ancestros y enfoca el nodo | — | ✅ (ya no pide al API ids que no existen: `?abrir=egresos` daba 404) |
| B8 | Migas del nodo seleccionado | — | ✅ |
| B9 | Teclado: ↑/↓ mueven, →/← abren/cierran, Enter abre o muestra documento | — | ✅ |
| B10 | Visor de documento (lateral en escritorio, hoja en móvil): vista previa, datos, descargar, bloqueo «para la junta», enlace que caduca | `balance.ver_documentos` (API) | ✅ |
| B11 | Nota de conciliación (si el API la manda) | — | ✅ acepta texto o el objeto nuevo `{ texto, conciliado, … }` (antes rompía la pantalla) |
| B12 | Insignia «sin sustento» en el nodo | — | ✅ |
| B13 | Estado vacío y error | — | ✅ |

## 05 · Recibos (`pantallas/05-recibos/Recibos.jsx`, `PagoModal.jsx`)

| # | Función | Permiso | Ahora |
|---|---|---|---|
| R1 | Cambiar de periodo | admin (no `propio`) | ✅ |
| R2 | Generar borradores (alerta con total y advertencias) | `recibos.emitir` | ✅ secundaria del encabezado en escritorio; ➜ en móvil, menú `⋯` (`soloMovil`); ✚ también como acción del estado vacío |
| R3 | Emitir el periodo (confirmación) | `recibos.emitir` | ✅ acción principal |
| R4 | Filtro por estado: Todos · Pagados · Vencidos (con contador si el API lo manda), en la URL | — | ➜ chips (`Chip`): Todos · Pagados · Vencidos · ✚ Parciales (`estado=pagado_parcial`), siempre con contador (del API o pidiendo los totales con `por_pagina=1`) |
| R5 | Buscar unidad (con pausa de 350 ms, en la URL) | — | ✅ misma línea que los chips |
| R6 | Lista paginada (25) con unidad, propietario, (periodo si es propio), total, estado; «N de M recibos» | — | ✅ |
| R7 | Vista «Mis recibos» del propietario (`mios=1`) | `portal.ver` sin `recibos.emitir` | ✅ |
| R8 | Seleccionar recibo (`?id=`) → detalle a la derecha (móvil: uno tras otro, «Volver a la lista») | — | ✅ panel lateral fijo en escritorio (entra desde la derecha, la lista sigue a la vista); hoja completa en móvil con «Volver a la lista» |
| R9 | Detalle: número, edificio, unidad, propietario, participación, emitido, vence, insignia con fecha/medio/op. | — | ✅ |
| R10 | Desglose de líneas, total, saldo pendiente | — | ✅ |
| R11 | Foto del medidor con serie, fecha y operario | — | ✅ |
| R12 | Lista de pagos con estado | — | ✅ |
| R13 | Nota «no es comprobante SUNAT» | — | ✅ |
| R14 | Descargar PDF | — | ✅ |
| R15 | Enviar por correo | `recibos.emitir` | ✅ |
| R16 | Enviar por WhatsApp (aviso si SIMULADO) | `whatsapp.enviar` | ✅ |
| R17 | Registrar pago (admin) / Pagar (propietario) con voucher, pago parcial, medios, código de operación | `pagos.registrar` / `pagos.informar` | ✅ |
| R18 | Anular con motivo (sin pagos) | `recibos.emitir` | ➜ menú `⋯` «Más acciones del recibo» (escritorio y móvil) |
| R19 | `?pagar=1` abre el pago directo (desde el portal) | — | ✅ |
| R20 | Estado vacío (propio y admin, con filtro o sin él), error | — | ✅ |

## 06 · Unidades (`pantallas/06-unidades/Unidades.jsx`, `Importar.jsx`)

| # | Función | Permiso | Ahora |
|---|---|---|---|
| U1 | Pestañas Unidades · Importar Excel (`?tab=importar`) | importar: `unidades.importar` | ✅ |
| U2 | Descargar plantilla Excel | `unidades.importar` | ✅ |
| U3 | Buscar unidad o propietario (URL) | — | ✅ |
| U4 | Tabla paginada: unidad, propietario, tipo, DNI enmascarado, celular, inquilino, participación, deuda | — | ✅ tabla densa; deuda como punto rojo + monto |
| U5 | ✚ Filtro rápido «Solo morosos» | — | ✚ chip junto al buscador, en la URL (`?morosos=1`); filtra la página cargada (el API no tiene ese filtro) |
| U6 | Detalle de unidad (lateral): tipo, piso, participación, personas, historial, medidores | — | ✅ |
| U7 | Importar en 3 pasos: subir con progreso → validar (KPI, vista previa con errores por fila, «ver solo observaciones», «revisé las observaciones») → confirmar | `unidades.importar` | ✅ indicador de pasos animado |
| U8 | Bloqueos de la importación: errores, suma ≠ 100 %, observaciones sin revisar | — | ✅ |
| U9 | Resultado: creadas, actualizadas, personas, deudas iniciales; «Ver las unidades», «Importar otro archivo» | — | ✅ |
| U10 | Estado vacío con «Importar Excel» | `unidades.importar` | ✅ |

## 07 · Reservas (`pantallas/07-reservas/Reservas.jsx`, `NuevaReserva.jsx`, `ui/Calendario.jsx`)

| # | Función | Permiso | Ahora |
|---|---|---|---|
| V1 | Calendario semanal (recursos × días), semana anterior/siguiente (`?semana=`) | `reservas.ver` / `reservas.administrar` | ✅ |
| V2 | Móvil: selector de día y lista de franjas por recurso | — | ✅ carrusel con `scroll-snap` |
| V3 | Evento → modal con estado, unidad, día, cobro | — | ✅ |
| V4 | Validar pago y confirmar / Cancelar reserva (motivo) / No se presentó (motivo) | `reservas.administrar` | ✅ |
| V5 | Celda vacía → nueva reserva en ese recurso y día | `reservas.administrar` | ✅ |
| V6 | Nueva reserva (botón) | `reservas.administrar` | ✅ |
| V7 | Resumen de la semana (reservas, en línea, al recibo, por confirmar) y reglas del área | — | ✅ |
| V8 | Leyenda de estados | — | ✅ |
| V9 | Flujo de reservar: unidad (admin), área, recurso o «cualquiera libre», día (14), turno, forma de pago, acepto normas | `reservas.crear` | ✅ |
| V10 | Aviso 409 (otro reservó) y recarga; 403 MOROSO → pantalla de deuda con «Ver mi deuda» | — | ✅ |
| V11 | Reserva creada: código, retención de 15 min con cuenta regresiva, subir voucher + código, «Listo» | — | ✅ |
| V12 | «Hoy» marcado | — | ✅ + línea |

## 08 · Medidores (`pantallas/08-medidores/Medidores.jsx`, `Reparto.jsx`)

| # | Función | Permiso | Ahora |
|---|---|---|---|
| M1 | Ronda en orden, con avance «N de M» y barra | `lecturas.registrar`/`ver` | ✅ barra fina arriba |
| M2 | Ver reparto del recibo general | `lecturas.aprobar_reparto` | ✅ |
| M3 | «¡Listo!» al completar la ronda | — | ✅ |
| M4 | Captura: foto obligatoria (cámara trasera, compresión), ayuda «¿La cámara no abre?» | — | ✅ |
| M5 | Lectura con teclado numérico, anterior, consumo × tarifa, cargo | — | ✅ |
| M6 | Alerta PICO (confirmación) y NEGATIVO (motivo obligatorio) | — | ➜ EN LÍNEA, sin diálogo: PICO pide marcar «Revisé la foto…»; NEGATIVO, un motivo escrito; sin eso no se guarda (mismas reglas) |
| M7 | Observación | — | ✅ |
| M8 | Reintentar ante fallo de red (subida con 3 reintentos) | — | ✅ |
| M9 | Avance automático a la siguiente pendiente; «Guardar · sigue X» | — | ✅ |
| M10 | Caja de reparto 5.000/4.800/200 por participación | — | ✅ |
| M11 | Reparto: periodo, registrar recibo general (monto, consumo, foto), aprobar reparto, KPI, alertas, tabla, «cuadra / no cuadra» | `lecturas.aprobar_reparto` | ✅ |

## 09 · Mantenimiento (`pantallas/09-mantenimiento/Tablero.jsx`, `Reportar.jsx`, `lib/kanban.js`)

| # | Función | Permiso | Ahora |
|---|---|---|---|
| K1 | Filtros en la URL: criticidad, categoría, responsable, desde, hasta, buscar | — | ➜ barra de una línea: buscador + chips de criticidad (✚ multiselección) + menú «Filtros» (categoría, responsable, fechas) + chips activos removibles |
| K2 | Limpiar filtros | — | ✅ |
| K3 | Botón «Filtros (n)» en móvil | — | ➜ menú «Filtros» con contador (móvil y escritorio) |
| K4 | Registrar incidencia | `incidencias.reportar` | ✅ |
| K5 | 8 columnas con contador; en móvil pestañas con contador (`?col=`) | — | ✅ + suma de montos al pie de cada columna |
| K6 | Arrastrar y soltar con columna válida/ inválida | permiso de la transición | ✅ + elevación, `asentar`, temblor si el API rechaza |
| K7 | Botones de transición en cada tarjeta | por transición | ➜ botón «siguiente paso →» (al pasar el ratón o con foco en escritorio; siempre en táctil) + menú `⋯` de la tarjeta con Ver detalle y TODAS las transiciones permitidas |
| K8 | Descartar/Rechazar con motivo; confirmar «terminado» | — | ✅ |
| K9 | Movimiento optimista con reversión y alerta si el API rechaza | — | ✅ (+ toast) |
| K10 | Tarjeta: criticidad, código, título, monto, responsable, votos de la junta, fotos, antigüedad, avance | — | ➜ tarjeta compacta de 3 líneas (punto de criticidad + código · título · monto · responsable · icono foto/votos) |
| K11 | Detalle lateral: estado, criticidad, línea de tiempo, motivo, descripción, ubicación, categoría, reportado, responsable, presupuesto, junta, acciones | — | ✅ |
| K12 | Aviso de umbral de junta (votos necesarios) | — | ✅ (icono `Vote` + texto) |
| K13 | Vacío «Sin trabajos este mes» | — | ✅ |
| K14 | Reportar: hasta 5 fotos (≥ 1), tipo, dónde (sugerencias), qué pasa, progreso, enviado con código | `incidencias.reportar` | ✅ |
| K15 | Teclado: mover tarjeta sin ratón | — | ✅ (menú `⋯` y botón de siguiente paso son botones enfocables) |

## 10 · Portal (`pantallas/10-portal/Portal.jsx`)

| # | Función | Permiso | Ahora |
|---|---|---|---|
| P1 | Saludo, edificio, unidad | — | ✅ |
| P2 | Estado de cuenta (sin recibos / debes / en revisión / al día) con «Pagar» o «Ver recibo» | no inquilino | ✅ tarjeta más baja |
| P3 | Reserva pendiente de pago → Pagar | — | ✅ |
| P4 | Detalle de deuda por mes o por unidad | — | ✅ |
| P5 | Reservar | `reservas.crear` | ✅ |
| P6 | Reportar | `incidencias.reportar` | ✅ |
| P7 | Balance del mes (4 mini KPI) + «Ver detalle» | `balance.ver` | ✅ cuadrícula 2×2 compacta |
| P8 | Mis reportes con progreso por pasos | — | ✅ pasos animados |
| P9 | Mis reservas | — | ✅ |
| P10 | Mantenimiento del mes | — | ✅ |
| P11 | Normas del edificio (Abrir / Pronto) | — | ✅ |
| P12 | Último recibo | — | ✅ |

## 11 · Roles (`pantallas/11-roles/Roles.jsx`)

| # | Función | Permiso | Ahora |
|---|---|---|---|
| O1 | Pestañas Usuarios · Permisos por rol · Junta (`?tab=`) | `roles.administrar` | ✅ |
| O2 | Invitar persona (correo, rol) → enlace + copiar | — | ✅ |
| O3 | Conteo por rol | — | ✅ como pastillas; ahora cuenta bien con los roles del API (vienen con `codigo`, no `id`) |
| O4 | Tabla de usuarios: nombre, correo/unidad, rol (presidente), último ingreso, estado | — | ✅ |
| O5 | Cambiar rol (modal) | — | ➜ menú `⋯` de la fila → mismo modal |
| O6 | Desactivar/Reactivar con confirmación; «Tú» en la propia fila; error ULTIMO_ADMIN | — | ➜ menú `⋯` de la fila; confirmación y errores iguales |
| O7 | Matriz de permisos (escritorio) y acordeón por rol (móvil), leyenda, candado de «fijo» | — | ✅ con iconos (check/raya), columna y cabecera fijas |
| O8 | Junta: regla, umbral, miembros, presidente | — | ✅ |

## 12 · WhatsApp y chatbot (`pantallas/12-whatsapp/WhatsApp.jsx`, `Chatbot.jsx`)

| # | Función | Permiso | Ahora |
|---|---|---|---|
| W1 | Pestañas Bandeja · Enviar · Configuración | config: `whatsapp.configurar` | ✅ |
| W2 | Aviso de modo SIMULADO | — | ➜ franja fina ámbar persistente arriba |
| W3 | Enlace al simulador del chatbot | — | ✅ |
| W4 | Bandeja: filtro por estado (chips), buscar (URL) | — | ✅ |
| W5 | Mensaje: dirección, destinatario, unidad, teléfono, plantilla, estado, texto, fecha, quién, intención, error | — | ➜ lista de conversación (avatar con iniciales, estado con icono); el texto se ve en 2 líneas y «Ver mensaje completo» lo despliega; el resto de datos, igual |
| W6 | Enviar recibos del periodo (periodo, confirmación, aviso SIMULADO, sin teléfono) | `whatsapp.enviar` | ✅ |
| W7 | Mensaje individual: unidad o teléfono, plantilla, variables, validación | `whatsapp.enviar` | ✅ |
| W8 | «Solo lectura» si no puede enviar | — | ✅ |
| W9 | Configuración: simulado / evolution-go, URL, instancia, API key (se conserva) | `whatsapp.configurar` | ✅ |
| C1 | Chatbot: escribir como (unidad o teléfono), teléfono libre | `chatbot.probar` | ✅ |
| C2 | Conversación con hora, intención y «datos» desplegables; error en burbuja | — | ✅ burbujas con entrada animada |
| C3 | Sugerencias | — | ✅ |
| C4 | Indicador «escribiendo…» | — | ➜ tres puntos en CSS |
| C5 | Volver a WhatsApp | — | ✅ |

## 13 · Analítica (`pantallas/13-analitica/Analitica.jsx`, `Graficos.jsx`)

| # | Función | Ahora |
|---|---|---|
| N1 | Rango desde/hasta (mes) en la URL; atajos 3, 6, 12 meses | ✅ `SelectorMes` propio en español + chips «3 m · 6 m · 12 m» (el activo se marca) |
| N2 | 4 KPI: emitido, cobrado (% de lo emitido), morosidad del último mes, tiempo de resolución | ➜ franja compacta |
| N3 | Cobranza emitido vs cobrado (barras agrupadas, tooltip, «Ver como tabla») | ✅ paleta de series, barras que crecen |
| N4 | Morosidad mensual (línea, tooltip, tabla) | ✅ |
| N5 | Consumo de agua por unidad con resalte de picos (más del doble) | ✅ |
| N6 | Reservas por área con ingresos | ✅ |
| N7 | Incidencias por estado | ✅ colores del mapa de estados |
| N8 | Vacíos por gráfico, error, esqueletos | ✅ |

## Componentes compartidos (`src/ui/`) — API pública que se conserva

`Boton` (+`Spinner`), `Campo`, `Tabla` (+`Paginacion`), `TarjetaKPI`, `NodoDesplegable`, `SubirFoto`,
`SubirArchivo`, `Calendario` (+`LeyendaCalendario`), `Modal`, `useDialog`, `ToastProvider`/`useToast`,
`Insignia`, `Vacio`, `ErrorCarga`, `SinPermiso`, `Esqueleto`, `CargandoApp`, `SelectorPeriodo`,
`SelectorEdificio`, `Icono`, `Logo`/`Isotipo`; `estados.js` (`ESTADOS`, `TONO_CLASES`, `infoEstado`, `tonoDe`).
Mismas props y mismos nombres de export; lo nuevo se **añade**.

---

## Verificación al cerrar (plan §3)

| # | Comprobación | Resultado |
|---|---|---|
| 1 | Inventario marcado | Esta tabla, línea por línea |
| 2 | Recorrido `scripts/recorrido-ui.cjs` contra el API real (dev server 4720, login por el edge 4700) | **0 errores de consola, 0 del API** en las 18 vistas |
| 3 | Capturas «después» a 390 / 768 / 1024 / 1440 | `docs/capturas/despues/` (72) |
| 4 | Sin scroll horizontal a 360 px | 18/18 vistas (y a los cuatro anchos de captura) |
| 5 | Pruebas de la app | 77 en verde (63 de antes + mapa de estados, iconos, fecha, criticidad múltiple, cifras cortas) |
| 6 | Accesibilidad | Pares de texto AA; series de gráfico ≥ 3:1 contra blanco; `prefers-reduced-motion` comprobado (duraciones a 0,01 ms); 0 botones sin nombre en 11 pantallas |
| 7 | Peso | CSS 9,3 KB comprimido (≤ 40 KB). `dist` 724 KB frente a 624 KB: **+16 %, por encima del 10 %** (Lucide y los componentes nuevos) |
| 8 | Humo del sistema (`make smoke`) | No se corrió: toca Docker/API, fuera del alcance de esta rama |
