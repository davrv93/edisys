package app

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Nodo del árbol del balance (04). Los id son legibles y estables: ing, egr, egr.administracion,
// egr.administracion.conserjeria, egr.administracion.conserjeria.e12, ing.cuotas.r57…
type Nodo struct {
	ID                   string  `json:"id"`
	Tipo                 string  `json:"tipo"` // raiz | ingresos | egresos | rubro | concepto | documento
	Nombre               string  `json:"nombre"`
	TotalCts             int64   `json:"total_cts"`
	PctPadre             float64 `json:"pct_padre"`
	TieneHijos           bool    `json:"tiene_hijos"`
	SinSustento          bool    `json:"sin_sustento"`
	SinSustentoN         int     `json:"sin_sustento_n,omitempty"`
	DocumentoID          *int64  `json:"documento_id"`
	DocumentoTipo        string  `json:"documento_tipo,omitempty"`
	DocumentoRestringido bool    `json:"documento_restringido,omitempty"`
	Fecha                string  `json:"fecha,omitempty"`
	Estado               string  `json:"estado,omitempty"`
	Origen               string  `json:"origen,omitempty"`
	Hijos                []*Nodo `json:"hijos,omitempty"`
	orden                int
}

// Morosidad del periodo. «Del mes» (pct, monto_cts, unidades): saldo de los recibos del periodo ÷ emitido
// del periodo (§3 · 05). «Histórica»: toda la deuda vencida de la cuenta corriente —recibos anteriores y la
// deuda inicial importada— ÷ emitido del periodo.
type Morosidad struct {
	Pct               float64 `json:"pct"`
	Unidades          int     `json:"unidades"`
	MontoCts          int64   `json:"monto_cts"`
	HistoricaPct      float64 `json:"historica_pct"`
	HistoricaMontoCts int64   `json:"historica_monto_cts"`
	HistoricaUnidades int     `json:"historica_unidades"`
	DeudaInicialCts   int64   `json:"deuda_inicial_cts"`
}

// KPIs: los mismos 4 en 03, 04 y 10 (más emitido y banco).
type KPIs struct {
	IngresosCts int64     `json:"ingresos_cts"`
	EgresosCts  int64     `json:"egresos_cts"`
	SaldoCts    int64     `json:"saldo_cts"`
	EmitidoCts  int64     `json:"emitido_cts"`
	BancoCts    int64     `json:"banco_cts"`
	Morosidad   Morosidad `json:"morosidad"`
}

// Arbol del periodo con su índice por id.
type Arbol struct {
	Periodo  string
	Raiz     *Nodo
	Indice   map[string]*Nodo
	KPIs     KPIs
	HayDatos bool
}

// vista: qué puede ver quien pide el árbol.
type vista struct {
	soloLoSuyo bool
	propias    map[int64]bool
	verDocs    bool
}

func vistaDe(e *Edificio) vista {
	v := vista{soloLoSuyo: e.SoloLoSuyo(), verDocs: e.Puede("balance.ver_documentos"), propias: map[int64]bool{}}
	for _, u := range e.Unidades {
		v.propias[u] = true
	}
	return v
}

func pct(parte, total int64) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(parte)*1000/float64(total)) / 10
}

