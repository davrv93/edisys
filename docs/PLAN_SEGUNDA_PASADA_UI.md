# Plan de trabajo — Segunda pasada de interfaz de EDISYS

**Fecha:** 28-09-2026 · **Estado:** listo para empezar · **Estimado:** ~40 h en 7 bloques
**Alcance:** solo `app/` (Astro + islas React), `login/` y `packages/tokens`. El API no se toca.
**Regla madre:** **no se pierde ninguna función.** Cada botón, filtro, estado y permiso de hoy sigue existiendo al terminar. Si algo se esconde, pasa a un menú, un atajo o un despliegue, nunca desaparece.

Capturas de partida: `docs/capturas/antes/` (18 vistas, tomadas contra el API real).
Recorrido de verificación: `scripts/recorrido-ui.cjs` (3 roles, 18 vistas, errores de consola y del API).

---

## 0. Qué se ve hoy (diagnóstico de las capturas)

| # | Problema | Dónde | Efecto |
|---|---|---|---|
| D1 | Tarjetas altas con mucho aire vacío | Dashboard (KPI de 180 px, «Morosidad por unidad» casi vacía), Portal | Hay que hacer scroll para ver lo importante |
| D2 | El panel de filtros ocupa dos filas (≈170 px) | Kanban de mantenimiento | La primera columna empieza a media pantalla |
| D3 | Botones de acción apilados dentro de cada tarjeta del kanban | Kanban | Tarjetas de 200 px; caben 3 por columna |
| D4 | Los campos de fecha salen como `mm/dd/yyyy`, en inglés | Kanban, Analítica, Reservas | Rompe el español de Perú y confunde día con mes |
| D5 | Seis KPI iguales compitiendo | Dashboard | Nada destaca; la morosidad se pierde |
| D6 | Movimiento casi nulo: 4 `transition-all`, 3 `animate-pulse` | Toda la app | Se siente rígida; al cargar, salto brusco |
| D7 | Iconos desiguales: 37 dibujados a mano, grosores distintos, algunos que no dicen nada (`llave` para roles, `robot` para chatbot) | Menú, botones | Cuesta reconocer las secciones de un vistazo |
| D8 | Menú lateral fijo de 248 px en todas las pantallas | Escritorio de 1024–1280 px | Le roba un 20 % de ancho a tablas y kanban |
| D9 | Colores de estado sin sistema: algunas insignias en ámbar, otras en rojo o gris sin regla visible | Kanban, Recibos | El color no ayuda a leer el estado |

---

## 1. Principios de la pasada

1. **Minimalismo con densidad.** Menos cajas, menos bordes y menos sombras, y más información por pantalla. Separar con espacio y tipografía, no con recuadros.
2. **Una jerarquía por pantalla.** Una cifra o acción principal; el resto, secundario.
3. **Movimiento con propósito.** Solo para orientar: qué entró, qué cambió, qué se movió. Siempre CSS y siempre desactivable con `prefers-reduced-motion`.
4. **Iconos que dicen algo.** Uno por concepto, el mismo en todas partes, y nunca solo: con texto o con `aria-label`.
5. **Color con significado.** El petróleo es la acción y lo cobrado; el rojo, la deuda y lo crítico; el ámbar, lo pendiente; el gris, lo neutro. Nada más compite.
6. **Móvil primero, de verdad.** Se diseña a 390 px y se amplía; no se encoge el escritorio.

---

## 2. Bloques de trabajo

### Bloque A — Tokens v2: espaciado, forma, sombra y movimiento (4 h)

En `packages/tokens/tokens.css` y `tailwind-preset.js`:

