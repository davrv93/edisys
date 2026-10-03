package pdf

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

// Bloque I1 · la tabla de Code 128 está completa y bien formada.
func TestTabla128(t *testing.T) {
	if len(patrones128) != 107 {
		t.Fatalf("la tabla debe tener 107 símbolos, tiene %d", len(patrones128))
	}
	vistos := map[string]bool{}
	for i, p := range patrones128 {
		suma := 0
		for _, c := range p {
			suma += int(c - '0')
		}
		quiere := 11
		if i == 106 {
			quiere = 13
		}
		if suma != quiere {
			t.Errorf("símbolo %d (%s) suma %d módulos, debe sumar %d", i, p, suma, quiere)
		}
		if vistos[p] {
			t.Errorf("símbolo %d repetido: %s", i, p)
		}
		vistos[p] = true
	}
}

// Ejemplo conocido: «PJJ123C» con inicio B (104) lleva el dígito de control 55 (879 mod 103).
func TestModulos128(t *testing.T) {
	m := Modulos128("PJJ123C")
	// inicio + 7 caracteres + control (9 símbolos de 6 anchos) + parada de 7.
	if len(m) != 9*6+7 {
		t.Fatalf("largo %d", len(m))
	}
	suma := (104 + 48*1 + 42*2 + 42*3 + 17*4 + 18*5 + 19*6 + 35*7) % 103
	if suma != 55 {
		t.Fatalf("control esperado 55, sale %d", suma)
	}
	ctrl := m[8*6 : 9*6]
	for i, c := range patrones128[55] {
		if ctrl[i] != int(c-'0') {
			t.Fatalf("el símbolo de control no es el 55: %v", ctrl)
		}
	}
}

// Una imagen incrustada aparece como XObject y el PDF sigue cerrando bien.
func TestImagenEnPDF(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 900, 300))
	for x := 0; x < 900; x++ {
		for y := 0; y < 300; y++ {
			img.Set(x, y, color.NRGBA{21, 94, 117, 255})
		}
	}
	d := Nuevo()
	usado := d.Imagen(40, 700, 120, 40, img)
	if usado < 119 || usado > 121 {
		t.Fatalf("la imagen 3:1 en una caja 120×40 debe usar 120 de ancho, usa %.1f", usado)
	}
	d.Barras128(40, 600, 30, 1, "R-0001")
	b := d.Bytes()
	for _, quiere := range []string{"/XObject << /Im0", "/Subtype /Image", "/Width 480", "/Im0 Do", "%%EOF"} {
		if !bytes.Contains(b, []byte(quiere)) {
			t.Errorf("falta %q en el PDF", quiere)
		}
	}
}