// ArbolBalance arma el árbol del periodo. Es la ÚNICA función que calcula ingresos, egresos,
// saldo y morosidad: la usan el balance (04), el dashboard (03), el portal (10) y la analítica.
func (s *Server) ArbolBalance(ctx context.Context, q db.Q, eid int64, periodo string, v vista) (*Arbol, error) {
	var nombreEd string
	var saldoInicial int64
	if err := q.QueryRow(ctx, `SELECT nombre, saldo_inicial_cts FROM edificio WHERE id=$1`, eid).Scan(&nombreEd, &saldoInicial); err != nil {
		return nil, err
	}
	a := &Arbol{Periodo: periodo, Indice: map[string]*Nodo{}}
	raiz := &Nodo{ID: "raiz", Tipo: "raiz", Nombre: nombreEd + " · " + P.NombrePeriodo(periodo)}
	ing := &Nodo{ID: "ing", Tipo: "ingresos", Nombre: "Ingresos (cobrado)"}
	egr := &Nodo{ID: "egr", Tipo: "egresos", Nombre: "Egresos"}
	raiz.Hijos = []*Nodo{ing, egr}
	a.Raiz = raiz

	// --- Ingresos: lo cobrado de los recibos del periodo, repartido por línea en su orden.
	type linea struct {
		tipo, desc string
		monto      int64
		reservaID  *int64
	}
	type recibo struct {
		id, unidadID, total, pagado int64
		numero, estado, codigo      string
		nombre                      string
		voucher                     *int64
		lineas                      []linea
	}
	filas, err := q.Query(ctx, `SELECT r.id, r.unidad_id, r.total_cts, r.pagado_cts, COALESCE(r.numero,''), r.estado, u.codigo,
			COALESCE((SELECT pe.nombre FROM unidad_persona up JOIN persona pe ON pe.id = up.persona_id
			          WHERE up.unidad_id = u.id AND up.rol = 'propietario' AND up.hasta IS NULL LIMIT 1), ''),
			(SELECT pg.voucher_id FROM pago pg WHERE pg.recibo_id = r.id AND pg.estado = 'validado' AND pg.voucher_id IS NOT NULL ORDER BY pg.fecha DESC, pg.id DESC LIMIT 1)
		FROM recibo r JOIN periodo p ON p.id = r.periodo_id JOIN unidad u ON u.id = r.unidad_id
		WHERE r.edificio_id = $1 AND p.periodo = $2 AND r.origen = 'periodo' AND r.estado NOT IN ('borrador','anulado')
		ORDER BY u.codigo`, eid, periodo)
	if err != nil {
		return nil, err
	}
	var recibos []*recibo
	porID := map[int64]*recibo{}
	for filas.Next() {
		rc := &recibo{}
		if err := filas.Scan(&rc.id, &rc.unidadID, &rc.total, &rc.pagado, &rc.numero, &rc.estado, &rc.codigo, &rc.nombre, &rc.voucher); err != nil {
			filas.Close()
			return nil, err
		}
		recibos = append(recibos, rc)
		porID[rc.id] = rc
	}
	filas.Close()
	if len(recibos) > 0 {
		idsR := make([]int64, 0, len(recibos))
		for _, rc := range recibos {
			idsR = append(idsR, rc.id)
		}
		fl, err := q.Query(ctx, `SELECT recibo_id, tipo, descripcion, monto_cts, reserva_id FROM recibo_linea WHERE recibo_id = ANY($1) ORDER BY recibo_id, orden, id`, idsR)
		if err != nil {
			return nil, err
		}
		for fl.Next() {
			var rid int64
			var l linea
			if err := fl.Scan(&rid, &l.tipo, &l.desc, &l.monto, &l.reservaID); err != nil {
				fl.Close()
				return nil, err
			}
			porID[rid].lineas = append(porID[rid].lineas, l)
		}
		fl.Close()
	}
	rubrosIng := map[string]*Nodo{
		"cuotas":   {ID: "ing.cuotas", Tipo: "rubro", Nombre: "Cuotas de mantenimiento", orden: 1},
		"agua":     {ID: "ing.agua", Tipo: "rubro", Nombre: "Agua y áreas comunes (reparto de medidores)", orden: 2},
		"reservas": {ID: "ing.reservas", Tipo: "rubro", Nombre: "Reservas de áreas", orden: 3},
	}
	grupo := func(tipo string) string {
		switch tipo {
		case "agua", "agua_comun", "energia_comun":
			return "agua"
		case "reserva":
			return "reservas"
		}
		return "cuotas"
	}
	for _, rc := range recibos {
		a.HayDatos = true
		a.KPIs.EmitidoCts += rc.total
		if pend := rc.total - rc.pagado; pend > 0 {
			a.KPIs.Morosidad.MontoCts += pend
			a.KPIs.Morosidad.Unidades++
		}
		propia := v.propias[rc.unidadID]
		nombre := "Recibo " + rc.numero
		if !v.soloLoSuyo || propia {
			if rc.nombre != "" {
				nombre += " · " + rc.nombre
			}
		} else {
			nombre = "Recibo " + rc.numero + " · Dpto " + rc.codigo
		}
		nombre += " (" + etiquetaEstado(rc.estado) + ")"
		var doc *int64
		restringido := false
		if rc.voucher != nil {
			if !v.soloLoSuyo || propia {
				doc = rc.voucher
			} else {
				restringido = true
			}
		}
		restante := rc.pagado
		porGrupo := map[string]int64{}
		for _, l := range rc.lineas {
			aplic := l.monto
			if aplic > restante {
				aplic = restante
			}
			if aplic < 0 {
				aplic = 0
			}
			restante -= aplic
			if aplic == 0 {
				continue
			}
			g := grupo(l.tipo)
			if g == "reservas" && l.reservaID != nil {
				n := &Nodo{ID: fmt.Sprintf("ing.reservas.v%d", *l.reservaID), Tipo: "documento", Nombre: l.desc, TotalCts: aplic,
					Origen: "cargada al recibo", Estado: rc.estado}
				if v.soloLoSuyo && !propia {
					n.Nombre = strings.SplitN(l.desc, " · ", 2)[0]
				}
				rubrosIng["reservas"].Hijos = append(rubrosIng["reservas"].Hijos, n)
				rubrosIng["reservas"].TotalCts += aplic
				continue
			}
			porGrupo[g] += aplic
		}
		for _, g := range []string{"cuotas", "agua", "reservas"} {
			if porGrupo[g] == 0 {
				continue
			}
			n := &Nodo{ID: fmt.Sprintf("ing.%s.r%d", g, rc.id), Tipo: "documento", Nombre: nombre, TotalCts: porGrupo[g],
				DocumentoID: doc, DocumentoTipo: "voucher", DocumentoRestringido: restringido, Estado: rc.estado, Origen: "recibo"}
			rubrosIng[g].Hijos = append(rubrosIng[g].Hijos, n)
			rubrosIng[g].TotalCts += porGrupo[g]
		}
	}
	// Reservas con pago inmediato validado cuyo uso cae en el periodo.
	ini, fin := P.RangoPeriodo(periodo)
	fr, err := q.Query(ctx, `SELECT rv.id, rv.codigo, rc.nombre, u.id, u.codigo, rv.total_cts, rv.voucher_id, to_char(rv.inicio AT TIME ZONE 'America/Lima','YYYY-MM-DD')
		FROM reserva rv JOIN recurso rc ON rc.id = rv.recurso_id JOIN unidad u ON u.id = rv.unidad_id
		WHERE rv.edificio_id=$1 AND rv.modo_cobro='pago_inmediato' AND rv.pago_validado AND rv.estado='confirmada'
		  AND rv.inicio >= $2 AND rv.inicio < $3 AND rv.total_cts > 0 ORDER BY rv.inicio`, eid, ini, fin)
	if err != nil {
		return nil, err
	}
	for fr.Next() {
		var id, uid, total int64
		var codigo, recurso, ucod, fecha string
		var voucher *int64
		if err := fr.Scan(&id, &codigo, &recurso, &uid, &ucod, &total, &voucher, &fecha); err != nil {
			fr.Close()
			return nil, err
		}
		n := &Nodo{ID: fmt.Sprintf("ing.reservas.v%d", id), Tipo: "documento", Nombre: codigo + " " + recurso + " · Dpto " + ucod,
			TotalCts: total, DocumentoTipo: "voucher", Fecha: fecha, Origen: "pago inmediato", Estado: "confirmada"}
		if !v.soloLoSuyo || v.propias[uid] {
			n.DocumentoID = voucher
		} else if voucher != nil {
			n.DocumentoRestringido = true
		}
		rubrosIng["reservas"].Hijos = append(rubrosIng["reservas"].Hijos, n)
		rubrosIng["reservas"].TotalCts += total
		a.HayDatos = true
	}
	fr.Close()
	// Deuda anterior recuperada: lo cobrado en el mes sobre los cargos de deuda inicial.
	rubrosIng["deuda"] = &Nodo{ID: "ing.deuda", Tipo: "rubro", Nombre: "Deuda anterior recuperada", orden: 4}
	fd, err := q.Query(ctx, `SELECT pg.id, r.id, r.unidad_id, u.codigo, COALESCE(r.numero,''), pg.monto_cts, to_char(pg.fecha,'YYYY-MM-DD'), pg.voucher_id
		FROM pago pg JOIN recibo r ON r.id = pg.recibo_id JOIN unidad u ON u.id = r.unidad_id
		WHERE pg.edificio_id=$1 AND pg.estado='validado' AND r.origen='deuda_inicial' AND to_char(pg.fecha,'YYYY-MM') = $2
		ORDER BY pg.fecha, pg.id`, eid, periodo)
	if err != nil {
		return nil, err
	}
	for fd.Next() {
		var pid, rid, uid, monto int64
		var ucod, numero, fecha string
		var voucher *int64
		if err := fd.Scan(&pid, &rid, &uid, &ucod, &numero, &monto, &fecha, &voucher); err != nil {
			fd.Close()
			return nil, err
		}
		n := &Nodo{ID: fmt.Sprintf("ing.deuda.p%d", pid), Tipo: "documento", Nombre: "Cobro de " + numero + " · Dpto " + ucod, TotalCts: monto,
			DocumentoTipo: "voucher", Fecha: fecha, Origen: "deuda inicial"}
		if !v.soloLoSuyo || v.propias[uid] {
			n.DocumentoID = voucher
		} else if voucher != nil {
			n.DocumentoRestringido = true
		}
		rubrosIng["deuda"].Hijos = append(rubrosIng["deuda"].Hijos, n)
		rubrosIng["deuda"].TotalCts += monto
		a.HayDatos = true
	}
	fd.Close()
	for _, g := range []string{"cuotas", "agua", "reservas", "deuda"} {
		if rubrosIng[g].TotalCts > 0 {
			ing.Hijos = append(ing.Hijos, rubrosIng[g])
			ing.TotalCts += rubrosIng[g].TotalCts
		}
	}

	// --- Egresos: rubro → concepto → documento.
	fe, err := q.Query(ctx, `SELECT e.id, e.descripcion, e.monto_cts, to_char(e.fecha,'YYYY-MM-DD'), e.documento_id, e.tipo_documento, e.origen,
			r.slug, r.nombre, r.orden, c.slug, c.nombre, c.orden
		FROM egreso e JOIN rubro r ON r.id = e.rubro_id LEFT JOIN concepto c ON c.id = e.concepto_id
		WHERE e.edificio_id=$1 AND e.periodo=$2 ORDER BY r.orden, r.id, c.orden NULLS LAST, c.id, e.fecha, e.id`, eid, periodo)
	if err != nil {
		return nil, err
	}
	rubros := map[string]*Nodo{}
	conceptos := map[string]*Nodo{}
	for fe.Next() {
		var id, monto int64
		var desc, fecha, tdoc, origen, rslug, rnom string
		var rord int
		var doc *int64
		var cslug, cnom *string
		var cord *int
		if err := fe.Scan(&id, &desc, &monto, &fecha, &doc, &tdoc, &origen, &rslug, &rnom, &rord, &cslug, &cnom, &cord); err != nil {
			fe.Close()
			return nil, err
		}
		a.HayDatos = true
		rn := rubros[rslug]
		if rn == nil {
			rn = &Nodo{ID: "egr." + rslug, Tipo: "rubro", Nombre: rnom, orden: rord}
			rubros[rslug] = rn
			egr.Hijos = append(egr.Hijos, rn)
		}
		padre := rn
		if cslug != nil {
			k := rslug + "." + *cslug
			cn := conceptos[k]
			if cn == nil {
				cn = &Nodo{ID: "egr." + k, Tipo: "concepto", Nombre: *cnom}
				conceptos[k] = cn
				rn.Hijos = append(rn.Hijos, cn)
			}
			cn.TotalCts += monto
			padre = cn
		}
		d := &Nodo{ID: fmt.Sprintf("%s.e%d", padre.ID, id), Tipo: "documento", Nombre: desc, TotalCts: monto, Fecha: fecha,
			DocumentoTipo: tdoc, Origen: origen, SinSustento: doc == nil}
		if doc != nil {
			if v.verDocs {
				d.DocumentoID = doc
			} else {
				d.DocumentoRestringido = true
			}
		}
		padre.Hijos = append(padre.Hijos, d)
		rn.TotalCts += monto
		egr.TotalCts += monto
	}
	fe.Close()

	raiz.TotalCts = ing.TotalCts - egr.TotalCts
	a.KPIs.IngresosCts = ing.TotalCts
	a.KPIs.EgresosCts = egr.TotalCts
	a.KPIs.SaldoCts = raiz.TotalCts
	a.KPIs.Morosidad.Pct = pct(a.KPIs.Morosidad.MontoCts, a.KPIs.EmitidoCts)

	// Morosidad histórica: deuda vencida (hora de Lima) de los recibos hasta el periodo y de toda la deuda inicial.
	if err := q.QueryRow(ctx, `SELECT COALESCE(sum(r.total_cts - r.pagado_cts),0)::bigint, count(DISTINCT r.unidad_id)::int,
			COALESCE(sum(r.total_cts - r.pagado_cts) FILTER (WHERE r.origen='deuda_inicial'),0)::bigint
		FROM recibo r JOIN periodo p ON p.id = r.periodo_id JOIN edificio e ON e.id = r.edificio_id
		WHERE r.edificio_id=$1 AND r.estado IN ('emitido','pagado_parcial') AND r.total_cts > r.pagado_cts
		  AND (r.origen = 'deuda_inicial' OR p.periodo <= $2)
		  AND r.vence IS NOT NULL AND r.vence + e.dias_gracia < (now() AT TIME ZONE 'America/Lima')::date`, eid, periodo).
		Scan(&a.KPIs.Morosidad.HistoricaMontoCts, &a.KPIs.Morosidad.HistoricaUnidades, &a.KPIs.Morosidad.DeudaInicialCts); err != nil {
		return nil, err
	}
	a.KPIs.Morosidad.HistoricaPct = pct(a.KPIs.Morosidad.HistoricaMontoCts, a.KPIs.EmitidoCts)

	// Banco acumulado = saldo inicial + todo lo cobrado − todo lo gastado hasta el periodo.
	var cobrado, reservasInm, gastado int64
	if err := q.QueryRow(ctx, `SELECT
		COALESCE((SELECT SUM(pg.monto_cts) FROM pago pg JOIN recibo r ON r.id = pg.recibo_id JOIN periodo p ON p.id = r.periodo_id
		          WHERE pg.edificio_id=$1 AND pg.estado='validado'
		            AND ((r.origen = 'periodo' AND p.periodo <= $2) OR (r.origen = 'deuda_inicial' AND to_char(pg.fecha,'YYYY-MM') <= $2))),0),
		COALESCE((SELECT SUM(total_cts) FROM reserva WHERE edificio_id=$1 AND modo_cobro='pago_inmediato' AND pago_validado AND estado='confirmada'
		          AND to_char(inicio AT TIME ZONE 'America/Lima','YYYY-MM') <= $2),0),
		COALESCE((SELECT SUM(monto_cts) FROM egreso WHERE edificio_id=$1 AND periodo <= $2),0)`, eid, periodo).Scan(&cobrado, &reservasInm, &gastado); err != nil {
		return nil, err
	}
	a.KPIs.BancoCts = saldoInicial + cobrado + reservasInm - gastado

	indexar(a, raiz)
	return a, nil
}

