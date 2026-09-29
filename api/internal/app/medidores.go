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

func tipoMedidor(r *http.Request) string {
	t := r.URL.Query().Get("tipo")
	if t == "" && r.MultipartForm != nil {
		t = campo(r, "tipo")
	}
	if t != "energia" {
		t = "agua"
	}
	return t
}

// listarLecturas: GET /lecturas?periodo=&tipo=agua → avance y medidores en el orden de la ronda.
func (s *Server) listarLecturas(w http.ResponseWriter, r *http.Request) {
	periodo, err := s.periodoDe(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	filas, err := db.Filas(ctx, s.DB, `SELECT m.id AS medidor_id, u.id AS unidad_id, u.codigo AS unidad, u.piso, m.serie, m.orden_ronda,
			COALESCE((SELECT l2.valor FROM lectura l2 JOIN periodo p2 ON p2.id=l2.periodo_id WHERE l2.medidor_id=m.id AND p2.periodo < $2 ORDER BY p2.periodo DESC LIMIT 1), m.lectura_inicial)::text AS lectura_anterior,
			l.id AS lectura_id, l.valor::text AS lectura_actual, l.consumo::text AS consumo, l.foto_id, l.alerta, l.tomada_en, l.subida_en,
			CASE WHEN l.id IS NULL THEN 'pendiente' WHEN l.alerta IS NOT NULL THEN 'alerta' ELSE 'leida' END AS estado
		FROM medidor m JOIN unidad u ON u.id=m.unidad_id
		LEFT JOIN periodo p ON p.edificio_id=m.edificio_id AND p.periodo=$2
		LEFT JOIN lectura l ON l.medidor_id=m.id AND l.periodo_id=p.id
		WHERE m.edificio_id=$1 AND m.tipo=$3 AND m.activo AND u.activo ORDER BY m.orden_ronda, u.codigo`, e.ID, periodo, tipoMedidor(r))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	leidas := 0
	for _, f := range filas {
		if id, ok := f["foto_id"].(int64); ok {
			f["foto_url"] = s.Firma.URL(id)
			leidas++
		} else {
			f["foto_url"] = nil
		}
		delete(f, "foto_id")
	}
	var abierto bool
	_ = s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM periodo WHERE edificio_id=$1 AND periodo=$2)`, e.ID, periodo).Scan(&abierto)
	P.JSON(w, http.StatusOK, map[string]any{"periodo": periodo, "tipo": tipoMedidor(r), "periodo_abierto": abierto,
		"avance": map[string]any{"leidas": leidas, "total": len(filas)}, "medidores": filas})
}

// registrarLectura: POST /medidores/{mid}/lecturas (multipart {periodo, valor, foto, tomada_en, motivo}).
// 201 {consumo, alerta} · 422 FOTO_OBLIGATORIA · 409 YA_LEIDO (un reintento con el mismo valor devuelve 200, idempotente).
func (s *Server) registrarLectura(w http.ResponseWriter, r *http.Request) {
	mid, err := idRuta(r, "mid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	fotos, err := archivosDeForm(r, "foto", "fotos", "fotos[]")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if len(fotos) == 0 {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "FOTO_OBLIGATORIA", "La foto del medidor es obligatoria.").Campo("foto", "Toma la foto del medidor."))
		return
	}
	if !strings.HasPrefix(fotos[0].Mime, "image/") {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "FOTO_OBLIGATORIA", "La foto debe ser una imagen.").Campo("foto", "JPG, PNG o WebP."))
		return
	}
	e := edf(r)
	ctx := r.Context()
	periodo := campo(r, "periodo")
	if periodo == "" {
		periodo = P.PeriodoActual()
	}
	valor, err := P.Milesimas(campo(r, "valor"))
	if err != nil || valor < 0 {
		P.Fallo(w, r, P.Validacion("Escribe la lectura en m³ (hasta 3 decimales).").Campo("valor", "Número mayor o igual a 0."))
		return
	}
	var tomada *time.Time
	if t, err := parseFecha(campo(r, "tomada_en")); err == nil {
		tomada = &t
	}
	motivo := campo(r, "motivo")
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var unidad string
	if err := tx.QueryRow(ctx, `SELECT u.codigo FROM medidor m JOIN unidad u ON u.id=m.unidad_id WHERE m.id=$1 AND m.edificio_id=$2`, mid, e.ID).Scan(&unidad); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el medidor"))
		return
	}
	var pid int64
	if err := tx.QueryRow(ctx, `SELECT id FROM periodo WHERE edificio_id=$1 AND periodo=$2`, e.ID, periodo).Scan(&pid); err != nil {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "PERIODO_NO_ABIERTO", "El periodo "+periodo+" no está abierto. Pide a la administración que lo abra."))
		return
	}
	// Idempotencia por medidor + periodo.
	var existenteID int64
	var existenteValor string
	err = tx.QueryRow(ctx, `SELECT id, valor::text FROM lectura WHERE medidor_id=$1 AND periodo_id=$2`, mid, pid).Scan(&existenteID, &existenteValor)
	if err == nil {
		if v, _ := P.Milesimas(existenteValor); v == valor {
			f, _ := db.Fila(ctx, tx, `SELECT id, consumo::text AS consumo, alerta, valor::text AS valor, anterior::text AS lectura_anterior FROM lectura WHERE id=$1`, existenteID)
			f["repetida"] = true
			P.JSON(w, http.StatusOK, f)
			return
		}
		P.Fallo(w, r, P.Conflicto("YA_LEIDO", "Ese medidor ya tiene lectura en "+periodo+". Para corregirla usa PUT /lecturas/"+strconv.FormatInt(existenteID, 10)+".").Con("lectura_id", existenteID))
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		P.Fallo(w, r, err)
		return
	}
	var anteriorTxt string
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT l.valor FROM lectura l JOIN periodo p ON p.id=l.periodo_id WHERE l.medidor_id=$1 AND p.periodo < $2 ORDER BY p.periodo DESC LIMIT 1),
		(SELECT lectura_inicial FROM medidor WHERE id=$1))::text`, mid, periodo).Scan(&anteriorTxt); err != nil {
		P.Fallo(w, r, err)
		return
	}
	anterior, _ := P.Milesimas(anteriorTxt)
	consumo := valor - anterior
	var alerta *string
	if consumo < 0 {
		if motivo == "" {
			P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "CONSUMO_NEGATIVO", "La lectura es menor que la anterior ("+strings.Replace(anteriorTxt, ".", ",", 1)+"). Confirma con un motivo (cambio de medidor, vuelta de contador).").
				Campo("motivo", "Obligatorio con consumo negativo.").Con("lectura_anterior", anteriorTxt))
			return
		}
		a := "NEGATIVO"
		alerta = &a
	} else {
		var prom float64
		_ = tx.QueryRow(ctx, `SELECT COALESCE(avg(c),0) FROM (SELECT l.consumo AS c FROM lectura l JOIN periodo p ON p.id=l.periodo_id
			WHERE l.medidor_id=$1 AND p.periodo < $2 ORDER BY p.periodo DESC LIMIT 3) x`, mid, periodo).Scan(&prom)
		if prom > 0 && float64(consumo)/1000 > 2*prom {
			a := "PICO"
			alerta = &a
		}
	}
	uid := ses(r).UsuarioID
	fid, err := s.guardarArchivo(ctx, tx, e.ID, &uid, fotos[0])
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	f, err := db.Fila(ctx, tx, `INSERT INTO lectura (medidor_id, periodo_id, valor, anterior, consumo, foto_id, tomada_en, operario_id, alerta, motivo)
		VALUES ($1,$2,$3::numeric,$4::numeric,$5::numeric,$6,$7,$8,$9,NULLIF($10,''))
		RETURNING id, valor::text AS valor, anterior::text AS lectura_anterior, consumo::text AS consumo, alerta`,
		mid, pid, P.TextoMilesimas(valor), anteriorTxt, P.TextoMilesimas(consumo), fid, tomada, uid, alerta, motivo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var leidas, total int
	_ = tx.QueryRow(ctx, `SELECT count(l.id), count(*) FROM medidor m JOIN unidad u ON u.id=m.unidad_id LEFT JOIN lectura l ON l.medidor_id=m.id AND l.periodo_id=$2
		WHERE m.edificio_id=$1 AND m.activo AND u.activo AND m.tipo=(SELECT tipo FROM medidor WHERE id=$3)`, e.ID, pid, mid).Scan(&leidas, &total)
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	f["unidad"] = unidad
	f["foto_url"] = s.Firma.URL(fid)
	f["avance"] = map[string]any{"leidas": leidas, "total": total}
	P.JSON(w, http.StatusCreated, f)
}

