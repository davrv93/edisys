package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/conciliacion"
	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Recaudadora externa (bloque A1): el recibo se paga por código en banco, agentes o Yape y la recaudadora
// entrega dos reportes como archivo (CSV o XLSX, columnas mapeables): transacciones y liquidaciones.
// Idempotente por código de operación; cada transacción acreditada es un pago validado del recibo y la
// comisión de cada liquidación entra como egreso. Sin API: si el proveedor la diera, iría como adaptador aparte.

// MapeoTransacciones: qué cabecera del archivo es cada dato del reporte de transacciones.
type MapeoTransacciones struct {
	CodigoRecibo    string `json:"codigo_recibo"`
	Monto           string `json:"monto"`
	CodigoOperacion string `json:"codigo_operacion"`
	Fecha           string `json:"fecha"`
	Medio           string `json:"medio,omitempty"`
}

// MapeoLiquidaciones: ídem para el reporte de liquidaciones. Comisión y neto son opcionales.
type MapeoLiquidaciones struct {
	Codigo   string `json:"codigo,omitempty"`
	Fecha    string `json:"fecha"`
	Bruto    string `json:"bruto"`
	Comision string `json:"comision,omitempty"`
	Neto     string `json:"neto,omitempty"`
}

// ComisionCts calcula la comisión pactada: fijo + porcentaje (puntos básicos) del bruto, redondeo
// al céntimo más cercano, nunca más que el bruto. Pura: se prueba sola.
func ComisionCts(bruto int64, pbs int, fijo int64) int64 {
	if bruto <= 0 {
		return 0
	}
	c := fijo + (bruto*int64(pbs)+5000)/10000
	if c > bruto {
		c = bruto
	}
	if c < 0 {
		c = 0
	}
	return c
}

// NetoLiquidacion: neto = bruto − comisión (la misma regla que la base exige con un CHECK).
func NetoLiquidacion(bruto, comision int64) int64 { return bruto - comision }

// mensajeDe saca el texto para el usuario de un error de negocio.
func mensajeDe(err error) string {
	var pe *P.Error
	if errors.As(err, &pe) {
		return pe.Mensaje
	}
	return err.Error()
}

func siVistaPrevia(r *http.Request) bool {
	v := strings.ToLower(campo(r, "vista_previa"))
	return v == "1" || v == "true" || v == "si"
}

// ---------- cuentas y configuración ----------

