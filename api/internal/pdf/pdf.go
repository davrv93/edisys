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
	paginas []*bytes.Buffer
	actual  *bytes.Buffer
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
	for i, p := range d.paginas {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>", 6+2*i))
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", p.Len(), p.String()))
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
		case r < 256:
			b.WriteByte(byte(r))
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}
