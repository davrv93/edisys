package mantenimiento

import (
	"os"
	"regexp"
	"testing"
)

func TestCaminoFeliz(t *testing.T) {
	camino := []string{"reportado", "validado", "presupuestado", "aprobado", "en_ejecucion", "terminado"}
	for i := 0; i < len(camino)-1; i++ {
		if !PuedePasar(camino[i], camino[i+1]) {
			t.Errorf("%s → %s debería estar permitido", camino[i], camino[i+1])
		}
	}
}

func TestTransicionesProhibidas(t *testing.T) {
	prohibidas := [][2]string{
		{"reportado", "aprobado"},      // saltarse la validación y el presupuesto
		{"reportado", "terminado"},     //
		{"validado", "en_ejecucion"},   // ejecutar sin aprobación
		{"presupuestado", "terminado"}, //
		{"aprobado", "terminado"},      // terminar sin ejecutar
		{"terminado", "reportado"},     // no se reabre
		{"descartado", "validado"},     //
		{"en_ejecucion", "aprobado"},   // no se retrocede
		{"reportado", "rechazado"},     // se rechaza un presupuesto, no un reporte
	}
	for _, p := range prohibidas {
		if PuedePasar(p[0], p[1]) {
			t.Errorf("%s → %s NO debería estar permitido", p[0], p[1])
		}
	}
	if !PuedePasar("presupuestado", "rechazado") || !PuedePasar("rechazado", "presupuestado") {
		t.Error("rechazo y nuevo presupuesto deben estar permitidos")
	}
}

// La tabla de Go y la función SQL deben decir lo mismo.
func TestTablaIgualQueSQL(t *testing.T) {
	sql, err := os.ReadFile("../../migrations/0006_mantenimiento.sql")
	if err != nil {
		t.Skip("no encuentro la migración:", err)
	}
	re := regexp.MustCompile(`\('([a-z_]+)','([a-z_]+)'\)`)
	pares := map[[2]string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(sql), -1) {
		if EstadoValido(m[1]) && EstadoValido(m[2]) {
			pares[[2]string{m[1], m[2]}] = true
		}
	}
	n := 0
	for desde, hs := range Transiciones {
		for _, h := range hs {
			n++
			if !pares[[2]string{desde, h}] {
				t.Errorf("%s → %s está en Go y no en SQL", desde, h)
			}
		}
	}
	if n != len(pares) {
		t.Errorf("Go tiene %d transiciones y SQL %d", n, len(pares))
	}
}

func TestVotacionINC014(t *testing.T) {
	// Junta de 5, modo mayoría: con 2 votos a favor sigue pendiente; con el tercero se aprueba.
	if r, n := Votacion("mayoria", 5, 2, 0, ""); r != "pendiente" || n != 3 {
		t.Errorf("2 de 3: %s (%d)", r, n)
	}
	if r, _ := Votacion("mayoria", 5, 3, 0, ""); r != "aprobado" {
		t.Errorf("3 de 3: %s", r)
	}
	if r, _ := Votacion("mayoria", 5, 1, 3, ""); r != "rechazado" {
		t.Errorf("3 en contra: %s", r)
	}
	if r, _ := Votacion("presidente", 5, 0, 0, "aprueba"); r != "aprobado" {
		t.Errorf("presidente: %s", r)
	}
}
