// Package pdf escribe PDF sencillos (A4, texto y líneas) sin dependencias ni Chrome:
// alcanza para el recibo por departamento y los documentos de la semilla.
package pdf

import (
	"bytes"
	"fmt"
	"strings"
)

// Doc es un documento de una o más páginas A4 (595 × 842 pt).
type Doc struct {
	paginas  []*bytes.Buffer
	actual   *bytes.Buffer
	imagenes []imagenPDF // marca: I1 · logos y fotos incrustados (imagen.go)
}

// Nuevo crea un documento con una página.
func Nuevo() *Doc {
	d := &Doc{}
	d.NuevaPagina()
	return d
}

// NuevaPagina agrega una página.
func (d *Doc) NuevaPagina() {
	d.actual = &bytes.Buffer{}
	d.paginas = append(d.paginas, d.actual)
}

// Texto escribe en (x, y) desde abajo-izquierda. negrita usa Helvetica-Bold.
func (d *Doc) Texto(x, y float64, tam float64, negrita bool, s string) {
	fuente := "F1"
	if negrita {
		fuente = "F2"
	}
	fmt.Fprintf(d.actual, "BT /%s %.1f Tf %.1f %.1f Td (%s) Tj ET\n", fuente, tam, x, y, escapar(s))
}

// TextoDerecha alinea a la derecha en x (aprox. con el ancho medio de Helvetica).
func (d *Doc) TextoDerecha(x, y, tam float64, negrita bool, s string) {
	ancho := float64(len([]rune(s))) * tam * 0.52
	d.Texto(x-ancho, y, tam, negrita, s)
}

// Linea traza una línea gris.
func (d *Doc) Linea(x1, y1, x2, y2 float64) {
	fmt.Fprintf(d.actual, "0.8 0.84 0.88 RG 0.8 w %.1f %.1f m %.1f %.1f l S\n", x1, y1, x2, y2)
}

// Rect rellena un rectángulo con un color RGB (0–1).
func (d *Doc) Rect(x, y, w, h, r, g, b float64) {
	fmt.Fprintf(d.actual, "%.3f %.3f %.3f rg %.1f %.1f %.1f %.1f re f 0 0 0 rg\n", r, g, b, x, y, w, h)
}

// Color cambia el color del texto.
func (d *Doc) Color(r, g, b float64) { fmt.Fprintf(d.actual, "%.3f %.3f %.3f rg\n", r, g, b) }

// Bytes arma el archivo PDF.
func (d *Doc) Bytes() []byte {
	var out bytes.Buffer
	var offs []int
	obj := func(s string) {
		offs = append(offs, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offs), s)
	}
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	n := len(d.paginas)
	// 1 catálogo, 2 páginas, 3 F1, 4 F2, luego por página: página + contenido.
	kids := make([]string, n)
	for i := range d.paginas {
		kids[i] = fmt.Sprintf("%d 0 R", 5+2*i)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	xobj := d.recursosImagen(5 + 2*n) // marca: I1 · las imágenes van después de las páginas
	for i, p := range d.paginas {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 3 0 R /F2 4 0 R >>%s >> /Contents %d 0 R >>", xobj, 6+2*i))
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", p.Len(), p.String()))
	}
	for _, im := range d.imagenes { // marca: I1
		obj(im.objeto())
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offs)+1)
	for _, o := range offs {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offs)+1, xref)
	return out.Bytes()
}

// escapar pasa a WinAnsi (latin-1) y escapa paréntesis y barras.
func escapar(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '(' || r == ')' || r == '\\':
			b.WriteByte('\\')
			b.WriteByte(byte(r))
		case r == '€':
			b.WriteByte(0x80)
		case r == '·':
			b.WriteByte(0xB7)
		case r == '–' || r == '—':
			b.WriteByte('-')
		case r == '…':
			b.WriteByte(0x85)
		case r == '«' || r == '»':
			b.WriteByte(byte(r))
		case r == '−':
			b.WriteByte('-')
		case r == '✓':
			b.WriteByte('v')
		case r < 256:
			b.WriteByte(byte(r))
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

// Hoja escribe de arriba abajo y pasa de página sola; cada página lleva el pie con su número.
type Hoja struct {
	D      *Doc
	Y      float64
	Pie    string
	pagina int
}

// NuevaHoja abre un documento con un pie común.
func NuevaHoja(pie string) *Hoja {
	return &Hoja{D: Nuevo(), Y: 800, Pie: pie, pagina: 1}
}

func (h *Hoja) pie() {
	h.D.Color(0.39, 0.45, 0.55)
	h.D.Texto(40, 28, 8, false, h.Pie)
	h.D.TextoDerecha(555, 28, 8, false, fmt.Sprintf("Página %d", h.pagina))
	h.D.Color(0.06, 0.09, 0.16)
}

// Reservar asegura alto puntos libres en la página; si no caben, abre otra.
func (h *Hoja) Reservar(alto float64) {
	if h.Y-alto < 56 {
		h.pie()
		h.D.NuevaPagina()
		h.pagina++
		h.Y = 800
	}
}

// Titulo de sección con una línea debajo.
func (h *Hoja) Titulo(s string) {
	h.Reservar(44)
	h.Y -= 8
	h.D.Texto(40, h.Y, 13, true, s)
	h.D.Linea(40, h.Y-6, 555, h.Y-6)
	h.Y -= 22
}

// Fila: texto a la izquierda (con sangría) y cifra a la derecha.
func (h *Hoja) Fila(sangria, tam float64, negrita bool, izq, der string) {
	h.Reservar(tam + 6)
	h.D.Texto(40+sangria, h.Y, tam, negrita, recorte(izq, int((515-sangria-float64(len([]rune(der)))*tam*0.55)/(tam*0.5))))
	if der != "" {
		h.D.TextoDerecha(555, h.Y, tam, negrita, der)
	}
	h.Y -= tam + 6
}

// Columnas escribe una fila de tabla: x de cada columna (las que empiezan con «>» se alinean a la derecha).
func (h *Hoja) Columnas(tam float64, negrita bool, xs []float64, textos []string) {
	h.Reservar(tam + 6)
	for i, t := range textos {
		if i >= len(xs) {
			break
		}
		if strings.HasPrefix(t, ">") {
			h.D.TextoDerecha(xs[i], h.Y, tam, negrita, strings.TrimPrefix(t, ">"))
		} else {
			h.D.Texto(xs[i], h.Y, tam, negrita, t)
		}
	}
	h.Y -= tam + 6
}

// Parrafo parte el texto en líneas de hasta ~ancho caracteres.
func (h *Hoja) Parrafo(tam float64, texto string) {
	max := int(515 / (tam * 0.5))
	for _, l := range partir(texto, max) {
		h.Reservar(tam + 5)
		h.D.Texto(40, h.Y, tam, false, l)
		h.Y -= tam + 5
	}
}

// Espacio baja el cursor.
func (h *Hoja) Espacio(pt float64) { h.Y -= pt }

// Bytes cierra la última página (con su pie) y arma el PDF.
func (h *Hoja) Bytes() []byte {
	h.pie()
	return h.D.Bytes()
}

func recorte(s string, n int) string {
	r := []rune(s)
	if n < 4 || len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func partir(s string, max int) []string {
	var out []string
	for _, par := range strings.Split(s, "\n") {
		linea := ""
		for _, p := range strings.Fields(par) {
			if linea != "" && len([]rune(linea))+1+len([]rune(p)) > max {
				out = append(out, linea)
				linea = p
				continue
			}
			if linea != "" {
				linea += " "
			}
			linea += p
		}
		out = append(out, linea)
	}
	return out
}