- **Densidad:** alturas de control de 36 px en escritorio y 44 px en móvil (objetivo táctil). Filas de tabla de 40 px. Padding de tarjeta de 16 px (hoy 24).
- **Radios:** 10 px en tarjetas, 8 px en controles, 999 px en chips. Un radio por nivel, no tres por pantalla.
- **Bordes y sombras:** borde de 1 px `borde` como separación por defecto. Una sola sombra suave para lo flotante (menús, diálogos y la tarjeta que se arrastra). Las tarjetas en reposo, sin sombra.
- **Movimiento:**
  - `--dur-rapida: 120ms` (hover, presión), `--dur-media: 200ms` (entradas, despliegues), `--dur-lenta: 320ms` (diálogos, cambio de columna).
  - `--ease-salida: cubic-bezier(.2,.8,.2,1)`, `--ease-entrada: cubic-bezier(.4,0,1,1)`.
  - Un bloque `@media (prefers-reduced-motion: reduce)` que lleva todas las duraciones a 0.
- **Keyframes con nombre en el preset:** `aparecer` (opacidad + 6 px hacia arriba), `desplegar`, `brillo` (esqueleto con barrido en lugar del `pulse`), `latido-suave` (un punto de «en vivo») y `asentar` (la tarjeta que cae en una columna).
- **Tipografía compacta:** cuerpo de 14 px en escritorio y 15 px en móvil. Cifras KPI de 24–28 px (hoy 32+) en Fraunces con `tabular-nums`. Títulos de pantalla de 22 px.

**Listo cuando:** ninguna pantalla usa valores sueltos de duración, sombra o radio, y el lint (regla nueva) lo comprueba.

### Bloque B — Paleta refinada (3 h)

Se mantiene la identidad: petróleo `#155E75` y slate. Se añade lo que falta para leer estados y gráficos:

| Rol | Color | Uso |
|---|---|---|
| Acción / cobrado | petróleo 800 / 50 | Botón principal, «pagado», barras de cobrado |
| Crítico / deuda | rojo 700 / 50 | Moroso, vencido, crítico |
| Pendiente | ámbar 700 / 50 | Por validar, esperando a la junta |
| En curso | índigo 600 / 50 | En ejecución, reserva de hoy |
| Hecho / neutro | slate 500 / 100 | Terminado, archivado |
| Series de gráfico | petróleo 700, cian 400, índigo 400, ámbar 500, slate 400 | Siempre en este orden; se distinguen también por luminosidad, no solo por tono |

- **Mapa único de estados** en `ui/estados.js`: cada estado (reportado, validado, presupuestado, aprobado, en ejecución, terminado, rechazado, descartado, pagado, vencido, parcial, moroso, simulado, enviado, error) con su **color, icono y texto**. Las insignias, las columnas del kanban y la leyenda de los gráficos leen de ahí. Así se resuelve D9.
- Contraste AA verificado para texto sobre cada fondo suave. Se ajusta el ámbar de texto si no llega.
- **Modo oscuro:** solo se preparan las variables. No se activa en esta pasada (queda fuera, ver §4).

### Bloque C — Iconos (4 h)