func indexar(a *Arbol, n *Nodo) {
	a.Indice[n.ID] = n
	n.TieneHijos = len(n.Hijos) > 0
	for _, h := range n.Hijos {
		if n.Tipo != "raiz" {
			h.PctPadre = pct(h.TotalCts, n.TotalCts)
		}
		indexar(a, h)
		if h.SinSustento {
			n.SinSustentoN++
		}
		n.SinSustentoN += h.SinSustentoN
	}
	if n.Tipo == "ingresos" || n.Tipo == "egresos" {
		total := a.Raiz.Hijos[0].TotalCts + a.Raiz.Hijos[1].TotalCts
		n.PctPadre = pct(n.TotalCts, total)
	}
}

func etiquetaEstado(e string) string {
	switch e {
	case "pagado":
		return "pagado"
	case "pagado_parcial":
		return "pago parcial"
	case "emitido":
		return "pendiente"
	}
	return e
}

// copia sin hijos para listar.
func plano(n *Nodo) *Nodo {
	c := *n
	c.Hijos = nil
	return &c
}

// periodoDe: ?periodo= o {p}; si no viene, el último periodo abierto del edificio (no futuro), o el mes actual.
func (s *Server) periodoDe(r *http.Request) (string, error) {
	p := r.URL.Query().Get("periodo")
	if p == "" {
		p = chi.URLParam(r, "p")
	}
	if p == "" {
		actual := P.PeriodoActual()
		if e := edf(r); e != nil {
			var ult *string
			_ = s.DB.QueryRow(r.Context(), `SELECT max(periodo) FROM periodo WHERE edificio_id=$1 AND periodo <= $2`, e.ID, actual).Scan(&ult)
			if ult != nil {
				return *ult, nil
			}
		}
		return actual, nil
	}
	if !P.PeriodoValido(p) {
		return "", P.Validacion("El periodo debe tener el formato AAAA-MM (ej. 2026-09).").Campo("periodo", "Formato AAAA-MM.")
	}
	return p, nil
}

