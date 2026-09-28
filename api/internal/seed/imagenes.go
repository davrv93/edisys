package seed

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"edisys/api/internal/pdf"
)

// imagen genera una foto de demostración (PNG 480×320) con un título y líneas de texto,
// para que el visor de documentos muestre algo legible (medidor, recibo, voucher).
func imagen(fondo color.RGBA, titulo string, lineas ...string) []byte {
	chica := image.NewRGBA(image.Rect(0, 0, 240, 160))
	draw.Draw(chica, chica.Bounds(), &image.Uniform{fondo}, image.Point{}, draw.Src)
	// Marco
	borde := color.RGBA{255, 255, 255, 180}
	for x := 4; x < 236; x++ {
		chica.Set(x, 4, borde)
		chica.Set(x, 155, borde)
	}
	for y := 4; y < 156; y++ {
		chica.Set(4, y, borde)
		chica.Set(235, y, borde)
	}
	d := &font.Drawer{Dst: chica, Src: image.White, Face: basicfont.Face7x13}
	d.Dot = fixed.P(12, 24)
	d.DrawString(titulo)
	for i, l := range lineas {
		d.Dot = fixed.P(12, 50+i*18)
		d.DrawString(l)
	}
	grande := image.NewRGBA(image.Rect(0, 0, 480, 320))
	draw.NearestNeighbor.Scale(grande, grande.Bounds(), chica, chica.Bounds(), draw.Src, nil)
	var b bytes.Buffer
	_ = png.Encode(&b, grande)
	return b.Bytes()
}

var (
	colorMedidor = color.RGBA{0x33, 0x41, 0x55, 255}
	colorRecibo  = color.RGBA{0x15, 0x5E, 0x75, 255}
	colorVoucher = color.RGBA{0x74, 0x2F, 0x8F, 255}
	colorObra    = color.RGBA{0xB4, 0x53, 0x09, 255}
)

// facturaPDF genera un PDF simple de factura para el balance.
func facturaPDF(emisor, concepto, monto, fecha string) []byte {
	d := pdf.Nuevo()
	d.Texto(50, 780, 18, true, emisor)
	d.Texto(50, 760, 10, false, "RUC 20600000001 · Lima")
	d.TextoDerecha(545, 780, 14, true, "FACTURA F001-000"+fecha[5:7]+fecha[8:10])
	d.Linea(50, 740, 545, 740)
	d.Texto(50, 715, 11, false, "Cliente: Junta de Propietarios Edificio Demo")
	d.Texto(50, 698, 11, false, "Fecha de emisión: "+fecha)
	d.Texto(50, 660, 11, true, "Descripción")
	d.TextoDerecha(545, 660, 11, true, "Importe")
	d.Linea(50, 652, 545, 652)
	d.Texto(50, 632, 11, false, concepto)
	d.TextoDerecha(545, 632, 11, false, monto)
	d.Linea(50, 610, 545, 610)
	d.Texto(50, 590, 12, true, "Total")
	d.TextoDerecha(545, 590, 12, true, monto)
	d.Texto(50, 60, 8, false, "Documento de demostración generado por la semilla de EDISYS.")
	return d.Bytes()
}
