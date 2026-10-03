package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"edisys/api/internal/db"
	"edisys/api/internal/pdf"
	P "edisys/api/internal/plataforma"
)

// Plantillas de recibo (bloque I1): cada edificio elige color, título, nota al pie y qué bloques
// opcionales lleva el PDF (contómetro, foto del medidor, QR y código de barras). Se guarda en
// plantilla_recibo.config_json; lo que falta toma el valor por defecto, así que una fila vieja nunca rompe el PDF.

// BloquesRecibo: partes opcionales del recibo.
type BloquesRecibo struct {
	Contometro bool `json:"contometro"` // lecturas anterior/actual y consumo de agua
	Fotos      bool `json:"fotos"`      // foto del medidor del periodo
	QR         bool `json:"qr"`         // enlace al recibo en la app
	Barras     bool `json:"barras"`     // Code 128 con el número del recibo
}

// PlantillaRecibo es config_json.
type PlantillaRecibo struct {
	Color       string        `json:"color"` // #RRGGBB; vacío = el color de la marca
	Titulo      string        `json:"titulo"`
	Nota        string        `json:"nota"`
	MostrarLogo bool          `json:"mostrar_logo"`
	Bloques     BloquesRecibo `json:"bloques"`
}

// PlantillaPorDefecto reproduce el recibo de siempre: sin bloques opcionales y con el logo si hay.
func PlantillaPorDefecto() PlantillaRecibo {
	return PlantillaRecibo{Titulo: "Recibo de mantenimiento", MostrarLogo: true}
}

// LeerPlantilla parte de los valores por defecto y encima pone lo guardado.
func LeerPlantilla(crudo []byte) PlantillaRecibo {
	p := PlantillaPorDefecto()
	if len(crudo) > 0 {
		_ = json.Unmarshal(crudo, &p)
	}
	if strings.TrimSpace(p.Titulo) == "" {
		p.Titulo = PlantillaPorDefecto().Titulo
	}
	return p
}

// ValidarPlantilla: el título y la nota caben en la hoja y el color deja leer el texto blanco de la cabecera.
func ValidarPlantilla(p PlantillaRecibo) *P.Error {
	ev := P.Validacion("Revisa la plantilla.")
	if n := len([]rune(strings.TrimSpace(p.Titulo))); n == 0 || n > 60 {
		ev.Campo("titulo", "Entre 1 y 60 caracteres.")
	}
	if len([]rune(p.Nota)) > 400 {
		ev.Campo("nota", "Máximo 400 caracteres.")
	}
	if p.Color != "" {
		if !reHex.MatchString(p.Color) {
			ev.Campo("color", "Usa un color #RRGGBB o déjalo vacío para usar el de la marca.")
		} else if c := Contraste(p.Color, "#FFFFFF"); c < ContrasteMinimo {
			ev.Campo("color", fmt.Sprintf("Muy claro: el texto blanco de la cabecera no se lee (contraste %.1f:1, mínimo 4,5:1).", c))
		}
	}
	if len(ev.Campos) > 0 {
		return ev
	}
	return nil
}

func (s *Server) plantillaDeEdificio(ctx context.Context, q db.Q, eid int64) (PlantillaRecibo, bool) {
	var crudo []byte
	if err := q.QueryRow(ctx, `SELECT config_json FROM plantilla_recibo WHERE edificio_id=$1`, eid).Scan(&crudo); err != nil {
		return PlantillaPorDefecto(), false
	}
	return LeerPlantilla(crudo), true
}

// ---------- endpoints ----------

// verPlantillaRecibo: GET /plantilla-recibo
func (s *Server) verPlantillaRecibo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e := edf(r)
	p, guardada := s.plantillaDeEdificio(ctx, s.DB, e.ID)
	m := s.MarcaDeEdificio(ctx, e.ID)
	color := p.Color
	if color == "" {
		color = m.ColorPrimario
	}
	fila, _ := db.Fila(ctx, s.DB, `SELECT pr.actualizado_en, us.nombre AS actualizado_por FROM plantilla_recibo pr
		LEFT JOIN usuario us ON us.id=pr.actualizado_por WHERE pr.edificio_id=$1`, e.ID)
	P.JSON(w, http.StatusOK, map[string]any{"config": p, "guardada": guardada, "color_efectivo": color,
		"marca":       map[string]any{"nombre": m.Nombre, "color_primario": m.ColorPrimario, "tiene_logo": m.LogoArchivoID != 0},
		"actualizado": fila})
}