// corregirLectura: PUT /lecturas/{lid} {valor, motivo} — queda en auditoría; la foto original no se borra.
func (s *Server) corregirLectura(w http.ResponseWriter, r *http.Request) {
	lid, err := idRuta(r, "lid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Valor  string `json:"valor"`
		Motivo string `json:"motivo"`
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
	antes, err := db.Fila(ctx, s.DB, `SELECT l.valor::text AS valor, l.consumo::text AS consumo, l.anterior::text AS anterior, l.alerta FROM lectura l JOIN medidor m ON m.id=l.medidor_id
		WHERE l.id=$1 AND m.edificio_id=$2`, lid, e.ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("la lectura"))
		return
	}
	anterior, _ := P.Milesimas(antes["anterior"].(string))
	consumo := valor - anterior
	var alerta *string
	if consumo < 0 {
		a := "NEGATIVO"
		alerta = &a
	}
	f, err := db.Fila(ctx, s.DB, `UPDATE lectura SET valor=$2::numeric, consumo=$3::numeric, alerta=$4, motivo=$5 WHERE id=$1
		RETURNING id, valor::text AS valor, consumo::text AS consumo, alerta, motivo`, lid, P.TextoMilesimas(valor), P.TextoMilesimas(consumo), alerta, in.Motivo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "lecturas", "corregir", "lectura", lid, antes, f)
	P.JSON(w, http.StatusOK, f)
}

