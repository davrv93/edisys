package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
	"edisys/api/internal/reparto"
)

// Corrección de lecturas en cascada (bloque 2).
//
// Corregir la lectura del periodo N cambia su consumo y también el del periodo N+1, porque la «anterior»
// de N+1 es la corregida. Si esos periodos ya tienen reparto aprobado, se rehace; si sus recibos siguen en
// borrador, se reescriben sus líneas de agua; si ya se emitieron, no se tocan: la diferencia de cada unidad
// queda como ajuste (nota de cargo o de abono interna) para su siguiente recibo.

// alertaLectura: NEGATIVO si el consumo es negativo; PICO si pasa el doble del promedio de los 3 meses previos.
func alertaLectura(ctx context.Context, q db.Q, mid int64, periodo string, consumo int64) *string {
	if consumo < 0 {
		a := "NEGATIVO"
		return &a
	}
	var prom float64
	_ = q.QueryRow(ctx, `SELECT COALESCE(avg(c),0) FROM (SELECT l.consumo AS c FROM lectura l JOIN periodo p ON p.id=l.periodo_id
		WHERE l.medidor_id=$1 AND p.periodo < $2 ORDER BY p.periodo DESC LIMIT 3) x`, mid, periodo).Scan(&prom)
	if prom > 0 && float64(consumo)/1000 > 2*prom {
		a := "PICO"
		return &a
	}
	return nil
}

type lecturaFila struct {
	id, medidor, periodoID int64
	periodo, tipo, unidad  string
	valor, anterior        int64
}

func leerLectura(ctx context.Context, q db.Q, sql string, args ...any) (*lecturaFila, error) {
	l := &lecturaFila{}
	var v, a string
	if err := q.QueryRow(ctx, sql, args...).Scan(&l.id, &l.medidor, &l.periodoID, &l.periodo, &l.tipo, &l.unidad, &v, &a); err != nil {
		return nil, err
	}
	l.valor, _ = P.Milesimas(v)
	l.anterior, _ = P.Milesimas(a)
	return l, nil
}

const sqlLecturaCascada = `SELECT l.id, l.medidor_id, l.periodo_id, p.periodo, m.tipo, COALESCE(u.codigo,''), l.valor::text, l.anterior::text
	FROM lectura l JOIN medidor m ON m.id=l.medidor_id JOIN periodo p ON p.id=l.periodo_id LEFT JOIN unidad u ON u.id=m.unidad_id`

