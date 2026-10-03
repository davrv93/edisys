package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
	"edisys/api/internal/reparto"
)

// periodoID busca el id del periodo del edificio (404 si no está abierto).
func periodoID(ctx context.Context, q db.Q, eid int64, periodo string) (int64, error) {
	var id int64
	err := q.QueryRow(ctx, `SELECT id FROM periodo WHERE edificio_id=$1 AND periodo=$2`, eid, periodo).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, P.NoEncontrado("el periodo " + periodo + " (ábrelo primero)")
	}
	return id, err
}

func (s *Server) listarPeriodos(w http.ResponseWriter, r *http.Request) {
	filas, err := db.Filas(r.Context(), s.DB, `SELECT p.id, p.periodo, to_char(p.fecha_corte,'YYYY-MM-DD') AS fecha_corte, p.estado,
		count(rc.id) FILTER (WHERE rc.estado NOT IN ('borrador','anulado')) AS recibos_emitidos,
		count(rc.id) FILTER (WHERE rc.estado='borrador') AS borradores,
		COALESCE(sum(rc.total_cts) FILTER (WHERE rc.estado NOT IN ('borrador','anulado')),0) AS emitido_cts,
		(SELECT COALESCE(sum(monto_cts),0) FROM presupuesto pr WHERE pr.periodo_id=p.id) AS presupuesto_cts
		FROM periodo p LEFT JOIN recibo rc ON rc.periodo_id=p.id AND rc.origen='periodo'
		WHERE p.edificio_id=$1 AND NOT p.historico GROUP BY p.id ORDER BY p.periodo DESC`, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas})
}