// registrarReciboGeneral: POST /periodos/{p}/recibo-general (multipart {tipo, monto_cts, consumo_total, foto_recibo}).
func (s *Server) registrarReciboGeneral(w http.ResponseWriter, r *http.Request) {
	fotos, err := archivosDeForm(r, "foto_recibo", "foto")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if len(fotos) == 0 {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "FOTO_OBLIGATORIA", "Sube la foto o el PDF del recibo general.").Campo("foto_recibo", "Obligatorio."))
		return
	}
	monto, _ := strconv.ParseInt(campo(r, "monto_cts"), 10, 64)
	consumo, errC := P.Milesimas(campo(r, "consumo_total"))
	if monto <= 0 || errC != nil || consumo <= 0 {
		P.Fallo(w, r, P.Validacion("Revisa el monto y el consumo del recibo general.").Campo("monto_cts", "Mayor que cero.").Campo("consumo_total", "m³ mayor que cero."))
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
	uid := ses(r).UsuarioID
	fid, err := s.guardarArchivo(ctx, tx, e.ID, &uid, fotos[0])
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	f, err := db.Fila(ctx, tx, `INSERT INTO recibo_general (periodo_id, tipo, monto_cts, consumo_total, foto_id, registrado_por) VALUES ($1,$2,$3,$4::numeric,$5,$6)
		RETURNING id, tipo, monto_cts, consumo_total::text AS consumo_total`, pid, tipoMedidor(r), monto, P.TextoMilesimas(consumo), fid, uid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	f["foto_url"] = s.Firma.URL(fid)
	P.JSON(w, http.StatusCreated, f)
}

func (s *Server) verReciboGeneral(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	f, err := db.Fila(r.Context(), s.DB, `SELECT rg.id, rg.tipo, rg.monto_cts, rg.consumo_total::text AS consumo_total, rg.foto_id, rg.creado_en
		FROM recibo_general rg JOIN periodo p ON p.id=rg.periodo_id WHERE p.edificio_id=$1 AND p.periodo=$2 AND rg.tipo=$3`, e.ID, chi.URLParam(r, "p"), tipoMedidor(r))
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("el recibo general del periodo"))
		return
	}
	f["foto_url"] = s.Firma.URL(f["foto_id"].(int64))
	P.JSON(w, http.StatusOK, f)
}

