package app

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/xuri/excelize/v2"

	P "edisys/api/internal/plataforma"
)

// Importar presupuesto por Excel (bloque A5). Hoja con dos columnas: «rubro» y «monto».

var columnasPresupuesto = []string{"rubro", "monto"}

// plantillaPresupuesto: GET /periodos/{p}/presupuesto/plantilla.xlsx
func (s *Server) plantillaPresupuesto(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	p, err := s.periodoDe(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	f := excelize.NewFile()
	defer f.Close()
	_ = f.SetSheetName("Sheet1", "Presupuesto")
	for i, c := range columnasPresupuesto {
		celda, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue("Presupuesto", celda, c)
	}
	rubros, err := s.listarRubrosDe(r, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for i, ru := range rubros {
		celda, _ := excelize.CoordinatesToCellName(1, i+2)
		_ = f.SetCellValue("Presupuesto", celda, ru)
	}
	_ = f.SetColWidth("Presupuesto", "A", "B", 28)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		P.Fallo(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="presupuesto-`+p+`.xlsx"`)
	_, _ = w.Write(buf.Bytes())
}

// importarPresupuesto: POST /periodos/{p}/presupuesto/importar (multipart «archivo»)
func (s *Server) importarPresupuesto(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	p, perr := s.periodoDe(r)
	if perr != nil {
		P.Fallo(w, r, perr)
		return
	}
	if !P.PeriodoValido(p) {
		P.Fallo(w, r, P.Validacion("El periodo debe ser AAAA-MM.").Campo("periodo", "Formato AAAA-MM."))
		return
	}
	var periodoID int64
	if err := s.DB.QueryRow(ctx, `SELECT id FROM periodo WHERE edificio_id=$1 AND periodo=$2`, e.ID, p).Scan(&periodoID); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el periodo "+p))
		return
	}
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		P.Fallo(w, r, P.Validacion("Adjunta el archivo Excel.").Campo("archivo", "Obligatorio."))
		return
	}
	file, _, err := r.FormFile("archivo")
	if err != nil {
		P.Fallo(w, r, P.Validacion("Adjunta el archivo Excel.").Campo("archivo", "Obligatorio."))
		return
	}
	defer file.Close()
	f, err := excelize.OpenReader(file)
	if err != nil {
		P.Fallo(w, r, P.Validacion("El archivo no es un Excel válido.").Campo("archivo", "Revisa el archivo."))
		return
	}
	defer f.Close()
	hoja := f.GetSheetName(0)
	filas, err := f.GetRows(hoja)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	mapa, err := s.mapaRubros(r, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var actualizados int
	var total int64
	for i, fila := range filas {
		if i == 0 { // cabecera
			continue
		}
		if len(fila) < 2 {
			continue
		}
		nombre := strings.TrimSpace(fila[0])
		if nombre == "" {
			continue
		}
		rid, ok := mapa[slugify(nombre)]
		if !ok {
			rid, ok = mapa[strings.ToLower(nombre)]
		}
		if !ok {
			continue
		}
		cts, err := P.Milesimas(strings.TrimSpace(fila[1]))
		if err != nil || cts < 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO presupuesto (periodo_id, rubro_id, monto_cts) VALUES ($1,$2,$3)
			ON CONFLICT (periodo_id, rubro_id) DO UPDATE SET monto_cts=EXCLUDED.monto_cts`, periodoID, rid, cts); err != nil {
			P.Fallo(w, r, err)
			return
		}
		actualizados++
		total += cts
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"periodo": p, "actualizados": actualizados, "total_cts": total})
}

func (s *Server) listarRubrosDe(r *http.Request, eid int64) ([]string, error) {
	filas, err := s.DB.Query(r.Context(), `SELECT nombre FROM rubro WHERE edificio_id=$1 ORDER BY orden, id`, eid)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []string
	for filas.Next() {
		var n string
		if err := filas.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, filas.Err()
}

func (s *Server) mapaRubros(r *http.Request, eid int64) (map[string]int64, error) {
	filas, err := s.DB.Query(r.Context(), `SELECT id, slug, nombre FROM rubro WHERE edificio_id=$1`, eid)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	m := map[string]int64{}
	for filas.Next() {
		var id int64
		var slug, nombre string
		if err := filas.Scan(&id, &slug, &nombre); err != nil {
			return nil, err
		}
		m[slug] = id
		m[strings.ToLower(nombre)] = id
	}
	return m, filas.Err()
}
