package pdf

// Imágenes incrustadas (bloque I1): el logo de la administradora y la foto del medidor en el recibo.
// Se guardan como RGB comprimido con Flate: sirve igual para JPG, PNG o WebP y no depende del formato de origen.

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	"strings"

	"golang.org/x/image/draw"
)

// LadoMaxImagen: lado mayor, en píxeles, con que se incrusta una imagen (de sobra para A4 a ese tamaño).
const LadoMaxImagen = 480

type imagenPDF struct {
	ancho, alto int
	datos       []byte // RGB comprimido con Flate
}

func (im imagenPDF) objeto() string {
	return fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream",
		im.ancho, im.alto, len(im.datos), im.datos)
}

// recursosImagen arma « /XObject << /Im0 N 0 R … >>» con las imágenes numeradas desde primero.
func (d *Doc) recursosImagen(primero int) string {
	if len(d.imagenes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(" /XObject <<")
	for i := range d.imagenes {
		fmt.Fprintf(&b, " /Im%d %d 0 R", i, primero+i)
	}
	b.WriteString(" >>")
	return b.String()
}

// Imagen dibuja img dentro de la caja (x, y, ancho, alto) sin deformarla: la ajusta y la centra.
// Las transparencias se aplanan sobre blanco. Devuelve el ancho realmente usado.
func (d *Doc) Imagen(x, y, ancho, alto float64, img image.Image) float64 {
	if img == nil || ancho <= 0 || alto <= 0 {
		return 0
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return 0
	}
	// Reduce a LadoMaxImagen para no inflar el PDF con fotos de 12 MP.
	w, h := b.Dx(), b.Dy()
	if w > LadoMaxImagen || h > LadoMaxImagen {
		if w >= h {
			h, w = max(1, h*LadoMaxImagen/w), LadoMaxImagen
		} else {
			w, h = max(1, w*LadoMaxImagen/h), LadoMaxImagen
		}
	}
	lienzo := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(lienzo, lienzo.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(lienzo, lienzo.Bounds(), img, b, draw.Over, nil)

	crudo := make([]byte, 0, w*h*3)
	for i := 0; i < len(lienzo.Pix); i += 4 {
		crudo = append(crudo, lienzo.Pix[i], lienzo.Pix[i+1], lienzo.Pix[i+2])
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write(crudo)
	_ = zw.Close()
	n := len(d.imagenes)
	d.imagenes = append(d.imagenes, imagenPDF{ancho: w, alto: h, datos: z.Bytes()})

	// Ajuste dentro de la caja conservando la proporción.
	esc := min(ancho/float64(w), alto/float64(h))
	dw, dh := float64(w)*esc, float64(h)*esc
	dx, dy := x, y+(alto-dh)/2
	fmt.Fprintf(d.actual, "q %.2f 0 0 %.2f %.2f %.2f cm /Im%d Do Q\n", dw, dh, dx, dy, n)
	return dw
}