// abrirPeriodo: POST /periodos {periodo, fecha_corte} → 201 · 409 PERIODO_EXISTE.
func (s *Server) abrirPeriodo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Periodo    string `json:"periodo"`
		FechaCorte string `json:"fecha_corte"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if !P.PeriodoValido(in.Periodo) {
		P.Fallo(w, r, P.Validacion("Periodo inválido.").Campo("periodo", "Formato AAAA-MM."))
		return
	}
	e := edf(r)
	ctx := r.Context()
	fc := in.FechaCorte
	if fc == "" {
		var dia int
		_ = s.DB.QueryRow(ctx, `SELECT dia_corte FROM edificio WHERE id=$1`, e.ID).Scan(&dia)
		t, _ := time.Parse("2006-01", in.Periodo)
		ult := t.AddDate(0, 1, -1).Day()
		if dia > ult {
			dia = ult
		}
		fc = fmt.Sprintf("%s-%02d", in.Periodo, dia)
	}
	// Si el periodo existía solo como «histórico» (deuda inicial importada), se abre de verdad.
	fila, err := db.Fila(ctx, s.DB, `INSERT INTO periodo (edificio_id, periodo, fecha_corte) VALUES ($1,$2,$3)
		ON CONFLICT (edificio_id, periodo) DO UPDATE SET historico=false, estado='abierto', fecha_corte=EXCLUDED.fecha_corte WHERE periodo.historico
		RETURNING id, periodo, to_char(fecha_corte,'YYYY-MM-DD') AS fecha_corte, estado`, e.ID, in.Periodo, fc)
	if errors.Is(err, pgx.ErrNoRows) {
		P.Fallo(w, r, P.Conflicto("PERIODO_EXISTE", "El periodo "+in.Periodo+" ya está abierto."))
		return
	}
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	// El presupuesto arranca copiado del periodo anterior (se edita después).
	_, _ = s.DB.Exec(ctx, `INSERT INTO presupuesto (periodo_id, rubro_id, monto_cts)
		SELECT $1, pr.rubro_id, pr.monto_cts FROM presupuesto pr JOIN periodo p ON p.id=pr.periodo_id
		WHERE p.edificio_id=$2 AND p.periodo = (SELECT max(periodo) FROM periodo WHERE edificio_id=$2 AND periodo < $3)`, fila["id"], e.ID, in.Periodo)
	P.JSON(w, http.StatusCreated, fila)
}

func (s *Server) verPresupuesto(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	pid, err := periodoID(r.Context(), s.DB, e.ID, chi.URLParam(r, "p"))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	filas, err := db.Filas(r.Context(), s.DB, `SELECT rb.id AS rubro_id, rb.slug, rb.nombre, COALESCE(pr.monto_cts,0) AS monto_cts
		FROM rubro rb LEFT JOIN presupuesto pr ON pr.rubro_id=rb.id AND pr.periodo_id=$2 WHERE rb.edificio_id=$1 ORDER BY rb.orden, rb.id`, e.ID, pid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var total int64
	for _, f := range filas {
		total += f["monto_cts"].(int64)
	}
	P.JSON(w, http.StatusOK, map[string]any{"rubros": filas, "total_cts": total})
}

// guardarPresupuesto: PUT /periodos/{p}/presupuesto {rubros: [{rubro_id, monto_cts}]}.
func (s *Server) guardarPresupuesto(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Rubros []struct {
			RubroID  int64 `json:"rubro_id"`
			MontoCts int64 `json:"monto_cts"`
		} `json:"rubros"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	pid, err := periodoID(ctx, tx, e.ID, chi.URLParam(r, "p"))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, rb := range in.Rubros {
		if rb.MontoCts < 0 {
			P.Fallo(w, r, P.Validacion("Los montos no pueden ser negativos.").Campo("monto_cts", "Mayor o igual a cero."))
			return
		}
		tag, err := tx.Exec(ctx, `INSERT INTO presupuesto (periodo_id, rubro_id, monto_cts) SELECT $1, id, $3 FROM rubro WHERE id=$2 AND edificio_id=$4
			ON CONFLICT (periodo_id, rubro_id) DO UPDATE SET monto_cts = EXCLUDED.monto_cts`, pid, rb.RubroID, rb.MontoCts, e.ID)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		if tag.RowsAffected() == 0 {
			P.Fallo(w, r, P.Validacion("El rubro "+strconv.FormatInt(rb.RubroID, 10)+" no es de este edificio."))
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.verPresupuesto(w, r)
}

type lineaRecibo struct {
	Tipo        string `json:"tipo"`
	Descripcion string `json:"descripcion"`
	MontoCts    int64  `json:"monto_cts"`
	ReservaID   *int64 `json:"reserva_id,omitempty"`
	LecturaID   *int64 `json:"lectura_id,omitempty"`
	AjusteID    *int64 `json:"ajuste_id,omitempty"`
}

// Generar arma los borradores del periodo con el motor de reparto (mayor residuo).
func (s *Server) Generar(ctx context.Context, tx pgx.Tx, eid int64, periodo string) (map[string]any, error) {
	pid, err := periodoID(ctx, tx, eid, periodo)
	if err != nil {
		return nil, err
	}
	var emitidos int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM recibo WHERE periodo_id=$1 AND origen='periodo' AND estado NOT IN ('borrador','anulado')`, pid).Scan(&emitidos); err != nil {
		return nil, err
	}
	if emitidos > 0 {
		return nil, P.Conflicto("YA_EMITIDO", "Los recibos de "+P.NombrePeriodo(periodo)+" ya se emitieron. Solo se pueden anular.")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM recibo WHERE periodo_id=$1 AND estado='borrador'`, pid); err != nil {
		return nil, err
	}
	var politica string
	var cobraAgua bool
	if err := tx.QueryRow(ctx, `SELECT politica_reparto, cobra_agua FROM edificio WHERE id=$1`, eid).Scan(&politica, &cobraAgua); err != nil {
		return nil, err
	}
	var presupuesto int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(monto_cts),0) FROM presupuesto WHERE periodo_id=$1`, pid).Scan(&presupuesto); err != nil {
		return nil, err
	}
	advertencias := []string{}
	if presupuesto == 0 {
		advertencias = append(advertencias, "El presupuesto del periodo está en cero: las cuotas salen en cero.")
	}
	type unidad struct {
		id          int64
		codigo      string
		part        int64
		propietario string
	}
	filas, err := tx.Query(ctx, `SELECT u.id, u.codigo, (u.participacion_pct*10000)::bigint,
		COALESCE((SELECT pe.nombre FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id WHERE up.unidad_id=u.id AND up.rol='propietario' AND up.hasta IS NULL LIMIT 1),'')
		FROM unidad u WHERE u.edificio_id=$1 AND u.activo ORDER BY u.codigo`, eid)
	if err != nil {
		return nil, err
	}
	var us []unidad
	for filas.Next() {
		var u unidad
		if err := filas.Scan(&u.id, &u.codigo, &u.part, &u.propietario); err != nil {
			filas.Close()
			return nil, err
		}
		us = append(us, u)
	}
	filas.Close()
	pesos := make([]int64, len(us))
	for i, u := range us {
		pesos[i] = u.part
		if politica == "partes_iguales" {
			pesos[i] = 1
		}
		if u.propietario == "" {
			advertencias = append(advertencias, "Unidad "+u.codigo+" sin propietario.")
		}
	}
	cuotas := reparto.PorPesos(presupuesto, pesos)
	lineas := map[int64][]lineaRecibo{}
	for i, u := range us {
		if cuotas[i] > 0 {
			lineas[u.id] = append(lineas[u.id], lineaRecibo{Tipo: "cuota", Descripcion: "Cuota de mantenimiento", MontoCts: cuotas[i]})
		}
	}
	// Agua: del reparto aprobado (08).
	var repLineas []byte
	err = tx.QueryRow(ctx, `SELECT lineas FROM reparto_medidor WHERE periodo_id=$1 AND tipo='agua'`, pid).Scan(&repLineas)
	if err == nil {
		var ls []reparto.Linea
		_ = json.Unmarshal(repLineas, &ls)
		for _, l := range ls {
			lineas[l.UnidadID] = append(lineas[l.UnidadID],
				lineaRecibo{Tipo: "agua", Descripcion: "Agua (consumo propio " + strings.Replace(l.Consumo, ".", ",", 1) + " m³)", MontoCts: l.PropioCts},
				lineaRecibo{Tipo: "agua_comun", Descripcion: "Áreas comunes (agua)", MontoCts: l.ComunCts})
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		if cobraAgua {
			advertencias = append(advertencias, "Aún no se aprueba el reparto de agua del periodo: los recibos salen sin agua.")
		}
	} else {
		return nil, err
	}
	// Reservas cargadas al recibo cuyo uso cae en el periodo.
	ini, fin := P.RangoPeriodo(periodo)
	fr, err := tx.Query(ctx, `SELECT rv.id, rv.unidad_id, rv.codigo, rc.nombre, rv.total_cts FROM reserva rv JOIN recurso rc ON rc.id=rv.recurso_id
		JOIN unidad u ON u.id = rv.unidad_id
		WHERE rv.edificio_id=$1 AND rv.estado='confirmada' AND rv.modo_cobro='cargo_recibo' AND rv.total_cts > 0
		  AND rv.inicio >= $2 AND rv.inicio < $3
		  AND NOT EXISTS (SELECT 1 FROM recibo_linea rl JOIN recibo r2 ON r2.id=rl.recibo_id WHERE rl.reserva_id=rv.id AND r2.estado <> 'anulado')
		ORDER BY rv.inicio`, eid, ini, fin)
	if err != nil {
		return nil, err
	}
	for fr.Next() {
		var rid, uid, total int64
		var codigo, recurso string
		if err := fr.Scan(&rid, &uid, &codigo, &recurso, &total); err != nil {
			fr.Close()
			return nil, err
		}
		id := rid
		lineas[uid] = append(lineas[uid], lineaRecibo{Tipo: "reserva", Descripcion: "Reserva " + codigo + " " + recurso, MontoCts: total, ReservaID: &id})
	}
	fr.Close()
	// Ajustes pendientes (notas de cargo/abono por correcciones de periodos ya emitidos, 0009).
	fa, err := tx.Query(ctx, `SELECT id, unidad_id, monto_cts, motivo FROM ajuste WHERE edificio_id=$1 AND recibo_id IS NULL AND periodo_origen < $2 ORDER BY id`, eid, periodo)
	if err != nil {
		return nil, err
	}
	for fa.Next() {
		var aid, uid, monto int64
		var motivo string
		if err := fa.Scan(&aid, &uid, &monto, &motivo); err != nil {
			fa.Close()
			return nil, err
		}
		id := aid
		lineas[uid] = append(lineas[uid], lineaRecibo{Tipo: "ajuste", Descripcion: descAjuste(motivo, monto), MontoCts: monto, AjusteID: &id})
	}
	fa.Close()

	var recibos []map[string]any
	var totalPeriodo int64
	for _, u := range us {
		ls := lineas[u.id]
		var total int64
		for _, l := range ls {
			total += l.MontoCts
		}
		if total < 0 {
			// Un abono no puede dejar el recibo en negativo: los ajustes esperan al siguiente periodo.
			var sin []lineaRecibo
			total = 0
			for _, l := range ls {
				if l.Tipo != "ajuste" {
					sin = append(sin, l)
					total += l.MontoCts
				}
			}
			ls = sin
			advertencias = append(advertencias, "Unidad "+u.codigo+": el abono pendiente supera el recibo; se aplicará el mes siguiente.")
		}
		var rid int64
		if err := tx.QueryRow(ctx, `INSERT INTO recibo (edificio_id, periodo_id, unidad_id, estado, total_cts) VALUES ($1,$2,$3,'borrador',$4) RETURNING id`,
			eid, pid, u.id, total).Scan(&rid); err != nil {
			return nil, err
		}
		for i, l := range ls {
			if _, err := tx.Exec(ctx, `INSERT INTO recibo_linea (recibo_id, tipo, descripcion, monto_cts, orden, reserva_id, lectura_id, ajuste_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
				rid, l.Tipo, l.Descripcion, l.MontoCts, i+1, l.ReservaID, l.LecturaID, l.AjusteID); err != nil {
				return nil, err
			}
			if l.AjusteID != nil {
				if _, err := tx.Exec(ctx, `UPDATE ajuste SET recibo_id=$2 WHERE id=$1`, *l.AjusteID, rid); err != nil {
					return nil, err
				}
			}
		}
		if ls == nil {
			ls = []lineaRecibo{}
		}
		recibos = append(recibos, map[string]any{"recibo_id": rid, "unidad_id": u.id, "unidad": u.codigo, "propietario": u.propietario, "lineas": ls, "total_cts": total})
		totalPeriodo += total
	}
	if recibos == nil {
		recibos = []map[string]any{}
	}
	return map[string]any{"periodo": periodo, "recibos": recibos, "total_cts": totalPeriodo, "presupuesto_cts": presupuesto, "advertencias": advertencias}, nil
}

