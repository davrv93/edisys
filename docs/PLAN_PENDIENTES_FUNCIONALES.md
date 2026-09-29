# Plan de trabajo — Pendientes funcionales de EDISYS

**Fecha:** 28-09-2026 · **Estimado:** ~34 h en 7 bloques · **Ejecuta:** un agente, en orden
**Base:** `main` en `04fa010`. El stack corre en local en http://localhost:4700 (puertos 47xx).

**Reglas:**
- `go test ./...`, vitest, `make smoke` y `scripts/recorrido-ui.cjs` en verde al cerrar cada bloque.
- Nada sale a terceros de verdad:
  - el correo va a **Mailpit** local;
  - SUNAT, a un **OSE simulado** o al entorno **beta**;
  - WhatsApp sigue en **simulado**.
- Commit por bloque, sin push.
- No se toca el diseño visual (eso es la segunda pasada, `PLAN_SEGUNDA_PASADA_UI.md`): las pantallas nuevas usan los componentes existentes.

---

## 1. Deuda inicial en la morosidad (3 h)

**Hoy:** la deuda importada con el Excel se guarda, pero no entra en la morosidad ni en el estado de cuenta.

- La deuda inicial pasa a ser un **cargo por periodo** (`origen = 'deuda_inicial'`) en la misma cuenta corriente de la unidad. Entra en:
  - la antigüedad de la deuda;
  - el índice de morosidad;
  - el bloqueo de reservas;
  - el portal del propietario;
  - el chatbot («cuánto debo»).
- Los pagos se aplican **del más antiguo al más nuevo**.
- **Morosidad** = deuda vencida total (incluida la inicial) ÷ emitido del periodo. Se muestran aparte «del mes» e «histórica», para que setiembre siga dando **13,1 %** con la semilla actual.
- **Pruebas:**
  - Una unidad con S/ 1.200 de deuda de 2023 y el mes al día aparece como morosa, no puede reservar y el chatbot le dice cuánto debe.
  - Un pago de S/ 500 reduce primero lo más antiguo.

## 2. Corrección de lecturas en cascada (3 h)

**Hoy:** corregir una lectura no recalcula la «anterior» del mes siguiente.

- Al corregir la lectura del periodo *N*:
  - se recalcula el consumo de *N* y de *N+1*, porque la «anterior» de *N+1* es la corregida;
  - se rehace el reparto de los dos periodos (5.000 / 4.800 / 200).
- Si los recibos de esos periodos **ya están emitidos**, no se editan. Se genera un **ajuste** (nota de cargo o de abono interna) en el siguiente recibo, con el motivo.
- La corrección exige motivo y queda en la auditoría. Una lectura menor que la anterior sigue exigiendo confirmación («cambio de medidor»).
- **Pruebas:**
  - Corregir setiembre del 201 recalcula octubre.
  - Con setiembre emitido, genera el ajuste en octubre.
  - Los repartos siguen sumando exacto al céntimo.

## 3. PDF del balance y envío por correo (5 h)

- **PDF del balance** del periodo, generado por el API con el paquete `pdf` que ya existe:
  - portada con los 4 indicadores;
  - árbol de ingresos y egresos por rubro y concepto;
  - morosidad por unidad;
  - trabajos del mes.

  Endpoint `GET /api/v1/edificios/{eid}/balance/{periodo}.pdf`. El botón «Imprimir» de la pantalla 04 pasa a «Descargar PDF» (se conserva imprimir en el menú).