// balance: GET /edificios/{eid}/balance?periodo= → { kpis, raiz (dos niveles abiertos) }.
func (s *Server) balance(w http.ResponseWriter, r *http.Request) {
	periodo, err := s.periodoDe(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	a, err := s.ArbolBalance(r.Context(), s.DB, e.ID, periodo, vistaDe(e))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	conc, _ := s.EstadoConciliacion(r.Context(), e.ID, periodo)
	raiz := plano(a.Raiz)
	for _, h := range a.Raiz.Hijos {
		ph := plano(h)
		for _, n := range h.Hijos {
			ph.Hijos = append(ph.Hijos, plano(n))
		}
		raiz.Hijos = append(raiz.Hijos, ph)
	}
	P.JSON(w, http.StatusOK, map[string]any{
		"periodo": periodo, "kpis": a.KPIs, "raiz": raiz, "hay_datos": a.HayDatos, "conciliacion": conc,
		"nota_ingresos": "Los ingresos cuentan lo cobrado. Lo emitido y no cobrado es la morosidad.",
	})
}

// balanceNodo: GET /balance/nodos/{nodo} → hijos del nodo (carga perezosa).
func (s *Server) balanceNodo(w http.ResponseWriter, r *http.Request) {
	periodo, err := s.periodoDe(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	a, err := s.ArbolBalance(r.Context(), s.DB, e.ID, periodo, vistaDe(e))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	n, ok := a.Indice[chi.URLParam(r, "nodo")]
	if !ok {
		P.Fallo(w, r, P.NoEncontrado("ese nodo del balance"))
		return
	}
	hijos := make([]*Nodo, 0, len(n.Hijos))
	for _, h := range n.Hijos {
		hijos = append(hijos, plano(h))
	}
	P.JSON(w, http.StatusOK, hijos)
}

// balanceDocumento: GET /balance/documentos/{doc} → documento con URL firmada (10 min).
func (s *Server) balanceDocumento(w http.ResponseWriter, r *http.Request) {
	id, err := idRuta(r, "doc")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	var nombre, mime, creado string
	if err := s.DB.QueryRow(ctx, `SELECT nombre, tipo_mime, to_char(creado_en AT TIME ZONE 'America/Lima','YYYY-MM-DD') FROM archivo WHERE id=$1 AND edificio_id=$2`, id, e.ID).
		Scan(&nombre, &mime, &creado); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el documento"))
		return
	}
	var origen, fecha string
	var monto int64
	var unidad *int64
	err = s.DB.QueryRow(ctx, `
		SELECT 'egreso', to_char(fecha,'YYYY-MM-DD'), monto_cts, NULL::bigint FROM egreso WHERE documento_id=$1 AND edificio_id=$2
		UNION ALL SELECT 'recibo', to_char(pg.fecha,'YYYY-MM-DD'), pg.monto_cts, r.unidad_id FROM pago pg JOIN recibo r ON r.id=pg.recibo_id WHERE pg.voucher_id=$1 AND pg.edificio_id=$2
		UNION ALL SELECT 'reserva', to_char(inicio AT TIME ZONE 'America/Lima','YYYY-MM-DD'), total_cts, unidad_id FROM reserva WHERE voucher_id=$1 AND edificio_id=$2
		UNION ALL SELECT 'recibo_general', to_char(rg.creado_en AT TIME ZONE 'America/Lima','YYYY-MM-DD'), rg.monto_cts, NULL FROM recibo_general rg JOIN periodo p ON p.id=rg.periodo_id WHERE rg.foto_id=$1 AND p.edificio_id=$2
		UNION ALL SELECT 'trabajo', to_char(i.actualizado_en AT TIME ZONE 'America/Lima','YYYY-MM-DD'), COALESCE(i.costo_real_cts, i.monto_presupuesto_cts, 0), NULL FROM incidencia i
		          WHERE (i.comprobante_id=$1 OR i.presupuesto_archivo_id=$1) AND i.edificio_id=$2
		LIMIT 1`, id, e.ID).Scan(&origen, &fecha, &monto, &unidad)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("el documento en el balance"))
		return
	}
	restringido := false
	switch origen {
	case "recibo", "reserva":
		restringido = e.SoloLoSuyo() && (unidad == nil || !e.EsSuya(*unidad))
	default:
		restringido = !e.Puede("balance.ver_documentos")
	}
	if restringido {
		P.Fallo(w, r, P.Prohibido("DOCUMENTO_RESTRINGIDO", "Documento disponible para la junta.").Con("permiso", "balance.ver_documentos"))
		return
	}
	tipo := "foto"
	if mime == "application/pdf" {
		tipo = "pdf"
	}
	if origen == "recibo" || origen == "reserva" {
		tipo = "voucher"
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "tipo": tipo, "nombre": nombre, "tipo_mime": mime, "url_firmada": s.Firma.URL(id),
		"vence_en": time.Now().Add(10 * time.Minute).UTC(), "fecha": fecha, "monto_cts": monto, "origen": origen})
}