func (s *Server) generarRecibos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	res, err := s.Generar(ctx, tx, edf(r).ID, chi.URLParam(r, "p"))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, res)
}

// emitirRecibos: numera, congela y fija el vencimiento. 409 YA_EMITIDO · 422 LECTURAS_PENDIENTES.
func (s *Server) emitirRecibos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	periodo := chi.URLParam(r, "p")
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	pid, err := periodoID(ctx, tx, e.ID, periodo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var emitidos, borradores int
	var cobraAgua, hayReparto bool
	if err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE estado NOT IN ('borrador','anulado')), count(*) FILTER (WHERE estado='borrador'),
		(SELECT cobra_agua FROM edificio WHERE id=$2), EXISTS (SELECT 1 FROM reparto_medidor WHERE periodo_id=$1 AND tipo='agua')
		FROM recibo WHERE periodo_id=$1 AND origen='periodo'`, pid, e.ID).Scan(&emitidos, &borradores, &cobraAgua, &hayReparto); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if emitidos > 0 {
		P.Fallo(w, r, P.Conflicto("YA_EMITIDO", "Los recibos de "+P.NombrePeriodo(periodo)+" ya se emitieron."))
		return
	}
	if borradores == 0 {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "SIN_BORRADORES", "Primero genera los borradores del periodo."))
		return
	}
	if cobraAgua && !hayReparto {
		var pend int
		_ = tx.QueryRow(ctx, `SELECT count(*) FROM medidor m WHERE m.edificio_id=$1 AND m.unidad_id IS NOT NULL AND m.activo AND m.tipo='agua'
			AND NOT EXISTS (SELECT 1 FROM lectura l WHERE l.medidor_id=m.id AND l.periodo_id=$2)`, e.ID, pid).Scan(&pend)
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "LECTURAS_PENDIENTES",
			fmt.Sprintf("El edificio cobra agua y falta aprobar el reparto (%d lecturas pendientes).", pend)).Con("lecturas_pendientes", pend))
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE recibo r SET estado='emitido', emitido_en=now(),
		numero = $2 || '-' || u.codigo,
		correlativo = 'R-' || lpad(nextval('recibo_correlativo_seq')::text, 6, '0'),
		vence = (now() AT TIME ZONE 'America/Lima')::date + (SELECT dias_vencimiento FROM edificio WHERE id=r.edificio_id)
		FROM unidad u WHERE u.id = r.unidad_id AND r.periodo_id=$1 AND r.estado='borrador'`, pid, periodo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE periodo SET estado='emitido' WHERE id=$1`, pid); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"emitidos": tag.RowsAffected(), "periodo": periodo})
}

// listarRecibos: GET /recibos?periodo=&estado=&unidad=&mios=1&pagina=
func (s *Server) listarRecibos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	q := r.URL.Query()
	pagina, por := paginacion(r)
	cond := []string{"r.edificio_id = $1"}
	args := []any{e.ID}
	add := func(c string, v any) {
		args = append(args, v)
		cond = append(cond, strings.ReplaceAll(c, "?", "$"+strconv.Itoa(len(args))))
	}
	if p := q.Get("periodo"); p != "" {
		add("p.periodo = ?", p)
	}
	if es := q.Get("estado"); es != "" {
		if es == "vencido" {
			cond = append(cond, "r.estado IN ('emitido','pagado_parcial') AND r.vence < (now() AT TIME ZONE 'America/Lima')::date")
		} else if es == "pendiente" {
			cond = append(cond, "r.estado IN ('emitido','pagado_parcial')")
		} else {
			add("r.estado = ?", es)
		}
	} else {
		cond = append(cond, "r.estado <> 'anulado'")
	}
	if un := q.Get("unidad"); un != "" {
		add("u.codigo = ?", un)
	}
	if id, err := strconv.ParseInt(q.Get("unidad_id"), 10, 64); err == nil {
		add("u.id = ?", id)
	}
	if e.SoloLoSuyo() || q.Get("mios") == "1" {
		add("r.unidad_id = ANY(?)", e.Unidades)
		cond = append(cond, "r.estado <> 'borrador'")
	}
	where := strings.Join(cond, " AND ")
	var total int64
	if err := s.DB.QueryRow(r.Context(), `SELECT count(*) FROM recibo r JOIN periodo p ON p.id=r.periodo_id JOIN unidad u ON u.id=r.unidad_id WHERE `+where, args...).Scan(&total); err != nil {
		P.Fallo(w, r, err)
		return
	}
	args = append(args, por, (pagina-1)*por)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT r.id, r.numero, r.correlativo, p.periodo, u.id AS unidad_id, u.codigo AS unidad,
			COALESCE((SELECT pe.nombre FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id WHERE up.unidad_id=u.id AND up.rol='propietario' AND up.hasta IS NULL LIMIT 1),'') AS propietario,
			r.total_cts, r.pagado_cts, (r.total_cts - r.pagado_cts) AS saldo_cts, r.estado, to_char(r.vence,'YYYY-MM-DD') AS vence,
			(r.estado IN ('emitido','pagado_parcial') AND r.vence < (now() AT TIME ZONE 'America/Lima')::date) AS vencido,
			(SELECT count(*) FROM pago pg WHERE pg.recibo_id=r.id AND pg.estado='pendiente_validacion') AS pagos_por_validar, r.origen
		FROM recibo r JOIN periodo p ON p.id=r.periodo_id JOIN unidad u ON u.id=r.unidad_id
		WHERE `+where+fmt.Sprintf(` ORDER BY p.periodo DESC, u.codigo LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, paginado(filas, total, pagina))
}

// reciboVisible carga un recibo del edificio; al propietario ajeno le responde 404 (no 403).
func (s *Server) reciboVisible(ctx context.Context, e *Edificio, rid int64) (map[string]any, error) {
	rc, err := db.Fila(ctx, s.DB, `SELECT r.id, r.numero, r.correlativo, p.periodo, p.id AS periodo_id, r.estado, r.total_cts, r.pagado_cts, r.origen,
			(r.total_cts - r.pagado_cts) AS saldo_cts, to_char(r.vence,'YYYY-MM-DD') AS vence, r.emitido_en, r.anulado_motivo,
			u.id AS unidad_id, u.codigo AS unidad, u.participacion_pct::float8 AS participacion_pct,
			COALESCE((SELECT pe.nombre FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id WHERE up.unidad_id=u.id AND up.rol='propietario' AND up.hasta IS NULL LIMIT 1),'') AS propietario,
			e.nombre AS edificio, e.direccion, e.yape_numero
		FROM recibo r JOIN periodo p ON p.id=r.periodo_id JOIN unidad u ON u.id=r.unidad_id JOIN edificio e ON e.id=r.edificio_id
		WHERE r.id=$1 AND r.edificio_id=$2`, rid, e.ID)
	if err != nil {
		return nil, P.NoEncontrado("este recibo")
	}
	if e.SoloLoSuyo() && (!e.EsSuya(rc["unidad_id"].(int64)) || rc["estado"] == "borrador") {
		return nil, P.NoEncontrado("este recibo")
	}
	return rc, nil
}

// verRecibo: detalle con líneas, pagos y la foto del medidor firmada.
func (s *Server) verRecibo(w http.ResponseWriter, r *http.Request) {
	rid, err := idRuta(r, "rid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	rc, err := s.reciboVisible(ctx, e, rid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	lineas, err := db.Filas(ctx, s.DB, `SELECT id, tipo, descripcion, monto_cts, reserva_id FROM recibo_linea WHERE recibo_id=$1 ORDER BY orden, id`, rid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	pagos, err := db.Filas(ctx, s.DB, `SELECT id, monto_cts, medio, codigo_operacion, to_char(fecha,'YYYY-MM-DD') AS fecha, estado, motivo, voucher_id
		FROM pago WHERE recibo_id=$1 ORDER BY fecha, id`, rid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, p := range pagos {
		if v, ok := p["voucher_id"].(int64); ok {
			p["voucher_url"] = s.Firma.URL(v)
		} else {
			p["voucher_url"] = nil
		}
	}
	rc["lineas"] = lineas
	rc["pagos"] = pagos
	rc["foto_medidor_url"] = nil
	rc["medidor"] = nil
	med, err := db.Fila(ctx, s.DB, `SELECT l.id AS lectura_id, m.serie, l.anterior::text AS lectura_anterior, l.valor::text AS lectura_actual, l.consumo::text AS consumo, l.foto_id,
			l.tomada_en, l.alerta FROM lectura l JOIN medidor m ON m.id=l.medidor_id
		WHERE m.unidad_id=$1 AND l.periodo_id=$2 AND m.tipo='agua' LIMIT 1`, rc["unidad_id"], rc["periodo_id"])
	if err == nil {
		fid := med["foto_id"].(int64)
		rc["foto_medidor_url"] = s.Firma.URL(fid)
		delete(med, "foto_id")
		rc["medidor"] = med
	}
	P.JSON(w, http.StatusOK, rc)
}

// pdfRecibo: el recibo en una hoja A4.
func (s *Server) pdfRecibo(w http.ResponseWriter, r *http.Request) {
	rid, err := idRuta(r, "rid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	rc, err := s.reciboVisible(ctx, e, rid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	datos, err := s.reciboPDF(ctx, rc, rid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"recibo-%v.pdf\"", val(rc["numero"])))
	_, _ = w.Write(datos)
}

// reciboPDF dibuja el recibo (cargado con reciboVisible) en una hoja A4.
// marca: I1 · el dibujo vive en recibo_plantilla.go: color, logo y bloques según la plantilla del edificio.
func (s *Server) reciboPDF(ctx context.Context, rc map[string]any, rid int64) ([]byte, error) {
	var eid int64
	if err := s.DB.QueryRow(ctx, `SELECT edificio_id FROM recibo WHERE id=$1`, rid).Scan(&eid); err != nil {
		return nil, err
	}
	return s.reciboConPlantilla(ctx, eid, rc, rid, nil)
}

func val(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// enviarRecibos: POST /recibos/enviar {recibo_ids, canal?: correo|whatsapp} → 202 {en_cola}. Por defecto va por
// correo con el PDF adjunto (bandeja correo_mensaje); con canal=whatsapp, por la bandeja de WhatsApp.
func (s *Server) enviarRecibos(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ReciboIDs []int64 `json:"recibo_ids"`
		Canal     string  `json:"canal"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	if in.Canal != "whatsapp" {
		if len(in.ReciboIDs) == 0 {
			P.Fallo(w, r, P.Validacion("Indica los recibos.").Campo("recibo_ids", "Obligatorio."))
			return
		}
		res, err := s.CorreoRecibos(r.Context(), e, ses(r).UsuarioID, "", in.ReciboIDs)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		P.JSON(w, http.StatusAccepted, res)
		return
	}
	res, err := s.encolarRecibos(r.Context(), e.ID, ses(r).UsuarioID, "", in.ReciboIDs)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	res["en_cola"] = res["encolados"]
	P.JSON(w, http.StatusAccepted, res)
}