// listarCuentasRecaudadora: GET /recaudadora/cuentas
func (s *Server) listarCuentasRecaudadora(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT c.id, c.proveedor, c.codigo_convenio, c.medio, c.activo,
			COALESCE(k.porcentaje_pbs,0) AS porcentaje_pbs, COALESCE(k.fijo_cts,0) AS fijo_cts, k.rubro_id,
			(SELECT count(*) FROM recaudadora_transaccion t WHERE t.cuenta_recaudadora_id=c.id AND t.estado='acreditada') AS acreditadas,
			(SELECT count(*) FROM recaudadora_transaccion t WHERE t.cuenta_recaudadora_id=c.id AND t.estado='observada') AS observadas
		FROM cuenta_recaudadora c LEFT JOIN recaudadora_comision_config k ON k.cuenta_recaudadora_id=c.id
		WHERE c.edificio_id=$1 ORDER BY c.activo DESC, c.proveedor`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

type entradaCuentaRecaudadora struct {
	Proveedor      string `json:"proveedor"`
	CodigoConvenio string `json:"codigo_convenio"`
	Medio          string `json:"medio"`
	Activo         *bool  `json:"activo"`
	PorcentajePbs  int    `json:"porcentaje_pbs"`
	FijoCts        int64  `json:"fijo_cts"`
	RubroID        int64  `json:"rubro_id"`
}

func (s *Server) validarCuentaRecaudadora(ctx context.Context, eid int64, in *entradaCuentaRecaudadora) *P.Error {
	ev := P.Validacion("Revisa la cuenta recaudadora.")
	in.Proveedor = strings.TrimSpace(in.Proveedor)
	in.CodigoConvenio = strings.TrimSpace(in.CodigoConvenio)
	if in.Proveedor == "" {
		ev.Campo("proveedor", "Escribe el proveedor (banco, agente o billetera).")
	}
	switch in.Medio {
	case "":
		in.Medio = "deposito"
	case "yape", "plin", "transferencia", "efectivo", "deposito", "tarjeta":
	default:
		ev.Campo("medio", "Elige yape, plin, transferencia, efectivo, deposito o tarjeta.")
	}
	if in.PorcentajePbs < 0 || in.PorcentajePbs > 10000 {
		ev.Campo("porcentaje_pbs", "Entre 0 y 100 %.")
	}
	if in.FijoCts < 0 {
		ev.Campo("fijo_cts", "No puede ser negativo.")
	}
	if in.RubroID > 0 {
		var ok bool
		_ = s.DB.QueryRow(ctx, `SELECT true FROM rubro WHERE id=$1 AND edificio_id=$2`, in.RubroID, eid).Scan(&ok)
		if !ok {
			ev.Campo("rubro_id", "Ese rubro no es del edificio.")
		}
	}
	if len(ev.Campos) > 0 {
		return ev
	}
	return nil
}

func rubroOpc(id int64) *int64 {
	if id > 0 {
		return &id
	}
	return nil
}

// crearCuentaRecaudadora: POST /recaudadora/cuentas
func (s *Server) crearCuentaRecaudadora(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	var in entradaCuentaRecaudadora
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ev := s.validarCuentaRecaudadora(ctx, e.ID, &in); ev != nil {
		P.Fallo(w, r, ev)
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO cuenta_recaudadora (edificio_id, proveedor, codigo_convenio, medio) VALUES ($1,$2,$3,$4) RETURNING id`,
		e.ID, in.Proveedor, in.CodigoConvenio, in.Medio).Scan(&id); err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("RECAUDADORA_DUPLICADA", "Ya existe esa recaudadora con ese convenio."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `INSERT INTO recaudadora_comision_config (cuenta_recaudadora_id, porcentaje_pbs, fijo_cts, rubro_id) VALUES ($1,$2,$3,$4)`,
		id, in.PorcentajePbs, in.FijoCts, rubroOpc(in.RubroID)); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// editarCuentaRecaudadora: PUT /recaudadora/cuentas/{id}
