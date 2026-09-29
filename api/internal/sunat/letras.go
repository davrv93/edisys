package sunat

import (
	"fmt"
	"strings"
)

var unidades = []string{"", "UNO", "DOS", "TRES", "CUATRO", "CINCO", "SEIS", "SIETE", "OCHO", "NUEVE", "DIEZ", "ONCE", "DOCE", "TRECE", "CATORCE", "QUINCE",
	"DIECISEIS", "DIECISIETE", "DIECIOCHO", "DIECINUEVE", "VEINTE", "VEINTIUNO", "VEINTIDOS", "VEINTITRES", "VEINTICUATRO", "VEINTICINCO", "VEINTISEIS",
	"VEINTISIETE", "VEINTIOCHO", "VEINTINUEVE"}
var decenas = []string{"", "", "", "TREINTA", "CUARENTA", "CINCUENTA", "SESENTA", "SETENTA", "OCHENTA", "NOVENTA"}
var centenas = []string{"", "CIENTO", "DOSCIENTOS", "TRESCIENTOS", "CUATROCIENTOS", "QUINIENTOS", "SEISCIENTOS", "SETECIENTOS", "OCHOCIENTOS", "NOVECIENTOS"}

func hasta999(n int64) string {
	if n == 0 {
		return ""
	}
	if n == 100 {
		return "CIEN"
	}
	var p []string
	if c := n / 100; c > 0 {
		p = append(p, centenas[c])
	}
	r := n % 100
	switch {
	case r == 0:
	case r < 30:
		p = append(p, unidades[r])
	default:
		d := decenas[r/10]
		if u := r % 10; u > 0 {
			d += " Y " + unidades[u]
		}
		p = append(p, d)
	}
	return strings.Join(p, " ")
}

// Letras escribe un entero en palabras (hasta 999 999 999).
func Letras(n int64) string {
	if n == 0 {
		return "CERO"
	}
	var p []string
	if mill := n / 1_000_000; mill > 0 {
		if mill == 1 {
			p = append(p, "UN MILLON")
		} else {
			p = append(p, strings.Replace(hasta999(mill), "VEINTIUNO", "VEINTIUN", 1)+" MILLONES")
		}
	}
	if miles := n / 1000 % 1000; miles > 0 {
		if miles == 1 {
			p = append(p, "MIL")
		} else {
			t := hasta999(miles)
			if strings.HasSuffix(t, "UNO") {
				t = strings.TrimSuffix(t, "O")
			}
			p = append(p, t+" MIL")
		}
	}
	if r := n % 1000; r > 0 {
		p = append(p, hasta999(r))
	}
	return strings.Join(p, " ")
}

// MontoEnLetras: la leyenda 1000 de SUNAT («SON NOVECIENTOS NOVENTA CON 00/100 SOLES»).
func MontoEnLetras(cts int64) string {
	return fmt.Sprintf("SON %s CON %02d/100 SOLES", Letras(cts/100), cts%100)
}