// CalcularReparto aplica reparto.Medidores con los datos del periodo.
func (s *Server) CalcularReparto(ctx context.Context, q db.Q, eid int64, periodo, tipo string) (map[string]any, *reparto.Resultado, int, error) {
	var monto int64
	var consumoTxt string
	err := q.QueryRow(ctx, `SELECT rg.monto_cts, rg.consumo_total::text FROM recibo_general rg JOIN periodo p ON p.id=rg.periodo_id
		WHERE p.edificio_id=$1 AND p.periodo=$2 AND rg.tipo=$3`, eid, periodo, tipo).Scan(&monto, &consumoTxt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, 0, P.Err(http.StatusUnprocessableEntity, "SIN_RECIBO_GENERAL", "Registra primero el recibo general de "+P.NombrePeriodo(periodo)+".")
	} else if err != nil {
		return nil, nil, 0, err
	}
	general, _ := P.Milesimas(consumoTxt)
	filas, err := q.Query(ctx, `SELECT u.id, u.codigo, (u.participacion_pct*10000)::bigint, l.consumo::text
		FROM medidor m JOIN unidad u ON u.id=m.unidad_id
		LEFT JOIN periodo p ON p.edificio_id=m.edificio_id AND p.periodo=$2
		LEFT JOIN lectura l ON l.medidor_id=m.id AND l.periodo_id=p.id
		WHERE m.edificio_id=$1 AND m.tipo=$3 AND m.activo AND u.activo ORDER BY m.orden_ronda, u.codigo`, eid, periodo, tipo)
	if err != nil {
		return nil, nil, 0, err
	}
	var us []reparto.Unidad
	pendientes := 0
	alertas := []string{}
	for filas.Next() {
		var u reparto.Unidad
		var c *string
		if err := filas.Scan(&u.UnidadID, &u.Codigo, &u.Participacion, &c); err != nil {
			filas.Close()
			return nil, nil, 0, err
		}
		if c == nil {
			pendientes++
			alertas = append(alertas, "Dpto "+u.Codigo+": lectura pendiente")
		} else {
			u.ConsumoLitros, _ = P.Milesimas(*c)
			if u.ConsumoLitros < 0 {
				alertas = append(alertas, "Dpto "+u.Codigo+": consumo negativo, se toma como cero")
				u.ConsumoLitros = 0
			}
		}
		us = append(us, u)
	}
	filas.Close()
	res, err := reparto.Medidores(monto, general, us)
	if errors.Is(err, reparto.ErrDiferenciaNegativa) {
		return nil, nil, pendientes, P.Err(http.StatusUnprocessableEntity, "DIFERENCIA_NEGATIVA",
			fmt.Sprintf("Los departamentos suman %s y el recibo general %s: revisa las lecturas o registra un ajuste.", P.Soles(res.TotalUnidadesCts), P.Soles(monto)))
	} else if err != nil {
		return nil, nil, pendientes, P.Validacion(err.Error())
	}
	lineas := make([]map[string]any, 0, len(res.Lineas))
	for i, l := range res.Lineas {
		lineas = append(lineas, map[string]any{"unidad_id": l.UnidadID, "unidad": l.Unidad, "consumo": l.Consumo, "propio_cts": l.PropioCts,
			"comun_cts": l.ComunCts, "total_cts": l.TotalCts, "participacion_pct": float64(us[i].Participacion) / 10000})
	}
	return map[string]any{"periodo": periodo, "tipo": tipo, "tarifa_cts_x_1000": res.TarifaCtsX1000, "monto_general_cts": monto,
		"consumo_general": consumoTxt, "total_unidades_cts": res.TotalUnidadesCts, "diferencia_cts": res.DiferenciaCts,
		"lecturas_pendientes": pendientes, "lineas": lineas, "alertas": alertas}, &res, pendientes, nil
}