// guardarPlantillaRecibo: PUT /plantilla-recibo {config}
func (s *Server) guardarPlantillaRecibo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Config PlantillaRecibo `json:"config"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	p := in.Config
	p.Color = strings.ToUpper(strings.TrimSpace(p.Color))
	p.Titulo = strings.TrimSpace(p.Titulo)
	p.Nota = strings.TrimSpace(p.Nota)
	if ev := ValidarPlantilla(p); ev != nil {
		P.Fallo(w, r, ev)
		return
	}
	ctx := r.Context()
	e := edf(r)
	antes, guardada := s.plantillaDeEdificio(ctx, s.DB, e.ID)
	crudo, _ := json.Marshal(p)
	if _, err := s.DB.Exec(ctx, `INSERT INTO plantilla_recibo (edificio_id, config_json, actualizado_por) VALUES ($1,$2,$3)
		ON CONFLICT (edificio_id) DO UPDATE SET config_json=EXCLUDED.config_json, actualizado_por=EXCLUDED.actualizado_por, actualizado_en=now()`,
		e.ID, crudo, ses(r).UsuarioID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	var previo any = antes
	if !guardada {
		previo = map[string]any{"por_defecto": true}
	}
	s.auditarCambio(ctx, s.DB, r, "recibos", "plantilla", "plantilla_recibo", e.ID, previo, p)
	P.JSON(w, http.StatusOK, map[string]any{"config": p, "guardada": true})
}

// vistaPreviaRecibo: POST /plantilla-recibo/vista-previa {config?} → PDF con el último recibo del edificio
// (o uno de muestra si aún no hay), con la plantilla enviada aunque no esté guardada.
func (s *Server) vistaPreviaRecibo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Config *PlantillaRecibo `json:"config"`
	}
	if r.ContentLength != 0 {
		if err := P.Leer(r, &in); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	ctx := r.Context()
	e := edf(r)
	var pl PlantillaRecibo
	if in.Config != nil {
		pl = *in.Config
		pl.Color = strings.ToUpper(strings.TrimSpace(pl.Color))
		if ev := ValidarPlantilla(pl); ev != nil {
			P.Fallo(w, r, ev)
			return
		}
	} else {
		pl, _ = s.plantillaDeEdificio(ctx, s.DB, e.ID)
	}
	var rid int64
	_ = s.DB.QueryRow(ctx, `SELECT id FROM recibo WHERE edificio_id=$1 AND estado <> 'borrador' ORDER BY emitido_en DESC NULLS LAST, id DESC LIMIT 1`, e.ID).Scan(&rid)
	var datos []byte
	if rid > 0 {
		rc, err := s.reciboVisible(ctx, e, rid)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		datos, err = s.reciboConPlantilla(ctx, e.ID, rc, rid, &pl)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
	} else {
		datos = s.dibujarRecibo(reciboMuestra(e.Nombre), pl, s.MarcaDeEdificio(ctx, e.ID), s.logoSiCorresponde(ctx, e.ID, pl))
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="vista-previa-recibo.pdf"`)
	_, _ = w.Write(datos)
}

// ---------- render ----------

// datosReciboPDF: lo que el dibujo necesita, ya leído de la base y del almacén.
type datosReciboPDF struct {
	id      int64
	rc      map[string]any
	lineas  []map[string]any
	medidor map[string]any // nil si no hay lectura de agua en el periodo
	foto    image.Image
	enlace  string // para el QR
}

func (s *Server) logoSiCorresponde(ctx context.Context, eid int64, pl PlantillaRecibo) image.Image {
	if !pl.MostrarLogo {
		return nil
	}
	return s.logoMarca(ctx, s.MarcaDeEdificio(ctx, eid))
}

// reciboConPlantilla arma el PDF del recibo con la plantilla dada (nil = la guardada del edificio).
func (s *Server) reciboConPlantilla(ctx context.Context, eid int64, rc map[string]any, rid int64, pl *PlantillaRecibo) ([]byte, error) {
	if pl == nil {
		p, _ := s.plantillaDeEdificio(ctx, s.DB, eid)
		pl = &p
	}
	lineas, err := db.Filas(ctx, s.DB, `SELECT descripcion, monto_cts FROM recibo_linea WHERE recibo_id=$1 ORDER BY orden, id`, rid)
	if err != nil {
		return nil, err
	}
	d := datosReciboPDF{id: rid, rc: rc, lineas: lineas, enlace: fmt.Sprintf("%s/app/recibos/?recibo=%d", s.Cfg.URLPublica, rid)}
	if pl.Bloques.Contometro || pl.Bloques.Fotos {
		med, err := db.Fila(ctx, s.DB, `SELECT m.serie, l.anterior::text AS anterior, l.valor::text AS actual, l.consumo::text AS consumo, l.foto_id
			FROM lectura l JOIN medidor m ON m.id=l.medidor_id
			WHERE m.unidad_id=$1 AND l.periodo_id=$2 AND m.tipo='agua' LIMIT 1`, rc["unidad_id"], rc["periodo_id"])
		if err == nil {
			d.medidor = med
			if fid, ok := med["foto_id"].(int64); ok && pl.Bloques.Fotos {
				d.foto = s.imagenDeArchivo(ctx, fid)
			}
		}
	}
	return s.dibujarRecibo(d, *pl, s.MarcaDeEdificio(ctx, eid), s.logoSiCorresponde(ctx, eid, *pl)), nil
}