// registrarPago: POST /recibos/{rid}/pagos (multipart o JSON).
// Administrador (pagos.registrar) → validado. Propietario (pagos.informar, su recibo) → pendiente_validacion con voucher.
func (s *Server) registrarPago(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	if !e.Puede("pagos.registrar") && !e.Puede("pagos.informar") {
		P.Fallo(w, r, P.Prohibido("SIN_PERMISO", "Tu rol no puede registrar pagos.").Con("permiso", "pagos.registrar"))
		return
	}
	rid, err := idRuta(r, "rid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	ctx := r.Context()
	rc, err := s.reciboVisible(ctx, e, rid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	admin := e.Puede("pagos.registrar")
	if !admin && !e.EsSuya(rc["unidad_id"].(int64)) {
		P.Fallo(w, r, P.NoEncontrado("este recibo"))
		return
	}
	var in struct {
		MontoCts        int64  `json:"monto_cts"`
		Medio           string `json:"medio"`
		CodigoOperacion string `json:"codigo_operacion"`
		Fecha           string `json:"fecha"`
	}
	var vouchers []Subido
	if esMultipart(r) {
		if err := leerMultipart(r); err != nil {
			P.Fallo(w, r, err)
			return
		}
		in.MontoCts, _ = strconv.ParseInt(campo(r, "monto_cts"), 10, 64)
		in.Medio, in.CodigoOperacion, in.Fecha = campo(r, "medio"), campo(r, "codigo_operacion"), campo(r, "fecha")
		if vouchers, err = archivosDeForm(r, "voucher", "foto"); err != nil {
			P.Fallo(w, r, err)
			return
		}
	} else if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	estadoRec := rc["estado"].(string)
	if estadoRec == "borrador" || estadoRec == "anulado" {
		P.Fallo(w, r, P.Conflicto("RECIBO_NO_COBRABLE", "Ese recibo está "+estadoRec+"."))
		return
	}
	saldo := rc["saldo_cts"].(int64)
	ev := P.Validacion("Revisa los datos del pago.")
	if in.MontoCts <= 0 {
		ev.Campo("monto_cts", "El monto debe ser mayor que cero.")
	} else if in.MontoCts > saldo {
		ev = P.Err(http.StatusUnprocessableEntity, "MONTO_MAYOR_AL_SALDO", "El monto pasa el saldo del recibo ("+P.Soles(saldo)+").").Campo("monto_cts", "Máximo "+P.Soles(saldo)+".")
	}
	switch in.Medio {
	case "yape", "plin", "transferencia", "deposito", "tarjeta":
		if strings.TrimSpace(in.CodigoOperacion) == "" {
			ev.Campo("codigo_operacion", "Escribe el código de operación del voucher.")
		}
	case "efectivo":
		if !admin {
			ev.Campo("medio", "Informa pagos por Yape, Plin, transferencia o depósito.")
		}
	default:
		ev.Campo("medio", "Elige yape, plin, transferencia, deposito o efectivo.")
	}
	if !admin && len(vouchers) == 0 {
		ev.Campo("voucher", "Sube la foto del voucher.")
	}
	fecha := time.Now().In(P.Lima).Format("2006-01-02")
	if in.Fecha != "" {
		if _, err := time.Parse("2006-01-02", in.Fecha); err != nil {
			ev.Campo("fecha", "Formato AAAA-MM-DD.")
		}
		fecha = in.Fecha
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	se := ses(r)
	var voucher *int64
	if len(vouchers) > 0 {
		id, err := s.guardarArchivo(ctx, tx, e.ID, &se.UsuarioID, vouchers[0])
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		voucher = &id
	}
	estado := "pendiente_validacion"
	var validadoPor *int64
	if admin {
		estado = "validado"
		validadoPor = &se.UsuarioID
	}
	var codigo *string
	if c := strings.TrimSpace(in.CodigoOperacion); c != "" {
		codigo = &c
	}
	pago, err := db.Fila(ctx, tx, `INSERT INTO pago (edificio_id, recibo_id, monto_cts, medio, codigo_operacion, fecha, voucher_id, estado, registrado_por, validado_por, validado_en)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, CASE WHEN $10::bigint IS NULL THEN NULL ELSE now() END)
		RETURNING id, monto_cts, medio, codigo_operacion, to_char(fecha,'YYYY-MM-DD') AS fecha, estado, voucher_id`,
		e.ID, rid, in.MontoCts, in.Medio, codigo, fecha, voucher, estado, se.UsuarioID, validadoPor)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if estado == "validado" {
		if pid, ok := pago["id"].(int64); ok {
			if fd, e2 := time.Parse("2006-01-02", fecha); e2 == nil {
				if err := s.asentarIngresoPago(ctx, tx, e.ID, pid, rid, in.MontoCts, fd); err != nil {
					P.Fallo(w, r, err)
					return
				}
			}
		}
	}
	recibo, err := db.Fila(ctx, tx, `SELECT id, estado, total_cts, pagado_cts, total_cts - pagado_cts AS saldo_cts FROM recibo WHERE id=$1`, rid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	pago["recibo"] = recibo
	if estado == "pendiente_validacion" {
		pago["mensaje"] = "Pago enviado, en revisión. La deuda baja cuando la administración lo valide."
	}
	P.JSON(w, http.StatusCreated, pago)
}

// listarPagos: GET /pagos?estado=pendiente_validacion — vouchers por validar.
func (s *Server) listarPagos(w http.ResponseWriter, r *http.Request) {
	estado := r.URL.Query().Get("estado")
	if estado == "" {
		estado = "pendiente_validacion"
	}
	filas, err := db.Filas(r.Context(), s.DB, `SELECT pg.id, pg.recibo_id, r.numero, u.codigo AS unidad, pg.monto_cts, pg.medio, pg.codigo_operacion,
			to_char(pg.fecha,'YYYY-MM-DD') AS fecha, pg.estado, pg.voucher_id, pg.creado_en
		FROM pago pg JOIN recibo r ON r.id=pg.recibo_id JOIN unidad u ON u.id=r.unidad_id
		WHERE pg.edificio_id=$1 AND pg.estado=$2 ORDER BY pg.creado_en DESC LIMIT 200`, edf(r).ID, estado)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, f := range filas {
		if v, ok := f["voucher_id"].(int64); ok {
			f["voucher_url"] = s.Firma.URL(v)
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// validarPago: PATCH /pagos/{pid} {estado: validado|rechazado, motivo}.
func (s *Server) validarPago(w http.ResponseWriter, r *http.Request) {
	pid, err := idRuta(r, "pid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Estado string `json:"estado"`
		Motivo string `json:"motivo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.Estado != "validado" && in.Estado != "rechazado" {
		P.Fallo(w, r, P.Validacion("Estado inválido.").Campo("estado", "validado o rechazado."))
		return
	}
	if in.Estado == "rechazado" && strings.TrimSpace(in.Motivo) == "" {
		P.Fallo(w, r, P.Validacion("Explica por qué se rechaza.").Campo("motivo", "Obligatorio al rechazar."))
		return
	}
	e := edf(r)
	ctx := r.Context()
	fila, err := db.Fila(ctx, s.DB, `UPDATE pago SET estado=$3, motivo=NULLIF($4,''), validado_por=$5, validado_en=now()
		WHERE id=$1 AND edificio_id=$2 AND estado='pendiente_validacion'
		RETURNING id, recibo_id, monto_cts, estado, motivo`, pid, e.ID, in.Estado, in.Motivo, ses(r).UsuarioID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			P.Fallo(w, r, P.Conflicto("PAGO_YA_REVISADO", "Ese pago no existe o ya se revisó."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "recibos", "pago."+in.Estado, "pago", pid, nil, fila)
	if in.Estado == "validado" {
		var fecha time.Time
		_ = s.DB.QueryRow(ctx, `SELECT fecha FROM pago WHERE id=$1`, pid).Scan(&fecha)
		_ = s.asentarIngresoPago(ctx, s.DB, e.ID, pid, fila["recibo_id"].(int64), fila["monto_cts"].(int64), fecha)
	} else {
		_ = s.revertirPago(ctx, s.DB, pid)
	}
	recibo, _ := db.Fila(ctx, s.DB, `SELECT id, estado, total_cts, pagado_cts, total_cts - pagado_cts AS saldo_cts FROM recibo WHERE id=$1`, fila["recibo_id"])
	fila["recibo"] = recibo
	P.JSON(w, http.StatusOK, fila)
}

// anularRecibo: POST /recibos/{rid}/anular {motivo} (solo sin pagos validados).
func (s *Server) anularRecibo(w http.ResponseWriter, r *http.Request) {
	rid, err := idRuta(r, "rid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if strings.TrimSpace(in.Motivo) == "" {
		P.Fallo(w, r, P.Validacion("Escribe el motivo de la anulación.").Campo("motivo", "Obligatorio."))
		return
	}
	e := edf(r)
	ctx := r.Context()
	var pagado int64
	var estado string
	if err := s.DB.QueryRow(ctx, `SELECT pagado_cts, estado FROM recibo WHERE id=$1 AND edificio_id=$2`, rid, e.ID).Scan(&pagado, &estado); err != nil {
		P.Fallo(w, r, P.NoEncontrado("este recibo"))
		return
	}
	if pagado > 0 {
		P.Fallo(w, r, P.Conflicto("TIENE_PAGOS", "El recibo tiene pagos: no se puede anular."))
		return
	}
	if estado == "anulado" {
		P.Fallo(w, r, P.Conflicto("YA_ANULADO", "El recibo ya estaba anulado."))
		return
	}
	if _, err := s.DB.Exec(ctx, `UPDATE recibo SET estado='anulado', anulado_motivo=$2 WHERE id=$1`, rid, in.Motivo); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "recibos", "anular", "recibo", rid, map[string]any{"estado": estado}, map[string]any{"estado": "anulado", "motivo": in.Motivo})
	P.JSON(w, http.StatusOK, map[string]any{"id": rid, "estado": "anulado"})
}

// morosidad: GET /morosidad?periodo= → índice del periodo (misma función que 03 y 04) y deuda por unidad.
func (s *Server) morosidad(w http.ResponseWriter, r *http.Request) {
	periodo, err := s.periodoDe(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	a, err := s.ArbolBalance(ctx, s.DB, e.ID, periodo, vistaDe(e))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	unidades, err := s.deudaPorUnidad(ctx, e.ID, nil)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	m := a.KPIs.Morosidad
	P.JSON(w, http.StatusOK, map[string]any{"periodo": periodo, "indice_pct": m.Pct, "monto_cts": m.MontoCts,
		"emitido_cts": a.KPIs.EmitidoCts, "unidades_con_saldo": m.Unidades, "unidades": unidades,
		"del_mes":   map[string]any{"pct": m.Pct, "monto_cts": m.MontoCts, "unidades": m.Unidades},
		"historica": map[string]any{"pct": m.HistoricaPct, "monto_cts": m.HistoricaMontoCts, "unidades": m.HistoricaUnidades, "deuda_inicial_cts": m.DeudaInicialCts}})
}

// deudaPorUnidad: saldo pendiente por unidad y por mes. unidades=nil → todas las del edificio.
func (s *Server) deudaPorUnidad(ctx context.Context, eid int64, unidades []int64) ([]map[string]any, error) {
	filas, err := db.Filas(ctx, s.DB, `SELECT u.id AS unidad_id, u.codigo AS unidad,
			COALESCE((SELECT pe.nombre FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id WHERE up.unidad_id=u.id AND up.rol='propietario' AND up.hasta IS NULL LIMIT 1),'') AS propietario,
			sum(r.total_cts - r.pagado_cts)::bigint AS deuda_cts,
			deuda_vencida_cts(u.id) AS deuda_vencida_cts,
			es_moroso(u.id) AS moroso,
			GREATEST(0, ((now() AT TIME ZONE 'America/Lima')::date - min(r.vence)))::int AS antiguedad_dias,
			COALESCE(sum(r.total_cts - r.pagado_cts) FILTER (WHERE r.origen='deuda_inicial'),0)::bigint AS deuda_inicial_cts,
			json_agg(json_build_object('periodo', p.periodo, 'recibo_id', r.id, 'saldo_cts', r.total_cts - r.pagado_cts, 'vence', to_char(r.vence,'YYYY-MM-DD'), 'origen', r.origen) ORDER BY r.vence, p.periodo) AS meses
		FROM recibo r JOIN unidad u ON u.id=r.unidad_id JOIN periodo p ON p.id=r.periodo_id
		WHERE r.edificio_id=$1 AND r.estado IN ('emitido','pagado_parcial') AND r.total_cts > r.pagado_cts
		  AND ($2::bigint[] IS NULL OR u.id = ANY($2))
		GROUP BY u.id ORDER BY deuda_cts DESC, u.codigo`, eid, unidades)
	return filas, err
}