- **Se cambian los 37 iconos dibujados a mano por [Lucide](https://lucide.dev)** (`lucide-react`, con árbol de importación: solo pesa lo que se usa). Trazo de 1,75 px, tamaños 16/18/20. `ui/Icono.jsx` se queda como envoltorio con el mismo nombre en español, para no tocar las pantallas.
- **Mapa de iconos por concepto** (uno por idea, el mismo en toda la app):

| Concepto | Hoy | Nuevo (Lucide) |
|---|---|---|
| Resumen | inicio | `LayoutDashboard` |
| Balance | balance | `Scale` |
| Recibos y cobranza | recibo | `ReceiptText` |
| Unidades | edificio | `Building2` |
| Reservas | calendario | `CalendarCheck` |
| Medidores | medidor | `Gauge` |
| Mantenimiento | herramienta | `Wrench` |
| WhatsApp | whatsapp | `MessageCircle` |
| Chatbot | robot | `Bot` |
| Analítica | grafico | `ChartColumn` |
| Roles y permisos | llave | `ShieldCheck` |
| Foto / cámara | camara | `Camera` |
| Voucher / pago | — | `BadgeCheck` |
| Moroso | — | `CircleAlert` |
| Crítico | alerta | `TriangleAlert` |
| Esperando a la junta | — | `Vote` |
| Arrastrar tarjeta | arrastrar | `GripVertical` |
| Importar Excel | subir | `FileSpreadsheet` |

- Cada estado del mapa del Bloque B lleva su icono (p. ej. `Clock` pendiente, `Loader` en ejecución, `CheckCircle2` terminado). El color nunca va solo: el icono lo refuerza, también para daltónicos.
- **Botones solo de icono** (cerrar, siguiente mes, menú) con `aria-label` y tooltip propio, sin `title` nativo.

**Listo cuando:** `grep` no encuentra ningún `<svg>` suelto en las pantallas y todos los botones de icono tienen `aria-label`.

### Bloque D — Armazón y responsividad (6 h)

- **Menú lateral en tres estados:**
  - ≥ 1280 px: abierto (220 px, hoy 248).
  - 1024–1279 px: **solo iconos** (64 px), con el nombre en un tooltip.
  - < 1024 px: cajón que se abre con el botón de menú.

  Se recuerda la elección del usuario (`localStorage` con try/catch). Resuelve D8.
- **Barra inferior en móvil para todos los roles** (hoy solo el portal): 4 accesos según el rol + «Más». Por ejemplo, el administrador tiene Resumen · Recibos · Mantenimiento · WhatsApp · Más, y el operario Medidores · Reportar · Más.
- **Encabezado de pantalla compacto:** 56 px (hoy 72). El título a la izquierda; a la derecha la acción principal y un menú `⋯` con las secundarias (Imprimir, Exportar…). Ninguna acción se pierde: pasan al menú.
- **Cortes:** 390 / 768 / 1024 / 1280 / 1440. Cada pantalla se revisa en los cinco.
- **Tablas:** en móvil pasan a lista de dos líneas (ya existe) con la cifra alineada a la derecha. En tablet, las columnas secundarias se ocultan por prioridad.
- **Kanban en tablet y móvil:** scroll horizontal con `scroll-snap` por columna y encabezado de columna fijo. En móvil sigue la lista con pestañas, ahora con el contador de cada estado en la pestaña.

### Bloque E — Pantalla por pantalla (13 h)

Por cada pantalla: qué cambia y qué **debe seguir existiendo** (la lista de «no perder» se revisa al final).

**03 Dashboard (2 h)**
- Los 6 KPI pasan a **una franja compacta**: la morosidad en grande y a la izquierda (es lo que más importa), y cobrado, egresos, saldo, banco y reservas en fila, con su variación como flecha pequeña. Altura total ≈ 96 px (hoy 180). Resuelve D1 y D5.
- «Morosidad por unidad», hoy casi vacía, se fusiona con «Cobranza de setiembre» en una tarjeta: barra de avance y debajo los 3 morosos en línea (Dpto, monto, días).
- «Tareas de hoy» pasa a lista con icono de estado, no tres cajas de color.
- *No perder:* cambiar de periodo, Imprimir, Ir a recibos, enlaces de cada KPI, detalle de trabajos del mes.

**04 Balance por nodos (1,5 h)**
- Filas de nodo de 40 px con guía vertical fina para la profundidad. El monto a la derecha con `tabular-nums` y una barra mínima de proporción (el % del padre).
- Despliegue animado (`grid-template-rows: 0fr → 1fr`, 200 ms). El chevron gira.
- *No perder:* expandir todo, `?abrir=`, visor de documentos, registrar egreso, imprimir, teclado.

**05 Recibos (1,5 h)**
- Filtros como **chips** (Todos · Pagados · Vencidos · Parciales) con contador, en una sola línea junto al buscador.
- El detalle del recibo se abre en **panel lateral** en escritorio (la lista no se pierde de vista) y en hoja completa en móvil.
- *No perder:* generar borradores, emitir, pago con voucher, pago parcial, anular, enviar por correo y WhatsApp, foto del medidor, PDF.

**06 Unidades (1 h)**
- Tabla densa con la deuda como punto rojo + monto. Filtro rápido «Solo morosos».
- La importación Excel en 3 pasos con indicador de pasos animado.
- *No perder:* buscar, detalle, plantilla, validación del 100 %, deuda inicial.

**07 Reservas (1 h)**
- Calendario con franjas más finas y el color por área (de la paleta de series). «Hoy» marcado con una línea.
- En móvil, selector de día en carrusel horizontal y franjas como botones grandes.
- *No perder:* nueva reserva, retención de 15 min, voucher, avisos 409/403.

**08 Medidores (1 h)**
- Una unidad por pantalla, la foto grande y el campo de lectura con teclado numérico. Barra de progreso fina arriba («14 de 24») y avance automático a la siguiente.
- Alerta de pico o negativo en línea, no en diálogo.
- *No perder:* foto obligatoria, compresión, reintentos, reparto 5.000/4.800/200.

**09 Mantenimiento — kanban (2 h)**
- **Barra de filtros en una línea** (≈ 48 px, resuelve D2):
  - buscador + chips de criticidad (Crítico · Medio · Bajo, multiselección);
  - menú «Filtros» con categoría, responsable y rango de fechas;
  - los filtros activos se ven como chips removibles y hay un botón «Limpiar».

  Siguen viviendo en la URL.
- **Tarjeta compacta** (≈ 110 px, resuelve D3):
  - línea 1: criticidad como punto de color + código;
  - línea 2: título;
  - línea 3: monto · responsable · icono de foto o de votos.

  Las acciones pasan a un menú `⋯` y a un botón que aparece al pasar el ratón («Aprobar →»). En táctil, siempre visibles en pequeño.
- **Arrastre:** la tarjeta se eleva (sombra flotante, 2° de giro), la columna válida se ilumina y la inválida se atenúa; al soltar, animación `asentar`. Si el API la rechaza, vuelve a su columna con un temblor corto y un toast.
- Contador por columna y suma de montos al pie de la columna.
- *No perder:* todos los filtros, transiciones válidas, descartar con motivo, aviso de umbral de junta, reportar desde móvil, 14 trabajos visibles.

**10 Portal del propietario (1 h)**
- Se mantiene la estructura (ya es la mejor pantalla). Tarjeta de estado de cuenta más baja. Los 4 indicadores en una cuadrícula de 2×2 más compacta. Progreso de los reportes con pasos animados.
- *No perder:* ver recibo, reservar, reportar, balance, detalle.

**11 Roles (0,5 h)**
- La matriz de permisos con iconos de check/raya en lugar de texto, columnas fijas y fila de rol pegada al hacer scroll.

**12 WhatsApp y chatbot (0,5 h)**
- Bandeja como lista de conversación (avatar con iniciales, último mensaje, estado con icono). El aviso de MODO SIMULADO pasa a una franja fina persistente arriba, en ámbar.
- Chatbot: burbujas con entrada animada y un indicador «escribiendo…» de tres puntos (CSS).

**13 Analítica (0,5 h)**
- Gráficos con la paleta de series, rejilla más tenue, etiquetas directas en lugar de leyenda cuando caben. Barras que crecen al entrar (una vez, 320 ms).
- *No perder:* rango, vista de tabla de cada gráfico.

**01 Login (0,5 h)**
- Panel de marca más estrecho, formulario centrado con más aire y transición suave al enviar.

### Bloque F — Movimiento, componente por componente (4 h)

Todo en CSS, con los tokens del Bloque A; nada de librerías de animación:

| Componente | Animación |
|---|---|
| Cambio de pantalla | `aparecer` del contenido principal, 200 ms, con escalonado de 30 ms en las tarjetas (máx. 6) |
| Esqueleto de carga | `brillo` con barrido (reemplaza `animate-pulse`) y la misma forma que el contenido que viene |
| Botón | Presión `scale(.98)`, 120 ms; hover solo de color |
| Modal / diálogo | Fondo que funde, panel `scale(.97) → 1` + opacidad, 200 ms; al cerrar, 120 ms |
| Panel lateral (detalle) | Entra desde la derecha 16 px + opacidad |
| Toast | Entra desde abajo; barra de tiempo restante |
| Nodo desplegable | `grid-template-rows` 0fr→1fr; chevron que gira 90° |
| Chips de filtro | Aparecen/desaparecen con escala |
| Kanban | Elevación al arrastrar, `asentar` al soltar, temblor si se rechaza |
| Cifras KPI | Sin conteo animado (distrae y retrasa leer); solo `aparecer` |
| Gráficos | Barras y líneas crecen una vez al entrar |
| Foco de teclado | Anillo petróleo de 2 px con `outline-offset`, sin animación |

- Todo respeta `prefers-reduced-motion`: se ve igual, pero sin moverse.
- Presupuesto: ninguna animación bloquea la interacción ni dura más de 320 ms.

### Bloque G — Detalles de precisión (3 h)

- **Fechas en español (D4):** un `SelectorFecha` propio (`dd/mm/aaaa`, calendario desplegable en español, lunes primero) en lugar del `<input type="date">` nativo. En móvil, fallback al nativo si el navegador lo muestra en español.
- **Cifras:** siempre `S/ 4.800,00` con `tabular-nums`; en la franja de KPI, `S/ 19,5 mil` cuando no cabe, con el valor completo en el tooltip.
- **Estados vacíos** con icono de Lucide, una frase y la acción siguiente.
- **Truncado:** títulos largos con `line-clamp-2` y el texto completo en el `title` del detalle.
- **Foco y teclado:** orden de tabulación revisado en el kanban (mover con teclado) y en los diálogos.

---

## 3. Cómo se verifica que no se perdió nada

1. **Inventario de funciones** (`docs/INVENTARIO_FUNCIONES.md`): se escribe **antes** de tocar nada. Es una lista por pantalla de cada botón, filtro, estado y permiso de hoy, sacada del código. Al terminar, cada línea se marca con dónde vive ahora.
2. **Recorrido automático** `scripts/recorrido-ui.cjs` contra el API real, con 3 roles y 18 vistas: **0 errores de consola y 0 errores del API** (hoy da 0).
3. **Capturas antes y después** a 390, 768, 1024 y 1440 px en `docs/capturas/despues/`, lado a lado.
4. **Sin scroll horizontal** a 360 px en ninguna vista.
5. **Pruebas de la app:** las 63 de vitest siguen en verde, y se añaden las del mapa de estados (cada estado tiene color, icono y texto).
6. **Accesibilidad:** contraste AA en todos los pares de color; `prefers-reduced-motion` comprobado; todos los botones de icono con nombre.
7. **Peso:** el CSS final ≤ 40 KB comprimido, y los iconos de Lucide solo por importación nombrada. El `dist` de hoy (616 KB) no crece más de un 10 %.
8. **Humo del sistema:** `make smoke` sigue 16/16.

---

## 4. Fuera de esta pasada

- Modo oscuro (las variables quedan listas).
- Cambios en el API o en las reglas de negocio.
- Nuevas funciones (PDF del balance, conciliación, SUNAT): siguen como ampliaciones de la cotización.

---

## 5. Orden y horas

| Orden | Bloque | Horas | Depende de |
|---|---|---|---|
| 0 | Inventario de funciones + capturas «antes» (ya hechas) | 1,5 | — |
| 1 | A · Tokens v2 | 4 | 0 |
| 2 | B · Paleta y mapa de estados | 3 | A |
| 3 | C · Iconos Lucide | 4 | A |
| 4 | D · Armazón y responsividad | 6 | A, C |
| 5 | F · Movimiento (componentes base) | 4 | A |
| 6 | E · Pantallas (03 → 13, 01) | 13 | B, C, D, F |
| 7 | G · Precisión (fechas, cifras, vacíos, foco) | 3 | E |
| 8 | Verificación §3 y capturas «después» | 1 | todo |
| | **Total** | **~40 h** | |

Los bloques B, C y F pueden ir en paralelo una vez cerrado A. Las pantallas (E) se reparten en dos grupos sin archivos en común (03–07 y 08–13 + 01) para trabajar a la vez.