// imagenDeArchivo lee y decodifica un archivo del almacén (nil si no es una imagen legible).
func (s *Server) imagenDeArchivo(ctx context.Context, id int64) image.Image {
	if s.Almacen == nil {
		return nil
	}
	var clave string
	if err := s.DB.QueryRow(ctx, `SELECT clave FROM archivo WHERE id=$1`, id).Scan(&clave); err != nil {
		return nil
	}
	datos, err := s.Almacen.Leer(ctx, clave)
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return nil
	}
	return img
}

// reciboMuestra: datos de ejemplo para la vista previa de un edificio que todavía no emite recibos.
func reciboMuestra(edificio string) datosReciboPDF {
	per := time.Now().In(P.Lima).Format("2006-01")
	return datosReciboPDF{
		rc: map[string]any{"numero": "R-MUESTRA", "correlativo": "0001", "edificio": edificio, "direccion": "", "unidad": "101",
			"propietario": "Propietario de ejemplo", "periodo": per, "participacion_pct": 1.25, "vence": time.Now().In(P.Lima).AddDate(0, 0, 10).Format("2006-01-02"),
			"estado": "emitido", "total_cts": int64(35000), "pagado_cts": int64(0), "saldo_cts": int64(35000), "yape_numero": ""},
		lineas:  []map[string]any{{"descripcion": "Cuota de mantenimiento", "monto_cts": int64(28000)}, {"descripcion": "Agua (consumo del periodo)", "monto_cts": int64(7000)}},
		medidor: map[string]any{"serie": "MUESTRA-01", "anterior": "1250.000", "actual": "1268.500", "consumo": "18.500"},
		enlace:  "https://edisys.pe/app/recibos/",
	}
}

func colorPDF(hex string) (float64, float64, float64) {
	c, ok := hexARGB(hex)
	if !ok {
		c, _ = hexARGB(marcaPrimario)
	}
	return c[0] / 255, c[1] / 255, c[2] / 255
}