// corregirLectura: PUT /lecturas/{lid} {valor, motivo, confirmar_negativo?}.
// 200 {lectura, siguiente, repartos, ajustes} · 422 CONSUMO_NEGATIVO sin confirmar · 422 DIFERENCIA_NEGATIVA.
func (s *Server) corregirLectura(w http.ResponseWriter, r *http.Request) {
	lid, err := idRuta(r, "lid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Valor             string `json:"valor"`
		Motivo            string `json:"motivo"`
		ConfirmarNegativo bool   `json:"confirmar_negativo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	valor, err := P.Milesimas(in.Valor)
	if err != nil || valor < 0 || strings.TrimSpace(in.Motivo) == "" {
		P.Fallo(w, r, P.Validacion("Escribe el valor corregido y el motivo.").Campo("valor", "m³ con hasta 3 decimales.").Campo("motivo", "Obligatorio."))
		return
	}
	ctx := r.Context()
	e := edf(r)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	l, err := leerLectura(ctx, tx, sqlLecturaCascada+` WHERE l.id=$1 AND m.edificio_id=$2 FOR UPDATE OF l`, lid, e.ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("la lectura"))
		return
	}
	sig, err := leerLectura(ctx, tx, sqlLecturaCascada+` WHERE l.medidor_id=$1 AND p.periodo > $2 ORDER BY p.periodo LIMIT 1 FOR UPDATE OF l`, l.medidor, l.periodo)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		P.Fallo(w, r, err)
		return
	}
	consumo := valor - l.anterior
	var consumoSig int64
	if sig != nil {
		consumoSig = sig.valor - valor
	}
	if !in.ConfirmarNegativo && (consumo < 0 || (sig != nil && consumoSig < 0)) {
		msg := "La lectura corregida es menor que la anterior (" + strings.Replace(P.TextoMilesimas(l.anterior), ".", ",", 1) + ")."
		if consumo >= 0 {
			msg = "La lectura corregida es mayor que la del mes siguiente (" + strings.Replace(P.TextoMilesimas(sig.valor), ".", ",", 1) + "): el consumo de " + P.NombrePeriodo(sig.periodo) + " saldría negativo."
		}
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "CONSUMO_NEGATIVO", msg+" Si hubo cambio de medidor, confirma.").
			Campo("confirmar_negativo", "Confirma que hubo cambio de medidor.").Con("lectura_anterior", P.TextoMilesimas(l.anterior)))
		return
	}
	antes := map[string]any{"valor": P.TextoMilesimas(l.valor), "consumo": P.TextoMilesimas(l.valor - l.anterior)}
	f, err := db.Fila(ctx, tx, `UPDATE lectura SET valor=$2::numeric, consumo=$3::numeric, alerta=$4, motivo=$5 WHERE id=$1
		RETURNING id, valor::text AS valor, anterior::text AS lectura_anterior, consumo::text AS consumo, alerta, motivo`,
		lid, P.TextoMilesimas(valor), P.TextoMilesimas(consumo), alertaLectura(ctx, tx, l.medidor, l.periodo, consumo), in.Motivo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	periodos := []*lecturaFila{l}
	var fs map[string]any
	if sig != nil {
		fs, err = db.Fila(ctx, tx, `UPDATE lectura SET anterior=$2::numeric, consumo=$3::numeric, alerta=$4 WHERE id=$1
			RETURNING id, valor::text AS valor, anterior::text AS lectura_anterior, consumo::text AS consumo, alerta`,
			sig.id, P.TextoMilesimas(valor), P.TextoMilesimas(consumoSig), alertaLectura(ctx, tx, sig.medidor, sig.periodo, consumoSig))
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		fs["periodo"] = sig.periodo
		periodos = append(periodos, sig)
	}
	motivo := fmt.Sprintf("Corrección de la lectura del Dpto %s de %s: %s", l.unidad, P.NombrePeriodo(l.periodo), strings.TrimSpace(in.Motivo))
	uid := ses(r).UsuarioID
	repartos := []map[string]any{}
	ajustes := []map[string]any{}
	for _, lp := range periodos {
		rp, ajs, err := s.rehacerReparto(ctx, tx, e.ID, lp.periodoID, lp.periodo, l.tipo, motivo, lid, uid)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		if rp != nil {
			repartos = append(repartos, rp)
		}
		ajustes = append(ajustes, ajs...)
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	f["periodo"] = l.periodo
	resp := map[string]any{"lectura": f, "siguiente": fs, "repartos": repartos, "ajustes": ajustes}
	s.auditarCambio(ctx, s.DB, r, "lecturas", "corregir", "lectura", lid, antes, resp)
	// Compatibilidad: los campos de la lectura corregida también van en la raíz.
	for k, v := range f {
		resp[k] = v
	}
	P.JSON(w, http.StatusOK, resp)
}

// rehacerReparto recalcula el reparto aprobado del periodo (si lo hay). Con recibos en borrador reescribe sus
// líneas de agua; con recibos emitidos crea un ajuste por unidad con la diferencia. nil si no había reparto.
func (s *Server) rehacerReparto(ctx context.Context, tx pgx.Tx, eid, pid int64, periodo, tipo, motivo string, lid, uid int64) (map[string]any, []map[string]any, error) {
	var viejasB []byte
	err := tx.QueryRow(ctx, `SELECT lineas FROM reparto_medidor WHERE periodo_id=$1 AND tipo=$2 FOR UPDATE`, pid, tipo).Scan(&viejasB)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	} else if err != nil {
		return nil, nil, err
	}
	_, res, _, err := s.CalcularReparto(ctx, tx, eid, periodo, tipo)
	if err != nil {
		return nil, nil, err
	}
	nuevas, _ := json.Marshal(res.Lineas)
	if _, err := tx.Exec(ctx, `UPDATE reparto_medidor SET tarifa_cts_x_1000=$3, total_unidades_cts=$4, diferencia_cts=$5, lineas=$6, aprobado_por=$7, aprobado_en=now()
		WHERE periodo_id=$1 AND tipo=$2`, pid, tipo, res.TarifaCtsX1000, res.TotalUnidadesCts, res.DiferenciaCts, nuevas, uid); err != nil {
		return nil, nil, err
	}
	var emitidos int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM recibo WHERE periodo_id=$1 AND origen='periodo' AND estado NOT IN ('borrador','anulado')`, pid).Scan(&emitidos); err != nil {
		return nil, nil, err
	}
	var suma int64
	for _, l := range res.Lineas {
		suma += l.TotalCts
	}
	out := map[string]any{"periodo": periodo, "tipo": tipo, "monto_general_cts": res.MontoGeneralCts, "total_unidades_cts": res.TotalUnidadesCts,
		"diferencia_cts": res.DiferenciaCts, "suma_lineas_cts": suma, "emitido": emitidos > 0}
	ajustes := []map[string]any{}
	if emitidos == 0 {
		if tipo == "agua" {
			n, err := aguaEnBorradores(ctx, tx, pid, res.Lineas)
			if err != nil {
				return nil, nil, err
			}
			out["recibos_actualizados"] = n
		}
		return out, ajustes, nil
	}
	var viejas []reparto.Linea
	_ = json.Unmarshal(viejasB, &viejas)
	previo := map[int64]int64{}
	for _, l := range viejas {
		previo[l.UnidadID] = l.TotalCts
	}
	var sumaAjustes int64
	for _, l := range res.Lineas {
		delta := l.TotalCts - previo[l.UnidadID]
		if delta == 0 {
			continue
		}
		a, err := s.crearAjuste(ctx, tx, eid, l.UnidadID, delta, motivo+" (reparto de "+P.NombrePeriodo(periodo)+")", periodo, &lid, uid)
		if err != nil {
			return nil, nil, err
		}
		a["unidad"] = l.Unidad
		ajustes = append(ajustes, a)
		sumaAjustes += delta
	}
	out["ajustes"] = len(ajustes)
	out["suma_ajustes_cts"] = sumaAjustes
	return out, ajustes, nil
}

