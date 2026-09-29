package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"edisys/api/internal/db"
	"edisys/api/internal/pdf"
	P "edisys/api/internal/plataforma"
)

// PDF del balance del periodo (04) y del informe a la junta. Salen de ArbolBalance, la misma función
// del balance, el dashboard y el portal: las cifras del PDF son las del API.

var tinta = [3]float64{0.06, 0.09, 0.16}

func periodoPDF(r *http.Request) (string, error) {
	p := strings.TrimSuffix(chi.URLParam(r, "periodo"), ".pdf")
	if !P.PeriodoValido(p) {
		return "", P.Validacion("El periodo debe tener el formato AAAA-MM.").Campo("periodo", "Formato AAAA-MM.")
	}
	return p, nil
}

// pdfBalance: GET /balance/{periodo}.pdf
func (s *Server) pdfBalance(w http.ResponseWriter, r *http.Request) { s.servirInforme(w, r, false) }

// pdfInformeJunta: GET /balance/{periodo}/informe-junta.pdf
func (s *Server) pdfInformeJunta(w http.ResponseWriter, r *http.Request) { s.servirInforme(w, r, true) }

func (s *Server) servirInforme(w http.ResponseWriter, r *http.Request, junta bool) {
	periodo, err := periodoPDF(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	datos, err := s.BalancePDF(r.Context(), e.ID, periodo, vistaDe(e), junta)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	nombre := "balance-" + periodo + ".pdf"
	if junta {
		nombre = "informe-junta-" + periodo + ".pdf"
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+nombre+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(datos)
}

// vistaCompleta: la del administrador (para los PDF que salen por correo). El PDF no lleva nombres de
// propietarios: los ingresos van por rubro y la morosidad por número de unidad.
func vistaCompleta() vista { return vista{verDocs: true, propias: map[int64]bool{}} }

// BalancePDF arma el PDF: portada con los 4 indicadores, árbol de ingresos y egresos, morosidad por unidad
// y trabajos del mes. Con junta=true agrega los pendientes por criticidad y las aprobaciones del mes.
func (s *Server) BalancePDF(ctx context.Context, eid int64, periodo string, v vista, junta bool) ([]byte, error) {
	a, err := s.ArbolBalance(ctx, s.DB, eid, periodo, v)
	if err != nil {
		return nil, err
	}
	var nombre, direccion string
	_ = s.DB.QueryRow(ctx, `SELECT nombre, direccion || CASE WHEN distrito <> '' THEN ', ' || distrito ELSE '' END FROM edificio WHERE id=$1`, eid).Scan(&nombre, &direccion)
	titulo := "Balance de " + P.NombrePeriodo(periodo)
	if junta {
		titulo = "Informe a la junta · " + P.NombrePeriodo(periodo)
	}
	h := pdf.NuevaHoja("EDISYS · " + nombre + " · " + titulo + " · generado el " + time.Now().In(P.Lima).Format("02/01/2006 15:04"))
	d := h.D
	d.Rect(0, 770, 595, 72, 0.082, 0.369, 0.459)
	d.Color(1, 1, 1)
	d.Texto(40, 805, 22, true, "EDISYS")
	d.Texto(40, 785, 11, false, nombre+" · "+direccion)
	d.TextoDerecha(555, 805, 13, true, titulo)
	d.TextoDerecha(555, 785, 10, false, "Cifras en soles · ingresos = lo cobrado")
	d.Color(tinta[0], tinta[1], tinta[2])
	h.Y = 740

	k := a.KPIs
	cajas := []struct{ t, v, n string }{
		{"Ingresos cobrados", P.Soles(k.IngresosCts), "Emitido: " + P.Soles(k.EmitidoCts)},
		{"Egresos", P.Soles(k.EgresosCts), "Banco: " + P.Soles(k.BancoCts)},
		{"Saldo del mes", P.Soles(k.SaldoCts), "Ingresos - egresos"},
		{"Morosidad del mes", pctES(k.Morosidad.Pct), "Histórica: " + pctES(k.Morosidad.HistoricaPct)},
	}
	for i, c := range cajas {
		x := 40 + float64(i)*130
		d.Rect(x, h.Y-62, 122, 66, 0.945, 0.961, 0.976)
		d.Color(0.39, 0.45, 0.55)
		d.Texto(x+8, h.Y-12, 8, false, c.t)
		d.Color(tinta[0], tinta[1], tinta[2])
		d.Texto(x+8, h.Y-34, 13, true, c.v)
		d.Color(0.39, 0.45, 0.55)
		d.Texto(x+8, h.Y-52, 7.5, false, c.n)
		d.Color(tinta[0], tinta[1], tinta[2])
	}
	h.Y -= 84
	h.Parrafo(9, fmt.Sprintf("Morosidad del mes: %s de %s emitidos (%d unidades). Morosidad histórica: %s vencidos en %d unidades, de ellos %s de deuda anterior a EDISYS.",
		P.Soles(k.Morosidad.MontoCts), P.Soles(k.EmitidoCts), k.Morosidad.Unidades, P.Soles(k.Morosidad.HistoricaMontoCts), k.Morosidad.HistoricaUnidades, P.Soles(k.Morosidad.DeudaInicialCts)))
	if c := s.textoConciliacion(ctx, eid, periodo); c != "" {
		h.Parrafo(9, c)
	}

	ing, egr := a.Raiz.Hijos[0], a.Raiz.Hijos[1]
	h.Titulo("Ingresos (cobrado)")
	for _, rb := range ing.Hijos {
		h.Fila(0, 10, true, rb.Nombre, P.Soles(rb.TotalCts))
	}
	if len(ing.Hijos) == 0 {
		h.Fila(0, 10, false, "Sin cobros en el periodo.", "")
	}
	h.Fila(0, 11, true, "Total ingresos", P.Soles(ing.TotalCts))
	h.Titulo("Egresos por rubro y concepto")
	for _, rb := range egr.Hijos {
		h.Fila(0, 10, true, rb.Nombre, P.Soles(rb.TotalCts))
		for _, c := range rb.Hijos {
			if c.Tipo == "documento" {
				h.Fila(18, 8.5, false, docTexto(c), P.Soles(c.TotalCts))
				continue
			}
			h.Fila(18, 9.5, false, c.Nombre, P.Soles(c.TotalCts))
			for _, doc := range c.Hijos {
				h.Fila(36, 8, false, docTexto(doc), P.Soles(doc.TotalCts))
			}
		}
	}
	h.Fila(0, 11, true, "Total egresos", P.Soles(egr.TotalCts))
	h.Espacio(4)
	h.Fila(0, 12, true, "Saldo del mes", P.Soles(a.Raiz.TotalCts))
	if egr.SinSustentoN > 0 {
		h.Parrafo(8.5, fmt.Sprintf("%d egreso(s) sin sustento adjunto: marcados como «sin sustento».", egr.SinSustentoN))
	}

	h.Titulo("Morosidad por unidad")
	deudas, err := s.deudaPorUnidad(ctx, eid, nil)
	if err != nil {
		return nil, err
	}
	xs := []float64{40, 250, 340, 430, 470}
	h.Columnas(9, true, xs, []string{"Unidad", ">Deuda", ">Vencida", ">Días", "Cargos"})
	for _, u := range deudas {
		cargos := 0
		if ms, ok := u["meses"].([]any); ok {
			cargos = len(ms)
		}
		h.Columnas(9, false, xs, []string{"Dpto " + fmt.Sprint(u["unidad"]), ">" + P.Soles(u["deuda_cts"].(int64)), ">" + P.Soles(u["deuda_vencida_cts"].(int64)),
			">" + fmt.Sprint(u["antiguedad_dias"]), fmt.Sprint(cargos)})
	}
	if len(deudas) == 0 {
		h.Fila(0, 9.5, false, "Todas las unidades están al día.", "")
	}

	h.Titulo("Trabajos del mes")
	trabajos, err := db.Filas(ctx, s.DB, `SELECT i.codigo, i.titulo, i.estado, COALESCE(i.criticidad,'') AS criticidad, COALESCE(i.costo_real_cts, i.monto_presupuesto_cts, 0) AS monto_cts
		FROM incidencia i WHERE i.edificio_id=$1 AND (i.estado NOT IN ('terminado','descartado','rechazado')
		   OR to_char(i.actualizado_en AT TIME ZONE 'America/Lima','YYYY-MM') = $2)
		ORDER BY CASE i.criticidad WHEN 'critica' THEN 0 WHEN 'media' THEN 1 ELSE 2 END, i.numero DESC LIMIT 40`, eid, periodo)
	if err != nil {
		return nil, err
	}
	xt := []float64{40, 95, 370, 460, 555}
	h.Columnas(9, true, xt, []string{"Código", "Trabajo", "Estado", "Criticidad", ">Monto"})
	for _, t := range trabajos {
		h.Columnas(8.5, false, xt, []string{t["codigo"].(string), recortar(t["titulo"].(string), 52), estadoTrabajo(t["estado"].(string)), t["criticidad"].(string), ">" + P.Soles(t["monto_cts"].(int64))})
	}
	if len(trabajos) == 0 {
		h.Fila(0, 9.5, false, "Sin trabajos en el mes.", "")
	}

	if junta {
		if err := s.seccionesJunta(ctx, h, eid, periodo); err != nil {
			return nil, err
		}
	}
	return h.Bytes(), nil
}

func (s *Server) seccionesJunta(ctx context.Context, h *pdf.Hoja, eid int64, periodo string) error {
	h.Titulo("Pendientes por criticidad")
	pend, err := db.Filas(ctx, s.DB, `SELECT i.codigo, i.titulo, i.estado, COALESCE(i.criticidad,'sin_clasificar') AS criticidad, COALESCE(i.monto_presupuesto_cts,0) AS monto_cts,
			(SELECT count(*) FROM voto v WHERE v.incidencia_id=i.id AND v.voto='aprueba') AS a_favor
		FROM incidencia i WHERE i.edificio_id=$1 AND i.estado NOT IN ('terminado','descartado','rechazado')
		ORDER BY CASE i.criticidad WHEN 'critica' THEN 0 WHEN 'media' THEN 1 WHEN 'baja' THEN 2 ELSE 3 END, i.numero`, eid)
	if err != nil {
		return err
	}
	xs := []float64{58, 115, 380, 555}
	actual := ""
	nombres := map[string]string{"critica": "Crítica", "media": "Media", "baja": "Baja", "sin_clasificar": "Sin clasificar"}
	for _, p := range pend {
		c := p["criticidad"].(string)
		if c != actual {
			actual = c
			h.Espacio(2)
			h.Fila(0, 10, true, nombres[c], "")
		}
		estado := estadoTrabajo(p["estado"].(string))
		if p["estado"] == "presupuestado" {
			estado += fmt.Sprintf(" (%d a favor)", p["a_favor"].(int64))
		}
		h.Columnas(8.5, false, xs, []string{p["codigo"].(string), recortar(p["titulo"].(string), 50), estado, ">" + P.Soles(p["monto_cts"].(int64))})
	}
	if len(pend) == 0 {
		h.Fila(0, 9.5, false, "No hay pendientes.", "")
	}
	h.Titulo("Aprobaciones del mes")
	aps, err := db.Filas(ctx, s.DB, `SELECT i.codigo, i.titulo, ev.estado_hasta AS decision, ev.nota, to_char(ev.creado_en AT TIME ZONE 'America/Lima','DD/MM/YYYY') AS fecha,
			COALESCE(i.monto_presupuesto_cts,0) AS monto_cts
		FROM incidencia_evento ev JOIN incidencia i ON i.id=ev.incidencia_id
		WHERE i.edificio_id=$1 AND ev.estado_hasta IN ('aprobado','rechazado') AND to_char(ev.creado_en AT TIME ZONE 'America/Lima','YYYY-MM')=$2
		ORDER BY ev.creado_en`, eid, periodo)
	if err != nil {
		return err
	}
	xa := []float64{40, 95, 330, 400, 555}
	for _, a := range aps {
		h.Columnas(8.5, false, xa, []string{a["codigo"].(string), recortar(a["titulo"].(string), 44), a["decision"].(string), a["fecha"].(string), ">" + P.Soles(a["monto_cts"].(int64))})
		if n, _ := a["nota"].(string); n != "" {
			h.Fila(55, 8, false, n, "")
		}
	}
	if len(aps) == 0 {
		h.Fila(0, 9.5, false, "No hubo aprobaciones ni rechazos en el mes.", "")
	}
	return nil
}

func docTexto(n *Nodo) string {
	t := n.Nombre
	if len(n.Fecha) == 10 {
		t = n.Fecha[8:10] + "/" + n.Fecha[5:7] + " · " + t
	}
	if n.SinSustento {
		t += " (sin sustento)"
	}
	return t
}

func pctES(v float64) string { return strings.Replace(fmt.Sprintf("%.1f %%", v), ".", ",", 1) }

func estadoTrabajo(e string) string {
	if e == "en_ejecucion" {
		return "en ejecución"
	}
	return strings.ReplaceAll(e, "_", " ")
}

// textoConciliacion: estado de la conciliación bancaria del periodo (bloque 4). "" si no hay extracto.
func (s *Server) textoConciliacion(ctx context.Context, eid int64, periodo string) string {
	return ""
}