// dibujarRecibo: cabecera con el color y el logo, conceptos, totales y los bloques que pida la plantilla.
func (s *Server) dibujarRecibo(dt datosReciboPDF, pl PlantillaRecibo, m Marca, logo image.Image) []byte {
	rc := dt.rc
	color := pl.Color
	if color == "" {
		color = m.ColorPrimario
	}
	cr, cg, cb := colorPDF(color)
	d := pdf.Nuevo()
	d.Rect(0, 770, 595, 72, cr, cg, cb)
	d.Color(1, 1, 1)
	if logo != nil {
		// Recuadro blanco para que cualquier logo se lea sobre el color de la cabecera.
		d.Rect(32, 778, 156, 56, 1, 1, 1)
		d.Imagen(38, 782, 144, 48, logo)
		d.Color(1, 1, 1)
		d.Texto(200, 785, 10, false, recortar(fmt.Sprint(rc["edificio"], " · ", rc["direccion"]), 38))
	} else {
		d.Texto(40, 805, 22, true, m.Nombre)
		d.Texto(40, 785, 11, false, fmt.Sprint(rc["edificio"], " · ", rc["direccion"]))
	}
	d.TextoDerecha(555, 805, 13, true, pl.Titulo)
	d.TextoDerecha(555, 785, 11, false, fmt.Sprint(val(rc["numero"]), "  ", val(rc["correlativo"])))
	d.Color(tinta[0], tinta[1], tinta[2])

	y := 730.0
	d.Texto(40, y, 12, true, fmt.Sprint("Dpto ", rc["unidad"], " · ", rc["propietario"]))
	periodo, _ := rc["periodo"].(string)
	pct, _ := rc["participacion_pct"].(float64)
	d.Texto(40, y-18, 10, false, fmt.Sprint("Periodo: ", P.NombrePeriodo(periodo), "   Participación: ", strings.Replace(fmt.Sprintf("%.2f", pct), ".", ",", 1), " %"))
	d.Texto(40, y-34, 10, false, fmt.Sprint("Vence: ", val(rc["vence"]), "   Estado: ", rc["estado"]))
	y -= 70
	d.Texto(40, y, 10, true, "Concepto")
	d.TextoDerecha(555, y, 10, true, "Importe")
	d.Linea(40, y-6, 555, y-6)
	y -= 24
	for _, l := range dt.lineas {
		d.Texto(40, y, 10, false, recortar(fmt.Sprint(l["descripcion"]), 80))
		cts, _ := l["monto_cts"].(int64)
		d.TextoDerecha(555, y, 10, false, P.Soles(cts))
		y -= 18
	}
	d.Linea(40, y+6, 555, y+6)
	y -= 14
	total, _ := rc["total_cts"].(int64)
	pagado, _ := rc["pagado_cts"].(int64)
	saldo, _ := rc["saldo_cts"].(int64)
	d.Texto(40, y, 13, true, "Total")
	d.TextoDerecha(555, y, 13, true, P.Soles(total))
	y -= 20
	d.Texto(40, y, 10, false, "Pagado: "+P.Soles(pagado)+"    Saldo: "+P.Soles(saldo))
	if yp, _ := rc["yape_numero"].(string); yp != "" {
		y -= 30
		d.Texto(40, y, 10, false, "Paga por Yape al "+yp+" con el concepto «Dpto "+fmt.Sprint(rc["unidad"])+" "+periodo+"» y sube tu voucher en la app.")
	}

	// Bloques opcionales, en una franja: contómetro y foto a la izquierda, QR y barras a la derecha.
	b := pl.Bloques
	if b.Contometro || b.Fotos || b.QR || b.Barras {
		y -= 36
		if y < 290 {
			d.NuevaPagina()
			y = 780
		}
		alto := 130.0
		top := y
		if b.Contometro {
			d.Texto(40, top, 10, true, "Contómetro de agua")
			if dt.medidor != nil {
				d.Texto(40, top-16, 9.5, false, "Serie: "+val(dt.medidor["serie"]))
				d.Texto(40, top-31, 9.5, false, "Lectura anterior: "+val(dt.medidor["anterior"]))
				d.Texto(40, top-46, 9.5, false, "Lectura actual: "+val(dt.medidor["actual"]))
				d.Texto(40, top-61, 9.5, true, "Consumo: "+val(dt.medidor["consumo"])+" m³")
			} else {
				d.Texto(40, top-16, 9.5, false, "Sin lectura de agua en este periodo.")
			}
		}
		if b.Fotos {
			x := 40.0
			if b.Contometro {
				x = 200
			}
			d.Texto(x, top, 10, true, "Foto del medidor")
			if dt.foto != nil {
				d.Imagen(x, top-alto+8, 150, alto-20, dt.foto)
			} else {
				d.Texto(x, top-16, 9.5, false, "Sin foto en este periodo.")
			}
		}
		if b.QR && dt.enlace != "" {
			dibujarQR(d, dt.enlace, 465, top-90, 90)
			d.Texto(448, top-102, 8, false, "Escanea para ver y pagar")
		}
		if b.Barras {
			// Debajo de la franja, alineado a la derecha: hasta 220 pt de ancho, con la barra fina de 0,6 a 1,2 pt.
			num := val(rc["numero"])
			if num == "" {
				num = fmt.Sprintf("R%d", dt.id)
			}
			modulos := 0
			for _, m := range pdf.Modulos128(num) {
				modulos += m
			}
			mod := max(0.6, min(1.2, 220/float64(modulos)))
			d.Barras128(555-float64(modulos)*mod, top-alto-36, 32, mod, num)
		}
		d.Color(tinta[0], tinta[1], tinta[2])
	}

	d.Color(0.39, 0.45, 0.55)
	if pl.Nota != "" {
		for i, l := range partirLineas(pl.Nota, 105) {
			if i == 3 {
				break
			}
			d.Texto(40, 82-float64(i)*12, 9, false, l)
		}
	}
	d.Texto(40, 40, 8, false, "Recibo interno de mantenimiento (no es comprobante SUNAT). Generado por "+m.Nombre+" el "+time.Now().In(P.Lima).Format("02/01/2006 15:04")+".")
	return d.Bytes()
}

// dibujarQR pinta el código QR de texto con su esquina inferior izquierda en (x, y).
func dibujarQR(d *pdf.Doc, texto string, x, y, lado float64) {
	q, err := qrcode.New(texto, qrcode.Medium)
	if err != nil {
		return
	}
	q.DisableBorder = true
	bm := q.Bitmap()
	modulo := lado / float64(len(bm))
	for i, fila := range bm {
		for j, negro := range fila {
			if negro {
				d.Rect(x+float64(j)*modulo, y+lado-float64(i+1)*modulo, modulo+0.05, modulo+0.05, 0, 0, 0)
			}
		}
	}
}

// partirLineas corta un texto en líneas de hasta n caracteres sin partir palabras.
func partirLineas(s string, n int) []string {
	var out []string
	linea := ""
	for _, p := range strings.Fields(s) {
		if linea != "" && len([]rune(linea))+1+len([]rune(p)) > n {
			out = append(out, linea)
			linea = p
			continue
		}
		if linea != "" {
			linea += " "
		}
		linea += p
	}
	if linea != "" {
		out = append(out, linea)
	}
	return out
}
