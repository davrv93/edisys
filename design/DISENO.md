# EDISYS — Diseño de interfaces

Lienzo: https://claude.ai/artifact/FyfD7psiPNCxcudrJT9PWe (privado hasta compartirlo desde el menú Share)

## Identidad

Sobria y confiable: se administra dinero ajeno. Neutros slate, un acento petróleo y un color de alerta.
Todo mapea a Tailwind (paleta por defecto, sin colores a medida).

| Rol | Hex | Tailwind |
|---|---|---|
| Acento petróleo (botones, activo, positivo) | `#155E75` | cyan-800 |
| Acento hover / texto sobre fondo claro | `#164E63` | cyan-900 |
| Acento fondo suave / borde | `#ECFEFF` / `#CFFAFE` | cyan-50 / cyan-100 |
| Acento sobre fondo oscuro | `#67E8F9` | cyan-300 |
| Tinta (texto, sidebar) | `#0F172A` | slate-900 |
| Superficie oscura secundaria | `#1E293B` / `#334155` | slate-800 / slate-700 |
| Texto secundario | `#475569` / `#334155` | slate-600 / slate-700 |
| Texto de apoyo | `#64748B` | slate-500 |
| Bordes | `#E2E8F0` / `#CBD5E1` | slate-200 / slate-300 |
| Fondo de la app | `#F8FAFC` | slate-50 |
| Superficie | `#FFFFFF` | white |
| **Alerta** (morosidad, crítico, vencido) | `#B91C1C`, fondo `#FEF2F2`, borde `#FECACA` | red-700 / red-50 / red-200 |
| Aviso (medio, pendiente, retenido) | `#B45309`, fondo `#FFFBEB`, borde `#FDE68A` | amber-700 / amber-50 / amber-200 |

Regla: "correcto/pagado" usa el acento petróleo, no verde, para que no dependa de distinguir rojo y verde.

## Tipografía

- **Fraunces** (Google Fonts, 500/600/700): títulos, logotipo y cifras grandes.
- **Public Sans** (Google Fonts, 400–700): interfaz y lectura; cifras con `font-variant-numeric: tabular-nums`.
- Escala: 12 / 14 / 16 / 18 / 20 / 24 / 30 / 36 / 48 / 60 px (text-xs … text-6xl).

## Escala y forma

- Espaciado en múltiplos de 4 px (p-3, p-4, p-5, gap-3, gap-4, gap-6, px-8…).
- Radios: rounded-lg (8 px) para campos y botones, rounded-xl (12 px) para tarjetas, rounded-full para chips.
- Botones y campos: 40–52 px de alto; en móvil, objetivos táctiles de 44 px o más.
- Escritorio: sidebar slate-900 de 248 px + barra superior de 72 px. Móvil: 390 × 844.
- Logotipo: cuadrado petróleo rounded-lg con tres pisos escalonados en blanco + "EDISYS" en Fraunces.

## Datos de demostración (coherentes entre pantallas)

- Edificio Demo, 24 departamentos (101–604), periodo Setiembre 2026.
- Emitido S/ 22.400 (cuotas 16.800 + agua 4.800 + común 200 + reservas 600); cobrado S/ 19.460; egresos S/ 18.950; saldo del mes S/ 510; banco S/ 34.120.
- Morosidad 13,1 %: Dpto 402 (S/ 1.420), 503 (S/ 760), 104 (S/ 760).
- Medidores: recibo general S/ 5.000 − departamentos S/ 4.800 = S/ 200 de áreas comunes, repartidos por participación. Tarifa demo S/ 14,00/m³.
- Recibo Dpto 201 (María Demo, 4,20 %): 705,60 + 196,00 + 8,40 + 80,00 = S/ 990,00.
- INC-014 Bomba de agua N.º 2, S/ 1.850, espera a la junta (2 de 3 votos necesarios; junta de 5).

## Pantallas (artboards)

| # | Pantalla | Archivo | Plataforma |
|---|---|---|---|
| 00 | Portada: identidad e índice | `Main.dc.html` | 1440 × 900 |
| 01 | Login (Qwik) | `01-login-escritorio.dc.html` / `01-login-movil.dc.html` | escritorio + móvil |
| 02 | Landing pública (Astro) | `02-landing.dc.html` | escritorio (1440 × 2320) |
| 03 | Dashboard del administrador | `03-dashboard.dc.html` | escritorio |
| 04 | Balance por nodos | `04-balance-nodos.dc.html` | escritorio |
| 05 | Recibos y facturación | `05-recibos.dc.html` | escritorio |
| 06 | Unidades, propietarios e importación Excel | `06-unidades-importacion.dc.html` | escritorio |
| 07 | Reservas: calendario / reserva con pago | `07-reservas-calendario.dc.html` / `07-reservas-propietario-movil.dc.html` | escritorio + móvil |
| 08 | Lectura de medidores del operario | `08-medidores-operario-movil.dc.html` | móvil |
| 09 | Mantenimiento: tablero / reporte con foto | `09-mantenimiento-tablero.dc.html` / `09-mantenimiento-reporte-movil.dc.html` | escritorio + móvil |
| 10 | Portal del propietario | `10-portal-propietario-movil.dc.html` | móvil |
| 11 | Roles y permisos | `11-roles-permisos.dc.html` | escritorio |