// dashboard: GET /edificios/{eid}/dashboard?periodo= — un solo endpoint agregado (03).
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
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
	resp := map[string]any{"periodo": periodo, "kpis": a.KPIs, "hay_datos": a.HayDatos}
	ant := P.PeriodoAnterior(periodo)
	if b, err := s.ArbolBalance(ctx, s.DB, e.ID, ant, vistaDe(e)); err == nil && b.HayDatos {
		variacion := func(act, prev int64) any {
			if prev == 0 {
				return nil
			}
			return math.Round(float64(act-prev)*1000/float64(prev)) / 10
		}
		resp["variacion_vs_mes_anterior"] = map[string]any{
			"periodo": ant, "nombre_periodo": P.NombrePeriodo(ant),
			"ingresos_pct":     variacion(a.KPIs.IngresosCts, b.KPIs.IngresosCts),
			"egresos_pct":      variacion(a.KPIs.EgresosCts, b.KPIs.EgresosCts),
			"saldo_cts":        a.KPIs.SaldoCts - b.KPIs.SaldoCts,
			"morosidad_puntos": math.Round((a.KPIs.Morosidad.Pct-b.KPIs.Morosidad.Pct)*10) / 10,
		}
	} else {
		resp["variacion_vs_mes_anterior"] = nil
	}
	var emitidos, pagados, parciales, pendientes int
	if err := s.DB.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE r.estado='pagado'), count(*) FILTER (WHERE r.estado='pagado_parcial'),
		count(*) FILTER (WHERE r.estado='emitido') FROM recibo r JOIN periodo p ON p.id=r.periodo_id
		WHERE r.edificio_id=$1 AND p.periodo=$2 AND r.origen='periodo' AND r.estado NOT IN ('borrador','anulado')`, e.ID, periodo).Scan(&emitidos, &pagados, &parciales, &pendientes); err != nil {
		P.Fallo(w, r, err)
		return
	}
	resp["cobranza"] = map[string]any{"emitidos": emitidos, "pagados": pagados, "parciales": parciales, "pendientes": pendientes}

	var lectPend, vouchers, incVal, aprob, sinSust int
	var umbral int64
	if err := s.DB.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM medidor m WHERE m.edificio_id=$1 AND m.unidad_id IS NOT NULL AND m.activo AND m.tipo='agua'
		   AND NOT EXISTS (SELECT 1 FROM lectura l JOIN periodo p ON p.id=l.periodo_id WHERE l.medidor_id=m.id AND p.periodo=$2)),
		(SELECT count(*) FROM pago WHERE edificio_id=$1 AND estado='pendiente_validacion')
		  + (SELECT count(*) FROM reserva WHERE edificio_id=$1 AND voucher_id IS NOT NULL AND NOT pago_validado AND estado='pendiente_pago'),
		(SELECT count(*) FROM incidencia WHERE edificio_id=$1 AND estado='reportado'),
		(SELECT count(*) FROM incidencia i JOIN edificio e ON e.id=i.edificio_id WHERE i.edificio_id=$1 AND i.estado='presupuestado'),
		(SELECT count(*) FROM egreso WHERE edificio_id=$1 AND periodo=$2 AND documento_id IS NULL),
		(SELECT umbral_aprobacion_cts FROM edificio WHERE id=$1)`, e.ID, periodo).Scan(&lectPend, &vouchers, &incVal, &aprob, &sinSust, &umbral); err != nil {
		P.Fallo(w, r, err)
		return
	}
	resp["tareas"] = map[string]any{"lecturas_pendientes": lectPend, "vouchers_por_validar": vouchers, "incidencias_por_validar": incVal,
		"aprobaciones_pendientes": aprob, "egresos_sin_sustento": sinSust}

	trabajos, err := db.Filas(ctx, s.DB, `SELECT i.id, i.codigo, i.titulo, i.estado, i.criticidad, i.categoria, i.monto_presupuesto_cts, i.costo_real_cts,
			(SELECT count(*) FROM voto v WHERE v.incidencia_id=i.id AND v.voto='aprueba') AS votos_a_favor
		FROM incidencia i WHERE i.edificio_id=$1 AND (i.estado NOT IN ('terminado','descartado','rechazado')
		   OR to_char(i.actualizado_en AT TIME ZONE 'America/Lima','YYYY-MM') = $2)
		ORDER BY CASE i.criticidad WHEN 'critica' THEN 0 WHEN 'media' THEN 1 ELSE 2 END, i.numero DESC LIMIT 20`, e.ID, periodo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	resp["trabajos_mes"] = trabajos
	resp["ingresos_reservas_cts"] = int64(0)
	if n, ok := a.Indice["ing.reservas"]; ok {
		resp["ingresos_reservas_cts"] = n.TotalCts
	}
	prox, err := db.Filas(ctx, s.DB, `SELECT rv.id, rv.codigo, ar.nombre AS area, rc.nombre AS recurso, u.codigo AS unidad, rv.estado,
			rv.inicio, rv.fin, rv.total_cts
		FROM reserva rv JOIN recurso rc ON rc.id=rv.recurso_id JOIN area ar ON ar.id=rc.area_id JOIN unidad u ON u.id=rv.unidad_id
		WHERE rv.edificio_id=$1 AND rv.estado IN ('confirmada','pendiente_pago') AND rv.inicio >= now() AND rv.inicio < now() + interval '7 days'
		ORDER BY rv.inicio LIMIT 20`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	resp["proximas_reservas"] = prox
	P.JSON(w, http.StatusOK, resp)
}

// listarRubros: rubros con sus conceptos (para el formulario de egresos y presupuesto).
func (s *Server) listarRubros(w http.ResponseWriter, r *http.Request) {
	filas, err := db.Filas(r.Context(), s.DB, `SELECT r.id, r.slug, r.nombre,
		COALESCE(json_agg(json_build_object('id', c.id, 'slug', c.slug, 'nombre', c.nombre) ORDER BY c.orden, c.id) FILTER (WHERE c.id IS NOT NULL), '[]') AS conceptos
		FROM rubro r LEFT JOIN concepto c ON c.rubro_id = r.id WHERE r.edificio_id=$1 GROUP BY r.id ORDER BY r.orden, r.id`, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas})
}