func (s *Server) editarCuentaRecaudadora(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	var in entradaCuentaRecaudadora
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ev := s.validarCuentaRecaudadora(ctx, e.ID, &in); ev != nil {
		P.Fallo(w, r, ev)
		return
	}
	activo := true
	if in.Activo != nil {
		activo = *in.Activo
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE cuenta_recaudadora SET proveedor=$1, codigo_convenio=$2, medio=$3, activo=$4 WHERE id=$5 AND edificio_id=$6`,
		in.Proveedor, in.CodigoConvenio, in.Medio, activo, id, e.ID)
	if err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("RECAUDADORA_DUPLICADA", "Ya existe esa recaudadora con ese convenio."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		P.Fallo(w, r, P.NoEncontrado("la cuenta recaudadora"))
		return
	}
	if _, err := tx.Exec(ctx, `INSERT INTO recaudadora_comision_config (cuenta_recaudadora_id, porcentaje_pbs, fijo_cts, rubro_id) VALUES ($1,$2,$3,$4)
		ON CONFLICT (cuenta_recaudadora_id) DO UPDATE SET porcentaje_pbs=EXCLUDED.porcentaje_pbs, fijo_cts=EXCLUDED.fijo_cts,
			rubro_id=EXCLUDED.rubro_id, actualizado_en=now()`, id, in.PorcentajePbs, in.FijoCts, rubroOpc(in.RubroID)); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

// cuentaRecaudadora carga la cuenta del edificio con su configuración de comisión.
type cuentaRec struct {
	ID        int64
	Proveedor string
	Medio     string
	MapeoTx   []byte
	MapeoLiq  []byte
	Pbs       int
	FijoCts   int64
	RubroID   *int64
}

func (s *Server) cuentaRecaudadora(ctx context.Context, eid, id int64) (*cuentaRec, error) {
	var c cuentaRec
	err := s.DB.QueryRow(ctx, `SELECT c.id, c.proveedor, c.medio, c.mapeo_transacciones, c.mapeo_liquidaciones,
			COALESCE(k.porcentaje_pbs,0), COALESCE(k.fijo_cts,0), k.rubro_id
		FROM cuenta_recaudadora c LEFT JOIN recaudadora_comision_config k ON k.cuenta_recaudadora_id=c.id
		WHERE c.id=$1 AND c.edificio_id=$2`, id, eid).Scan(&c.ID, &c.Proveedor, &c.Medio, &c.MapeoTx, &c.MapeoLiq, &c.Pbs, &c.FijoCts, &c.RubroID)
	if err != nil {
		return nil, P.NoEncontrado("la cuenta recaudadora")
	}
	return &c, nil
}

// elegirColumna: lo que mandó el usuario, si no lo guardado, si no lo adivinado por la cabecera.
func elegirColumna(r *http.Request, nombreForm, guardado string, cab []string, pistas ...string) string {
	if v := campo(r, nombreForm); v != "" {
		return v
	}
	if guardado != "" && columnaDe(cab, guardado) >= 0 {
		return guardado
	}
	return adivinarColumna(cab, pistas...)
}

// leerReporte lee el archivo subido (CSV o XLSX) como cabeceras y filas.
func leerReporte(r *http.Request) (string, []string, [][]string, error) {
	nombre, datos, err := archivoSubido(r)
	if err != nil {
		return "", nil, nil, err
	}
	cab, filas, err := conciliacion.Leer(nombre, datos)
	if err != nil {
		return "", nil, nil, P.Err(http.StatusUnprocessableEntity, "ARCHIVO_INVALIDO", err.Error()).Campo("archivo", err.Error())
	}
	return nombre, cab, filas, nil
}

// ---------- importar transacciones ----------

// importarTransaccionesRecaudadora: POST /recaudadora/cuentas/{id}/transacciones/importar
// (multipart {archivo, col_codigo_recibo, col_monto, col_codigo_operacion, col_fecha, col_medio?, vista_previa?}).
// Con vista_previa=1 hace todo dentro de una transacción que se deshace: el resultado es exacto y no guarda nada.
func (s *Server) importarTransaccionesRecaudadora(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	cu, err := s.cuentaRecaudadora(ctx, e.ID, idURL(r, "id"))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	nombre, cab, filas, err := leerReporte(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var guardado MapeoTransacciones
	_ = json.Unmarshal(cu.MapeoTx, &guardado)
	m := MapeoTransacciones{
		CodigoRecibo:    elegirColumna(r, "col_codigo_recibo", guardado.CodigoRecibo, cab, "recibo", "codigo pago", "cod pago", "referencia", "codigo cliente"),
		Monto:           elegirColumna(r, "col_monto", guardado.Monto, cab, "monto", "importe", "pagado"),
		CodigoOperacion: elegirColumna(r, "col_codigo_operacion", guardado.CodigoOperacion, cab, "operacion", "nro op", "n op", "transaccion"),
		Fecha:           elegirColumna(r, "col_fecha", guardado.Fecha, cab, "fecha"),
		Medio:           elegirColumna(r, "col_medio", guardado.Medio, cab, "medio", "canal"),
	}
	ix := map[string]int{"codigo_recibo": columnaDe(cab, m.CodigoRecibo), "monto": columnaDe(cab, m.Monto),
		"codigo_operacion": columnaDe(cab, m.CodigoOperacion), "fecha": columnaDe(cab, m.Fecha)}
	ev := P.Validacion("Indica qué columna es cada dato.")
	for k, v := range ix {
		if v < 0 {
			ev.Campo("col_"+k, "Elige la columna.")
		}
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev.Con("cabeceras", cab).Con("mapeo", m))
		return
	}
	iMedio := columnaDe(cab, m.Medio)
	previa := siVistaPrevia(r)
	uid := ses(r).UsuarioID

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	conteo := map[string]int{"acreditada": 0, "duplicada": 0, "observada": 0, "error": 0}
	var acreditado int64
	resultado := []map[string]any{}
	for i, fila := range filas {
		f := map[string]any{"fila": i + 2, "codigo_recibo": celda(fila, ix["codigo_recibo"]), "codigo_operacion": celda(fila, ix["codigo_operacion"])}
		if strings.TrimSpace(strings.Join(fila, "")) == "" {
			continue
		}
		est, motivo := s.procesarTransaccion(ctx, tx, e.ID, cu, uid, nombre, fila, ix, iMedio, f)
		f["estado"], f["motivo"] = est, motivo
		conteo[est]++
		if est == "acreditada" {
			acreditado += f["monto_cts"].(int64)
		}
		resultado = append(resultado, f)
	}
	if !previa {
		b, _ := json.Marshal(m)
		if _, err := tx.Exec(ctx, `UPDATE cuenta_recaudadora SET mapeo_transacciones=$1 WHERE id=$2`, b, cu.ID); err != nil {
			P.Fallo(w, r, err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"vista_previa": previa, "archivo": nombre, "mapeo": m, "cabeceras": cab,
		"acreditadas": conteo["acreditada"], "duplicadas": conteo["duplicada"], "observadas": conteo["observada"], "errores": conteo["error"],
		"monto_acreditado_cts": acreditado, "filas": resultado})
}

// procesarTransaccion aplica una fila del reporte. Estados: acreditada (creó pago), duplicada (ya estaba
// acreditada: no hace nada), observada (se guarda sin pago, con motivo) y error (fila ilegible: no se guarda).
func (s *Server) procesarTransaccion(ctx context.Context, tx pgx.Tx, eid int64, cu *cuentaRec, uid int64, archivoNombre string,
	fila []string, ix map[string]int, iMedio int, f map[string]any) (string, string) {
	codRecibo := celda(fila, ix["codigo_recibo"])
	codOp := celda(fila, ix["codigo_operacion"])
	monto, errM := conciliacion.MontoCts(celda(fila, ix["monto"]))
	fecha, errF := conciliacion.Fecha(celda(fila, ix["fecha"]))
	medioTexto := celda(fila, iMedio)
	f["monto_cts"], f["fecha"] = monto, fecha
	switch {
	case codOp == "":
		return "error", "Falta el código de operación."
	case errM != nil || monto <= 0:
		return "error", "Monto inválido."
	case errF != nil:
		return "error", "Fecha inválida."
	case codRecibo == "":
		return "error", "Falta el código del recibo."
	}
	var previo string
	err := tx.QueryRow(ctx, `SELECT estado FROM recaudadora_transaccion WHERE cuenta_recaudadora_id=$1 AND codigo_operacion=$2 FOR UPDATE`, cu.ID, codOp).Scan(&previo)
	if err == nil && previo == "acreditada" {
		return "duplicada", "Ya se acreditó en una carga anterior."
	}
	sp, err := tx.Begin(ctx) // punto de guardado: una fila que falla no arrastra a las demás
	if err != nil {
		return "error", err.Error()
	}
	var reciboID, pagoID *int64
	estado, motivo := "acreditada", ""
	obj, err := resolverRecibo(ctx, sp, eid, codRecibo)
	if err == nil {
		reciboID = &obj.ID
		f["recibo"], f["unidad"] = obj.Numero, obj.Unidad
		var apls []Aplicacion
		apls, err = s.acreditarRecibo(ctx, sp, eid, obj, &uid, monto, medioDePago(medioTexto, cu.Medio), codOp, fecha)
		if err == nil && len(apls) > 0 {
			pagoID = &apls[0].PagoID
			f["aplicaciones"] = apls
		}
	}
	if err != nil {
		_ = sp.Rollback(ctx)
		estado, motivo = "observada", mensajeDe(err)
		pagoID = nil
	} else if err := sp.Commit(ctx); err != nil {
		return "error", err.Error()
	}
	if _, err := tx.Exec(ctx, `INSERT INTO recaudadora_transaccion (edificio_id, cuenta_recaudadora_id, codigo_recibo, recibo_id, monto_cts, medio,
			codigo_operacion, fecha, estado, motivo, pago_id, archivo_nombre, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (cuenta_recaudadora_id, codigo_operacion) DO UPDATE SET codigo_recibo=EXCLUDED.codigo_recibo, recibo_id=EXCLUDED.recibo_id,
			monto_cts=EXCLUDED.monto_cts, medio=EXCLUDED.medio, fecha=EXCLUDED.fecha, estado=EXCLUDED.estado, motivo=EXCLUDED.motivo,
			pago_id=EXCLUDED.pago_id, archivo_nombre=EXCLUDED.archivo_nombre`,
		eid, cu.ID, codRecibo, reciboID, monto, medioTexto, codOp, fecha, estado, motivo, pagoID, archivoNombre, uid); err != nil {
		return "error", err.Error()
	}
	if estado == "acreditada" && reciboID != nil {
		_, _ = tx.Exec(ctx, `UPDATE recibo SET enviado_recaudadora_en=COALESCE(enviado_recaudadora_en, now()) WHERE id=$1`, *reciboID)
	}
	return estado, motivo
}

// ---------- importar liquidaciones ----------

// importarLiquidacionesRecaudadora: POST /recaudadora/cuentas/{id}/liquidaciones/importar
// (multipart {archivo, col_codigo?, col_fecha, col_bruto, col_comision?, col_neto?, vista_previa?}).
// Sin columna de comisión se calcula con la configuración; si viene el neto, debe ser bruto − comisión.
func (s *Server) importarLiquidacionesRecaudadora(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	cu, err := s.cuentaRecaudadora(ctx, e.ID, idURL(r, "id"))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	nombre, cab, filas, err := leerReporte(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var guardado MapeoLiquidaciones
	_ = json.Unmarshal(cu.MapeoLiq, &guardado)
	m := MapeoLiquidaciones{
		Codigo:   elegirColumna(r, "col_codigo", guardado.Codigo, cab, "liquidacion", "codigo", "lote"),
		Fecha:    elegirColumna(r, "col_fecha", guardado.Fecha, cab, "fecha"),
		Bruto:    elegirColumna(r, "col_bruto", guardado.Bruto, cab, "bruto", "recaudado", "monto"),
		Comision: elegirColumna(r, "col_comision", guardado.Comision, cab, "comision"),
		Neto:     elegirColumna(r, "col_neto", guardado.Neto, cab, "neto", "abonado"),
	}
	iCod, iFecha, iBruto, iCom, iNeto := columnaDe(cab, m.Codigo), columnaDe(cab, m.Fecha), columnaDe(cab, m.Bruto), columnaDe(cab, m.Comision), columnaDe(cab, m.Neto)
	ev := P.Validacion("Indica qué columna es cada dato.")
	if iFecha < 0 {
		ev.Campo("col_fecha", "Elige la columna.")
	}
	if iBruto < 0 {
		ev.Campo("col_bruto", "Elige la columna.")
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev.Con("cabeceras", cab).Con("mapeo", m))
		return
	}
	previa := siVistaPrevia(r)
	uid := ses(r).UsuarioID
	rubro := s.rubroComision(ctx, e.ID, cu.RubroID)

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	conteo := map[string]int{"registrada": 0, "duplicada": 0, "error": 0}
	var bruto, comision, neto int64
	resultado := []map[string]any{}
	for i, fila := range filas {
		if strings.TrimSpace(strings.Join(fila, "")) == "" {
			continue
		}
		f := map[string]any{"fila": i + 2}
		est, motivo := func() (string, string) {
			b, errB := conciliacion.MontoCts(celda(fila, iBruto))
			fecha, errF := conciliacion.Fecha(celda(fila, iFecha))
			if errB != nil || b <= 0 {
				return "error", "Monto bruto inválido."
			}
			if errF != nil {
				return "error", "Fecha inválida."
			}
			cod := celda(fila, iCod)
			if cod == "" {
				cod = fecha
			}
			com := ComisionCts(b, cu.Pbs, cu.FijoCts)
			if iCom >= 0 {
				c, err := conciliacion.MontoCts(celda(fila, iCom))
				if err != nil || c < 0 || c > b {
					return "error", "Comisión inválida."
				}
				com = c
			}
			n := NetoLiquidacion(b, com)
			f["codigo"], f["fecha"], f["bruto_cts"], f["comision_cts"], f["neto_cts"] = cod, fecha, b, com, n
			if iNeto >= 0 {
				nArchivo, err := conciliacion.MontoCts(celda(fila, iNeto))
				if err != nil || nArchivo != n {
					return "error", fmt.Sprintf("El neto del archivo no es bruto − comisión (%s).", P.Soles(n))
				}
			}
			sp, err := tx.Begin(ctx)
			if err != nil {
				return "error", err.Error()
			}
			defer sp.Rollback(ctx)
			var lid int64
			err = sp.QueryRow(ctx, `INSERT INTO recaudadora_liquidacion (edificio_id, cuenta_recaudadora_id, codigo_liquidacion, fecha, monto_bruto_cts,
					comision_cts, monto_neto_cts, archivo_nombre, creado_por) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
				ON CONFLICT (cuenta_recaudadora_id, codigo_liquidacion) DO NOTHING RETURNING id`,
				e.ID, cu.ID, cod, fecha, b, com, n, nombre, uid).Scan(&lid)
			if errors.Is(err, pgx.ErrNoRows) {
				return "duplicada", "Ya se registró en una carga anterior."
			}
			if err != nil {
				return "error", mensajeDe(P.Traducir(err))
			}
			if com > 0 {
				if rubro == 0 {
					return "error", "El edificio no tiene rubros para el egreso de comisión."
				}
				t, _ := time.Parse("2006-01-02", fecha)
				var eg int64
				if err := sp.QueryRow(ctx, `INSERT INTO egreso (edificio_id, periodo, rubro_id, descripcion, monto_cts, fecha, tipo_documento, registrado_por)
					VALUES ($1,$2,$3,$4,$5,$6,'voucher',$7) RETURNING id`, e.ID, fecha[:7], rubro,
					"Comisión "+cu.Proveedor+" · liquidación "+cod, com, fecha, uid).Scan(&eg); err != nil {
					return "error", mensajeDe(P.Traducir(err))
				}
				if err := s.asentarEgreso(ctx, sp, e.ID, eg, rubro, com, t); err != nil {
					return "error", err.Error()
				}
				if _, err := sp.Exec(ctx, `UPDATE recaudadora_liquidacion SET egreso_id=$1 WHERE id=$2`, eg, lid); err != nil {
					return "error", err.Error()
				}
				f["egreso_id"] = eg
			}
			if err := sp.Commit(ctx); err != nil {
				return "error", err.Error()
			}
			bruto, comision, neto = bruto+b, comision+com, neto+n
			return "registrada", ""
		}()
		f["estado"], f["motivo"] = est, motivo
		conteo[est]++
		resultado = append(resultado, f)
	}
	if !previa {
		b, _ := json.Marshal(m)
		if _, err := tx.Exec(ctx, `UPDATE cuenta_recaudadora SET mapeo_liquidaciones=$1 WHERE id=$2`, b, cu.ID); err != nil {
			P.Fallo(w, r, err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"vista_previa": previa, "archivo": nombre, "mapeo": m, "cabeceras": cab,
		"registradas": conteo["registrada"], "duplicadas": conteo["duplicada"], "errores": conteo["error"],
		"bruto_cts": bruto, "comision_cts": comision, "neto_cts": neto, "filas": resultado})
}

// rubroComision: el rubro configurado; si no, «administracion»; si no, el primero del edificio.
func (s *Server) rubroComision(ctx context.Context, eid int64, configurado *int64) int64 {
	if configurado != nil && *configurado > 0 {
		return *configurado
	}
	var id int64
	_ = s.DB.QueryRow(ctx, `SELECT id FROM rubro WHERE edificio_id=$1 ORDER BY (slug='administracion') DESC, orden, id LIMIT 1`, eid).Scan(&id)
	return id
}

// ---------- reportes ----------

// listarTransaccionesRecaudadora: GET /recaudadora/transacciones?cuenta_id=&estado=
func (s *Server) listarTransaccionesRecaudadora(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	cid, _ := strconv.ParseInt(r.URL.Query().Get("cuenta_id"), 10, 64)
	estado := strings.TrimSpace(r.URL.Query().Get("estado"))
	filas, err := db.Filas(r.Context(), s.DB, `SELECT t.id, c.proveedor, t.codigo_recibo, t.codigo_operacion, t.monto_cts, t.medio,
			to_char(t.fecha,'YYYY-MM-DD') AS fecha, t.estado, t.motivo, t.pago_id, COALESCE(rc.numero,'') AS recibo, COALESCE(u.codigo,'') AS unidad
		FROM recaudadora_transaccion t JOIN cuenta_recaudadora c ON c.id=t.cuenta_recaudadora_id
		LEFT JOIN recibo rc ON rc.id=t.recibo_id LEFT JOIN unidad u ON u.id=rc.unidad_id
		WHERE t.edificio_id=$1 AND ($2=0 OR t.cuenta_recaudadora_id=$2) AND ($3='' OR t.estado=$3)
		ORDER BY t.fecha DESC, t.id DESC LIMIT 500`, e.ID, cid, estado)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var total int64
	for _, f := range filas {
		if f["estado"] == "acreditada" {
			total += f["monto_cts"].(int64)
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "acreditado_cts": total})
}

// listarLiquidacionesRecaudadora: GET /recaudadora/liquidaciones?cuenta_id=
func (s *Server) listarLiquidacionesRecaudadora(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	cid, _ := strconv.ParseInt(r.URL.Query().Get("cuenta_id"), 10, 64)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT l.id, c.proveedor, l.codigo_liquidacion, to_char(l.fecha,'YYYY-MM-DD') AS fecha,
			l.monto_bruto_cts, l.comision_cts, l.monto_neto_cts, l.egreso_id
		FROM recaudadora_liquidacion l JOIN cuenta_recaudadora c ON c.id=l.cuenta_recaudadora_id
		WHERE l.edificio_id=$1 AND ($2=0 OR l.cuenta_recaudadora_id=$2) ORDER BY l.fecha DESC, l.id DESC LIMIT 500`, e.ID, cid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var bruto, com, neto int64
	for _, f := range filas {
		bruto += f["monto_bruto_cts"].(int64)
		com += f["comision_cts"].(int64)
		neto += f["monto_neto_cts"].(int64)
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "bruto_cts": bruto, "comision_cts": com, "neto_cts": neto})
}

// recibosConDeuda: recibos del periodo con saldo (lo que se manda a la recaudadora o al banco).
func recibosConDeuda(ctx context.Context, q db.Q, eid int64, periodo string) ([]map[string]any, error) {
	return db.Filas(ctx, q, `SELECT r.id, COALESCE(r.numero,'') AS numero, COALESCE(r.correlativo,'') AS correlativo, u.codigo AS unidad,
			COALESCE((SELECT p.nombre FROM unidad_persona up JOIN persona p ON p.id=up.persona_id
				WHERE up.unidad_id=u.id AND up.rol='propietario' AND up.hasta IS NULL LIMIT 1),'') AS titular,
			r.total_cts, r.pagado_cts, r.total_cts - r.pagado_cts AS saldo_cts, r.estado,
			to_char(r.emitido_en AT TIME ZONE 'America/Lima','YYYY-MM-DD') AS emitido, to_char(r.vence,'YYYY-MM-DD') AS vence,
			to_char(r.enviado_recaudadora_en AT TIME ZONE 'America/Lima','YYYY-MM-DD HH24:MI') AS enviado_recaudadora
		FROM recibo r JOIN periodo pe ON pe.id=r.periodo_id JOIN unidad u ON u.id=r.unidad_id
		WHERE r.edificio_id=$1 AND pe.periodo=$2 AND r.estado IN ('emitido','pagado_parcial') AND r.total_cts > r.pagado_cts
		ORDER BY u.codigo`, eid, periodo)
}

func periodoQuery(r *http.Request) (string, error) {
	p := strings.TrimSpace(r.URL.Query().Get("periodo"))
	if p == "" {
		p = P.PeriodoActual()
	}
	if !P.PeriodoValido(p) {
		return "", P.Validacion("El periodo debe ser AAAA-MM.").Campo("periodo", "Formato AAAA-MM.")
	}
	return p, nil
}

// listarRecibosRecaudadora: GET /recaudadora/recibos?periodo= → recibos con saldo y su estado «Enviado a recaudadora».
func (s *Server) listarRecibosRecaudadora(w http.ResponseWriter, r *http.Request) {
	p, err := periodoQuery(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	filas, err := recibosConDeuda(r.Context(), s.DB, edf(r).ID, p)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"periodo": p, "datos": filas, "total": len(filas), "pagina": 1})
}

