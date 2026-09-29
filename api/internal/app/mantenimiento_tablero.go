package app

// Tablero configurable + exportar + plan de trabajo (09).
// El grafo de transiciones NO se toca (regla dura en 0006 + estados.go):
// aquí se configura qué etapas se ven y en qué orden, qué campos trae la
// tarjeta, se exporta lo filtrado y se arma el plan con hitos e informe.

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"edisys/api/internal/db"
	M "edisys/api/internal/mantenimiento"
	P "edisys/api/internal/plataforma"
)

// condIncidencias: el WHERE de listarIncidencias, compartido con exportar y plan.
// Mismos filtros (estado, criticidad, categoria, responsable_id, q, desde, hasta,
// mes) y mismo recorte por rol.
func condIncidencias(e *Edificio, v url.Values, uid int64) (string, []any) {
	cond := []string{"i.edificio_id=$1"}
	args := []any{e.ID}
	add := func(c string, a any) {
		args = append(args, a)
		cond = append(cond, strings.ReplaceAll(c, "?", "$"+strconv.Itoa(len(args))))
	}
	lista := func(s string) []string {
		var out []string
		for _, x := range strings.Split(s, ",") {
			if x = strings.TrimSpace(x); x != "" {
				out = append(out, x)
			}
		}
		return out
	}
	if l := lista(v.Get("estado")); len(l) > 0 {
		add("i.estado = ANY(?)", l)
	}
	if l := lista(v.Get("criticidad")); len(l) > 0 {
		add("i.criticidad = ANY(?)", l)
	}
	if l := lista(v.Get("categoria")); len(l) > 0 {
		add("i.categoria = ANY(?)", l)
	}
	if n, err := strconv.ParseInt(v.Get("responsable_id"), 10, 64); err == nil {
		add("i.responsable_id = ?", n)
	}
	if t := strings.TrimSpace(v.Get("q")); t != "" {
		add("(i.codigo ILIKE ? OR i.titulo ILIKE ? OR i.descripcion ILIKE ? OR i.ubicacion ILIKE ?)", "%"+t+"%")
	}
	if t, err := parseFecha(v.Get("desde")); err == nil {
		add("i.creado_en >= ?", t)
	}
	if t, err := parseFecha(v.Get("hasta")); err == nil {
		add("i.creado_en < ?", t.AddDate(0, 0, 1))
	}
	if m := v.Get("mes"); P.PeriodoValido(m) {
		ini, fin := P.RangoPeriodo(m)
		add("(i.creado_en < ? ", fin)
		cond[len(cond)-1] += fmt.Sprintf("AND (i.terminado_en IS NULL OR i.terminado_en >= $%d))", len(args)+1)
		args = append(args, ini)
	}
	switch {
	case e.Rol == "tecnico":
		add("i.responsable_id = ?", uid)
	case e.SoloLoSuyo():
		if v.Get("todos") == "1" {
			cond = append(cond, "i.estado IN ('presupuestado','aprobado','en_ejecucion','terminado','rechazado')")
		} else {
			add("(i.reportado_por = ? OR i.unidad_id = ANY(", uid)
			args = append(args, e.Unidades)
			cond[len(cond)-1] += fmt.Sprintf("$%d))", len(args))
		}
	}
	return strings.Join(cond, " AND "), args
}

// ---------- configuración del tablero ----------

// Columnas y campos válidos para la configuración.
var columnasValidas = map[string]bool{}
var camposTarjetaValidos = map[string]bool{
	"monto": true, "responsable": true, "fotos": true, "votos": true, "antiguedad": true,
}

func init() {
	for _, e := range M.Estados {
		columnasValidas[e] = true
	}
}

type configTablero struct {
	Columnas []string        `json:"columnas"`
	Tarjeta  map[string]bool `json:"tarjeta"`
	Defecto  bool            `json:"por_defecto"`
}

func leerConfigTablero(ctx context.Context, q db.Q, eid int64) configTablero {
	def := configTablero{Columnas: append([]string{}, M.Estados...),
		Tarjeta: map[string]bool{"monto": true, "responsable": true, "fotos": true, "votos": true, "antiguedad": true},
		Defecto: true}
	var colB, tarB []byte
	if err := q.QueryRow(ctx, `SELECT columnas, tarjeta_campos FROM mantenimiento_config WHERE edificio_id=$1`, eid).Scan(&colB, &tarB); err != nil {
		return def
	}
	var col []string
	if json.Unmarshal(colB, &col) == nil {
		vistas := []string{}
		visto := map[string]bool{}
		for _, c := range col {
			if columnasValidas[c] && !visto[c] {
				visto[c] = true
				vistas = append(vistas, c)
			}
		}
		if len(vistas) > 0 {
			def.Columnas = vistas
			def.Defecto = false
		}
	}
	var tar map[string]bool
	if json.Unmarshal(tarB, &tar) == nil {
		for k := range def.Tarjeta {
			if v, ok := tar[k]; ok {
				def.Tarjeta[k] = v
				def.Defecto = false
			}
		}
	}
	return def
}

// verConfigTablero: GET /mantenimiento/tablero/config.
func (s *Server) verConfigTablero(w http.ResponseWriter, r *http.Request) {
	P.JSON(w, http.StatusOK, leerConfigTablero(r.Context(), s.DB, edf(r).ID))
}

