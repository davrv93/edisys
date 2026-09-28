package plataforma

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // la imagen distroless no trae zonas horarias
)

// Lima es la zona horaria de pantalla.
var Lima = func() *time.Location {
	loc, err := time.LoadLocation("America/Lima")
	if err != nil {
		return time.FixedZone("America/Lima", -5*3600)
	}
	return loc
}()

// Soles formatea céntimos como «S/ 4.800,00» (punto de miles, coma decimal: regla 8 de la guía).
func Soles(cts int64) string {
	signo := ""
	if cts < 0 {
		signo = "-"
		cts = -cts
	}
	entero := strconv.FormatInt(cts/100, 10)
	var b strings.Builder
	for i, c := range entero {
		if i > 0 && (len(entero)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return fmt.Sprintf("%sS/ %s,%02d", signo, b.String(), cts%100)
}

// Pct formatea un porcentaje con coma decimal: 13,1 %.
func Pct(v float64, decimales int) string {
	s := strconv.FormatFloat(v, 'f', decimales, 64)
	return strings.Replace(s, ".", ",", 1) + " %"
}

// Milesimas convierte "14.000" (m³ con 3 decimales) a 14000 (litros). Acepta coma o punto.
func Milesimas(s string) (int64, error) {
	s = strings.TrimSpace(strings.Replace(s, ",", ".", 1))
	if s == "" {
		return 0, fmt.Errorf("vacío")
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	partes := strings.SplitN(s, ".", 2)
	ent, err := strconv.ParseInt(partes[0], 10, 64)
	if err != nil {
		return 0, err
	}
	frac := int64(0)
	if len(partes) == 2 {
		f := partes[1]
		if len(f) > 3 {
			f = f[:3]
		}
		for len(f) < 3 {
			f += "0"
		}
		frac, err = strconv.ParseInt(f, 10, 64)
		if err != nil {
			return 0, err
		}
	}
	v := ent*1000 + frac
	if neg {
		v = -v
	}
	return v, nil
}

// TextoMilesimas convierte 14000 a "14.000".
func TextoMilesimas(v int64) string {
	signo := ""
	if v < 0 {
		signo = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d.%03d", signo, v/1000, v%1000)
}

// PeriodoValido comprueba el formato AAAA-MM.
func PeriodoValido(p string) bool {
	if len(p) != 7 || p[4] != '-' {
		return false
	}
	_, err := time.Parse("2006-01", p)
	return err == nil
}

// PeriodoActual devuelve el periodo de hoy en Lima.
func PeriodoActual() string { return time.Now().In(Lima).Format("2006-01") }

// PeriodoAnterior devuelve el periodo previo ("2026-09" → "2026-08").
func PeriodoAnterior(p string) string {
	t, err := time.Parse("2006-01", p)
	if err != nil {
		return ""
	}
	return t.AddDate(0, -1, 0).Format("2006-01")
}

// PeriodoSiguiente devuelve el periodo siguiente.
func PeriodoSiguiente(p string) string {
	t, err := time.Parse("2006-01", p)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 1, 0).Format("2006-01")
}

// RangoPeriodo devuelve [inicio, fin) del mes en hora de Lima.
func RangoPeriodo(p string) (time.Time, time.Time) {
	t, _ := time.ParseInLocation("2006-01", p, Lima)
	return t, t.AddDate(0, 1, 0)
}

var mesesES = []string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "setiembre", "octubre", "noviembre", "diciembre"}

// NombreMes devuelve «setiembre» para 9.
func NombreMes(m time.Month) string { return mesesES[int(m)-1] }

// NombrePeriodo devuelve «setiembre 2026».
func NombrePeriodo(p string) string {
	t, err := time.Parse("2006-01", p)
	if err != nil {
		return p
	}
	return NombreMes(t.Month()) + " " + strconv.Itoa(t.Year())
}

// EnmascararDNI muestra «4512****».
func EnmascararDNI(d string) string {
	if len(d) <= 4 {
		return d
	}
	return d[:4] + strings.Repeat("*", len(d)-4)
}