- **PDF del informe a la junta:** el balance, más los pendientes por criticidad y las aprobaciones del mes.
- **Correo:**
  - Servicio **Mailpit** en el compose (SMTP 4725, interfaz web **http://localhost:4726**).
  - En el API, `CORREO_MODO=smtp|simulado` y la configuración SMTP por entorno.
  - Tabla `correo_mensaje` como bandeja de salida, igual que WhatsApp.
  - Endpoints:
    - `POST /api/v1/edificios/{eid}/recibos/{periodo}/enviar-correo`: cada propietario recibe su recibo PDF adjunto.
    - `POST …/balance/{periodo}/enviar-correo`: la junta y los propietarios reciben el balance.
  - Plantillas HTML sencillas, en español y con los datos reales.
- **Pruebas:** 24 correos en Mailpit con su PDF adjunto; los PDF abren y sus cifras cuadran con el API.

## 4. Conciliación bancaria (6 h)

- **Subida del extracto:** CSV o XLSX. Columnas mapeables al subir (fecha, descripción, monto, código de operación); se guarda el mapeo por banco.
- **Emparejado automático, en este orden:**
  1. código de operación;
  2. monto exacto y fecha ±2 días;
  3. monto exacto sin fecha.

  Cada movimiento queda como **conciliado**, **sugerido** (a confirmar) o **sin pareja**. Del lado del sistema: los pagos (ingresos) y los egresos.
- **Pantalla nueva** «Conciliación», en `/app/conciliacion`, con permiso `balance.conciliar`:
  - dos columnas (banco | sistema);
  - resumen: saldo del banco, saldo del sistema y diferencia;
  - confirmar o deshacer parejas;
  - crear un egreso o ingreso desde un movimiento sin pareja.
- El balance muestra «Conciliado con el banco al 30/09» o la diferencia pendiente.
- **Datos de demo:** un extracto de setiembre que cuadre con los S/ 34.120 del banco, con 2 movimientos sin pareja a propósito.
- **Pruebas:** el emparejado por cada regla, la diferencia en cero tras confirmar y que no se pueda conciliar dos veces el mismo pago.

## 5. SUNAT: boleta y factura electrónica (8 h)

Sin credenciales reales. Se construye preparado para un OSE/PSE, probado en simulado.

- **Configuración** por edificio o administradora:
  - RUC, razón social y serie (`B001` boleta, `F001` factura);
  - modo `off | simulado | beta | produccion`;
  - proveedor OSE (URL y usuario) y certificado `.pfx` en el almacén de objetos.

  El certificado y su clave nunca se devuelven por el API.
- **Qué se emite:** por recibo, según el tipo de cliente: **boleta** a persona (DNI) y **factura** a empresa (RUC). La cuota de mantenimiento suele estar inafecta al IGV: es configurable por concepto (`gravado | exonerado | inafecto`).
- **Generación del XML UBL 2.1** (Invoice), firmado con el certificado, numeración correlativa sin huecos (bloqueo en la BD).
- **Modos:**
  - `simulado`: devuelve un CDR aceptado y guarda el XML.
  - `beta`: envía al entorno beta de SUNAT, solo si hay credenciales de prueba.
  - `produccion`: queda **deshabilitado** en esta entrega, con un aviso.
- **Anulación** por comunicación de baja o nota de crédito, según el caso.
- **La app:**
  - en el recibo, botón «Emitir comprobante», el estado del CDR y la descarga de XML y PDF con código QR;
  - en configuración, la pestaña «Facturación electrónica».
- **Pruebas:**
  - el XML valida contra el esquema UBL;
  - el correlativo no salta con 20 emisiones concurrentes;
  - boleta frente a factura según DNI o RUC;
  - una anulación.

## 6. Reservar desde el chatbot (4 h)

**Hoy:** el bot muestra la disponibilidad y manda a la app.

- **Conversación con estado** (tabla `chatbot_sesion` por teléfono, que caduca a los 15 min):
  1. «quiero reservar la parrilla el sábado»
  2. el bot ofrece las franjas libres, numeradas;
  3. el usuario elige «2»;
  4. el bot resume el área, la fecha, la hora y el costo, y pregunta «¿Confirmo?»;
  5. con «sí», crea la reserva **en retención de 15 min** y explica cómo pagar (voucher/Yape) o que va al recibo, según la regla del edificio.
- **Las mismas reglas que la app**, llamando al mismo servicio, no a una copia:
  - el moroso no reserva y el bot le dice cuánto debe;
  - no hay doble reserva (EXCLUDE);
  - horarios del reglamento.
- «cancelar» o «salir» en cualquier paso vuelve al menú. Fechas en lenguaje natural: hoy, mañana, el sábado, 12/10.
- Funciona igual desde el webhook de WhatsApp y desde el simulador de la app.
- **Pruebas:**
  - el flujo completo;
  - el moroso bloqueado;
  - dos personas reservando la misma franja a la vez por el bot: una gana y la otra recibe otras franjas;
  - la sesión caducada.

## 7. Respaldo y despliegue automáticos (5 h)

- **Servicio `backup`** en el compose (Alpine + cron):
  - `pg_dump` diario a las 02:00 de Lima;
  - copia del cubo de archivos de Garage;
  - rotación de 30 días en el volumen `edisys_respaldos`;
  - copia externa opcional a un S3 o R2 si hay credenciales (desactivada por defecto).
- `make backup` y `make restore FECHA=…`. Hay que **probar la restauración** en una base temporal y comparar el conteo de filas.
- **CI** (`.github/workflows/ci.yml`):
  - `go test`, vitest, lint;
  - build de las imágenes;
  - compose en el runner con `make smoke`.
- **Despliegue** (`.github/workflows/deploy.yml` + `scripts/desplegar.sh`):
  - imágenes a GHCR;
  - `ssh` al EC2 con `docker compose pull && up -d` y migraciones.

  Solo con el disparo manual (`workflow_dispatch`); **no se ejecuta**, porque no hay remoto ni EC2 todavía. Queda documentado en el README.
- **Pruebas:** el respaldo se genera, se restaura y los conteos coinciden.

---

## Orden y verificación final

| # | Bloque | Horas |
|---|---|---|
| 1 | Deuda inicial en morosidad | 3 |
| 2 | Lecturas en cascada | 3 |
| 3 | PDF del balance y correo | 5 |
| 4 | Conciliación bancaria | 6 |
| 5 | SUNAT (simulado/beta) | 8 |
| 6 | Reservar desde el chatbot | 4 |
| 7 | Respaldo y despliegue | 5 |
| | **Total** | **34** |

Al terminar:
- `make test`, `make smoke`, el recorrido UI (3 roles, 0 errores) y la nueva pantalla de conciliación en el recorrido;
- los correos visibles en http://localhost:4726;
- un respaldo restaurado.

A S/ 30 la hora, estos 34 h equivalen a **S/ 1.020** si se cotizan como ampliación.