// guardarConfigTablero: PUT /mantenimiento/tablero/config {columnas: [...], tarjeta: {...}}.
func (s *Server) guardarConfigTablero(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Columnas []string        `json:"columnas"`
		Tarjeta  map[string]bool `json:"tarjeta"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if len(in.Columnas) == 0 {
		P.Fallo(w, r, P.Validacion("Elige al menos una etapa.").Campo("columnas", "Obligatorio."))
		return
	}
	visto := map[string]bool{}
	for _, c := range in.Columnas {
		if !columnasValidas[c] || visto[c] {
			P.Fallo(w, r, P.Validacion("Etapa inválida o repetida: "+c).Campo("columnas", "Revisa las etapas."))
			return
		}
		visto[c] = true
	}
	tar := map[string]bool{"monto": true, "responsable": true, "fotos": true, "votos": true, "antiguedad": true}
	for k, v := range in.Tarjeta {
		if !camposTarjetaValidos[k] {
			P.Fallo(w, r, P.Validacion("Campo inválido: "+k).Campo("tarjeta", "Revisa los campos."))
			return
		}
		tar[k] = v
	}
	colB, _ := json.Marshal(in.Columnas)
	tarB, _ := json.Marshal(tar)
	e := edf(r)
	ctx := r.Context()
	if _, err := s.DB.Exec(ctx, `INSERT INTO mantenimiento_config (edificio_id, columnas, tarjeta_campos, actualizado_en)
		VALUES ($1,$2,$3,now()) ON CONFLICT (edificio_id)
		DO UPDATE SET columnas=EXCLUDED.columnas, tarjeta_campos=EXCLUDED.tarjeta_campos, actualizado_en=now()`,
		e.ID, colB, tarB); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "mantenimiento", "tablero_config", "mantenimiento_config", e.ID, nil,
		map[string]any{"columnas": in.Columnas, "tarjeta": tar})
	P.JSON(w, http.StatusOK, leerConfigTablero(ctx, s.DB, e.ID))
}

// exportarIncidencias: GET /mantenimiento/incidencias/exportar?{filtros} → CSV con BOM.
func (s *Server) exportarIncidencias(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	where, args := condIncidencias(e, r.URL.Query(), ses(r).UsuarioID)
	filas, err := db.Filas(ctx, s.DB, `SELECT i.codigo, i.titulo, i.estado, COALESCE(i.criticidad,'') AS criticidad,
			i.categoria, COALESCE(u.codigo,'') AS unidad, COALESCE(rs.nombre,'') AS responsable,
			COALESCE(i.monto_presupuesto_cts,0) AS monto_cts, COALESCE(i.costo_real_cts,0) AS costo_cts,
			to_char(i.creado_en,'YYYY-MM-DD') AS creado, COALESCE(to_char(i.terminado_en,'YYYY-MM-DD'),'') AS terminado
		FROM incidencia i LEFT JOIN unidad u ON u.id=i.unidad_id LEFT JOIN usuario rs ON rs.id=i.responsable_id
		WHERE `+where+` ORDER BY i.numero DESC LIMIT 2000`, args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var b strings.Builder
	b.WriteString("\ufeff")
	csvw := csv.NewWriter(&b)
	_ = csvw.Write([]string{"codigo", "titulo", "estado", "criticidad", "categoria", "unidad", "responsable", "monto_cts", "costo_real_cts", "reportado", "terminado"})
	for _, f := range filas {
		_ = csvw.Write([]string{
			fmt.Sprint(f["codigo"]), fmt.Sprint(f["titulo"]), fmt.Sprint(f["estado"]), fmt.Sprint(f["criticidad"]),
			fmt.Sprint(f["categoria"]), fmt.Sprint(f["unidad"]), fmt.Sprint(f["responsable"]),
			fmt.Sprint(f["monto_cts"]), fmt.Sprint(f["costo_cts"]), fmt.Sprint(f["creado"]), fmt.Sprint(f["terminado"]),
		})
	}
	csvw.Flush()
	nombre := "tablero-" + time.Now().In(P.Lima).Format("20060102-1504") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+nombre+`"`)
	_, _ = w.Write([]byte(b.String()))
}

// planTrabajos: GET /mantenimiento/plan → trabajos por hacer con hitos e informe.
// Hitos: reportado → validado → informe y costos → aprobado → en ejecución → terminado.
// tiene_informe = costos publicados (monto registrado, lo que la junta ve para aprobar).
func (s *Server) planTrabajos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	where, args := condIncidencias(e, r.URL.Query(), ses(r).UsuarioID)
	where += ` AND i.estado NOT IN ('terminado','descartado')`
	filas, err := db.Filas(ctx, s.DB, `SELECT i.id, i.codigo, i.titulo, i.estado, COALESCE(i.criticidad,'') AS criticidad,
			COALESCE(rs.nombre,'') AS responsable, COALESCE(i.monto_presupuesto_cts,0) AS monto_cts,
			COALESCE(i.costo_real_cts,0) AS costo_cts,
			(i.monto_presupuesto_cts IS NOT NULL) AS tiene_informe,
			(SELECT count(*) FROM incidencia_evidencia ev WHERE ev.incidencia_id=i.id AND ev.tipo='avance') AS avances,
			to_char(i.actualizado_en,'YYYY-MM-DD') AS actualizado
		FROM incidencia i LEFT JOIN usuario rs ON rs.id=i.responsable_id
		WHERE `+where+` ORDER BY CASE i.criticidad WHEN 'critica' THEN 0 WHEN 'media' THEN 1 WHEN 'baja' THEN 2 ELSE 3 END, i.numero DESC LIMIT 500`, args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas)})
}