// listarEgresos: GET /egresos?periodo=
func (s *Server) listarEgresos(w http.ResponseWriter, r *http.Request) {
	periodo, err := s.periodoDe(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT e.id, e.periodo, to_char(e.fecha,'YYYY-MM-DD') AS fecha, e.descripcion, e.monto_cts,
			r.nombre AS rubro, c.nombre AS concepto, e.documento_id, e.tipo_documento, e.origen, (e.documento_id IS NULL) AS sin_sustento
		FROM egreso e JOIN rubro r ON r.id=e.rubro_id LEFT JOIN concepto c ON c.id=e.concepto_id
		WHERE e.edificio_id=$1 AND e.periodo=$2 ORDER BY r.orden, e.fecha, e.id`, e.ID, periodo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if !e.Puede("balance.ver_documentos") {
		for _, f := range filas {
			f["documento_id"] = nil
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearEgreso: POST /egresos (multipart o JSON) {rubro_id|rubro, concepto_id?|concepto?, descripcion, monto_cts, fecha, periodo?, documento?}.
func (s *Server) crearEgreso(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	var in struct {
		RubroID     int64  `json:"rubro_id"`
		Rubro       string `json:"rubro"`
		ConceptoID  int64  `json:"concepto_id"`
		Concepto    string `json:"concepto"`
		Descripcion string `json:"descripcion"`
		MontoCts    int64  `json:"monto_cts"`
		Fecha       string `json:"fecha"`
		Periodo     string `json:"periodo"`
	}
	var docs []Subido
	if esMultipart(r) {
		if err := leerMultipart(r); err != nil {
			P.Fallo(w, r, err)
			return
		}
		in.RubroID, _ = strconv.ParseInt(campo(r, "rubro_id"), 10, 64)
		in.Rubro = campo(r, "rubro")
		in.ConceptoID, _ = strconv.ParseInt(campo(r, "concepto_id"), 10, 64)
		in.Concepto = campo(r, "concepto")
		in.Descripcion = campo(r, "descripcion")
		in.MontoCts, _ = strconv.ParseInt(campo(r, "monto_cts"), 10, 64)
		in.Fecha = campo(r, "fecha")
		in.Periodo = campo(r, "periodo")
		var err error
		if docs, err = archivosDeForm(r, "documento", "archivo", "foto"); err != nil {
			P.Fallo(w, r, err)
			return
		}
	} else if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa los datos del egreso.")
	if in.MontoCts <= 0 {
		ev.Campo("monto_cts", "El monto debe ser mayor que cero.")
	}
	if strings.TrimSpace(in.Descripcion) == "" {
		ev.Campo("descripcion", "Describe el egreso.")
	}
	fecha, errF := time.Parse("2006-01-02", in.Fecha)
	if in.Fecha == "" {
		fecha, errF = time.Now().In(P.Lima), nil
	}
	if errF != nil {
		ev.Campo("fecha", "Formato AAAA-MM-DD.")
	}
	if in.Periodo == "" {
		in.Periodo = fecha.Format("2006-01")
	}
	if !P.PeriodoValido(in.Periodo) {
		ev.Campo("periodo", "Formato AAAA-MM.")
	}
	if in.RubroID == 0 && in.Rubro != "" {
		_ = s.DB.QueryRow(ctx, `SELECT id FROM rubro WHERE edificio_id=$1 AND (slug=$2 OR lower(nombre)=lower($2))`, e.ID, in.Rubro).Scan(&in.RubroID)
	}
	var ok bool
	_ = s.DB.QueryRow(ctx, `SELECT true FROM rubro WHERE id=$1 AND edificio_id=$2`, in.RubroID, e.ID).Scan(&ok)
	if !ok {
		ev.Campo("rubro_id", "Elige un rubro del edificio.")
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
	var concepto *int64
	if in.ConceptoID > 0 {
		concepto = &in.ConceptoID
	} else if strings.TrimSpace(in.Concepto) != "" {
		var cid int64
		slug := slugify(in.Concepto)
		if err := tx.QueryRow(ctx, `INSERT INTO concepto (rubro_id, slug, nombre, orden) VALUES ($1,$2,$3,99)
			ON CONFLICT (rubro_id, slug) DO UPDATE SET nombre = concepto.nombre RETURNING id`, in.RubroID, slug, strings.TrimSpace(in.Concepto)).Scan(&cid); err != nil {
			P.Fallo(w, r, err)
			return
		}
		concepto = &cid
	}
	se := ses(r)
	var docID *int64
	tipoDoc := "foto"
	if len(docs) > 0 {
		id, err := s.guardarArchivo(ctx, tx, e.ID, &se.UsuarioID, docs[0])
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		docID = &id
		if docs[0].Mime == "application/pdf" {
			tipoDoc = "pdf"
		}
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO egreso (edificio_id, periodo, rubro_id, concepto_id, descripcion, monto_cts, fecha, documento_id, tipo_documento, registrado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, e.ID, in.Periodo, in.RubroID, concepto, strings.TrimSpace(in.Descripcion), in.MontoCts, fecha, docID, tipoDoc, se.UsuarioID).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "periodo": in.Periodo, "monto_cts": in.MontoCts, "sin_sustento": docID == nil, "documento_id": docID})
}

func slugify(s string) string {
	t := strings.ToLower(strings.TrimSpace(s))
	repl := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", "ü", "u")
	t = repl.Replace(t)
	var b strings.Builder
	guion := false
	for _, c := range t {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
			guion = false
		} else if !guion && b.Len() > 0 {
			b.WriteByte('_')
			guion = true
		}
	}
	return strings.Trim(b.String(), "_")
}