// crearAjuste guarda la nota de cargo/abono y, si la unidad ya tiene el recibo siguiente en borrador, la aplica ahí.
func (s *Server) crearAjuste(ctx context.Context, tx pgx.Tx, eid, unidad, monto int64, motivo, periodo string, lectura *int64, uid int64) (map[string]any, error) {
	a, err := db.Fila(ctx, tx, `INSERT INTO ajuste (edificio_id, unidad_id, monto_cts, motivo, periodo_origen, lectura_id, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, unidad_id, monto_cts, tipo, motivo, periodo_origen`, eid, unidad, monto, motivo, periodo, lectura, uid)
	if err != nil {
		return nil, err
	}
	var rid, total int64
	err = tx.QueryRow(ctx, `SELECT r.id, r.total_cts FROM recibo r JOIN periodo p ON p.id=r.periodo_id
		WHERE r.unidad_id=$1 AND r.estado='borrador' AND r.origen='periodo' AND p.periodo > $2 ORDER BY p.periodo LIMIT 1`, unidad, periodo).Scan(&rid, &total)
	a["recibo_id"] = nil
	if err == nil && total+monto >= 0 {
		if err := aplicarAjusteEnRecibo(ctx, tx, a["id"].(int64), rid, motivo, monto); err != nil {
			return nil, err
		}
		a["recibo_id"] = rid
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return a, nil
}

func aplicarAjusteEnRecibo(ctx context.Context, tx pgx.Tx, ajuste, rid int64, motivo string, monto int64) error {
	if _, err := tx.Exec(ctx, `INSERT INTO recibo_linea (recibo_id, tipo, descripcion, monto_cts, orden, ajuste_id) VALUES ($1,'ajuste',$2,$3,90,$4)`,
		rid, descAjuste(motivo, monto), monto, ajuste); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE recibo SET total_cts=(SELECT COALESCE(sum(monto_cts),0) FROM recibo_linea WHERE recibo_id=$1) WHERE id=$1`, rid); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE ajuste SET recibo_id=$2 WHERE id=$1`, ajuste, rid)
	return err
}

func descAjuste(motivo string, monto int64) string {
	t := "Nota de cargo"
	if monto < 0 {
		t = "Nota de abono"
	}
	return recortar(t+" · "+motivo, 160)
}

// listarAjustes: GET /ajustes?pendientes=1&unidad_id=
func (s *Server) listarAjustes(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	q := r.URL.Query()
	filas, err := db.Filas(r.Context(), s.DB, `SELECT a.id, a.unidad_id, u.codigo AS unidad, a.monto_cts, a.tipo, a.motivo, a.periodo_origen, a.lectura_id,
			a.recibo_id, rc.numero AS recibo_numero, a.creado_en
		FROM ajuste a JOIN unidad u ON u.id=a.unidad_id LEFT JOIN recibo rc ON rc.id=a.recibo_id
		WHERE a.edificio_id=$1 AND ($2 = '' OR a.recibo_id IS NULL) AND ($3 = '' OR a.unidad_id::text = $3)
		ORDER BY a.id DESC LIMIT 500`, e.ID, q.Get("pendientes"), q.Get("unidad_id"))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}