// enviarRecibosRecaudadora: POST /recaudadora/enviar {periodo} → marca los recibos con saldo como enviados.
func (s *Server) enviarRecibosRecaudadora(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Periodo string `json:"periodo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if !P.PeriodoValido(in.Periodo) {
		P.Fallo(w, r, P.Validacion("El periodo debe ser AAAA-MM.").Campo("periodo", "Formato AAAA-MM."))
		return
	}
	n, err := marcarEnviados(r.Context(), s.DB, edf(r).ID, in.Periodo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"periodo": in.Periodo, "marcados": n})
}

func marcarEnviados(ctx context.Context, q db.Q, eid int64, periodo string) (int64, error) {
	tag, err := q.Exec(ctx, `UPDATE recibo r SET enviado_recaudadora_en=now() FROM periodo pe
		WHERE pe.id=r.periodo_id AND r.edificio_id=$1 AND pe.periodo=$2 AND r.estado IN ('emitido','pagado_parcial')
			AND r.total_cts > r.pagado_cts AND r.enviado_recaudadora_en IS NULL`, eid, periodo)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// deudasRecaudadoraCSV: GET /recaudadora/deudas.csv?periodo= → archivo de deudas para cargar en la recaudadora.
func (s *Server) deudasRecaudadoraCSV(w http.ResponseWriter, r *http.Request) {
	p, err := periodoQuery(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	filas, err := recibosConDeuda(r.Context(), s.DB, edf(r).ID, p)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="deudas-`+p+`.csv"`)
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write([]string{"codigo_recibo", "unidad", "titular", "monto", "vence"})
	for _, f := range filas {
		saldo := f["saldo_cts"].(int64)
		vence, _ := f["vence"].(string)
		_ = cw.Write([]string{f["numero"].(string), f["unidad"].(string), f["titular"].(string), fmt.Sprintf("%d.%02d", saldo/100, saldo%100), vence})
	}
	cw.Flush()
}