func (s *Server) calcularReparto(w http.ResponseWriter, r *http.Request) {
	out, _, _, err := s.CalcularReparto(r.Context(), s.DB, edf(r).ID, chi.URLParam(r, "p"), tipoMedidor(r))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, out)
}

// aprobarReparto guarda el reparto y escribe las líneas de agua en los borradores del periodo.
func (s *Server) aprobarReparto(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	periodo, tipo := chi.URLParam(r, "p"), tipoMedidor(r)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	out, res, pend, err := s.CalcularReparto(ctx, tx, e.ID, periodo, tipo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if pend > 0 {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "LECTURAS_PENDIENTES", fmt.Sprintf("Faltan %d lecturas del periodo.", pend)).Con("lecturas_pendientes", pend))
		return
	}
	pid, _ := periodoID(ctx, tx, e.ID, periodo)
	var emitidos int
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM recibo WHERE periodo_id=$1 AND origen='periodo' AND estado NOT IN ('borrador','anulado')`, pid).Scan(&emitidos)
	if emitidos > 0 {
		P.Fallo(w, r, P.Conflicto("YA_EMITIDO", "Los recibos del periodo ya se emitieron: el reparto no se puede cambiar."))
		return
	}
	lineas, _ := json.Marshal(res.Lineas)
	uid := ses(r).UsuarioID
	if _, err := tx.Exec(ctx, `INSERT INTO reparto_medidor (periodo_id, tipo, tarifa_cts_x_1000, total_unidades_cts, diferencia_cts, lineas, aprobado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (periodo_id, tipo) DO UPDATE SET tarifa_cts_x_1000=EXCLUDED.tarifa_cts_x_1000,
		total_unidades_cts=EXCLUDED.total_unidades_cts, diferencia_cts=EXCLUDED.diferencia_cts, lineas=EXCLUDED.lineas, aprobado_por=EXCLUDED.aprobado_por, aprobado_en=now()`,
		pid, tipo, res.TarifaCtsX1000, res.TotalUnidadesCts, res.DiferenciaCts, lineas, uid); err != nil {
		P.Fallo(w, r, err)
		return
	}
	actualizados := 0
	if tipo == "agua" {
		for _, l := range res.Lineas {
			var rid int64
			if err := tx.QueryRow(ctx, `SELECT id FROM recibo WHERE periodo_id=$1 AND unidad_id=$2 AND estado='borrador'`, pid, l.UnidadID).Scan(&rid); err != nil {
				continue
			}
			if _, err := tx.Exec(ctx, `DELETE FROM recibo_linea WHERE recibo_id=$1 AND tipo IN ('agua','agua_comun')`, rid); err != nil {
				P.Fallo(w, r, err)
				return
			}
			if _, err := tx.Exec(ctx, `INSERT INTO recibo_linea (recibo_id, tipo, descripcion, monto_cts, orden) VALUES ($1,'agua',$2,$3,2), ($1,'agua_comun','Áreas comunes (agua)',$4,3)`,
				rid, "Agua (consumo propio "+strings.Replace(l.Consumo, ".", ",", 1)+" m³)", l.PropioCts, l.ComunCts); err != nil {
				P.Fallo(w, r, err)
				return
			}
			if _, err := tx.Exec(ctx, `UPDATE recibo SET total_cts=(SELECT COALESCE(sum(monto_cts),0) FROM recibo_linea WHERE recibo_id=$1) WHERE id=$1`, rid); err != nil {
				P.Fallo(w, r, err)
				return
			}
			actualizados++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	out["aprobado"] = true
	out["recibos_actualizados"] = actualizados
	P.JSON(w, http.StatusOK, out)
}
