// Package mantenimiento contiene la máquina de estados de incidencias y trabajos (09).
// La misma tabla vive en SQL (función transicion_incidencia_valida, migración 0006):
// si cambias una, cambia la otra; la prueba TestTablaIgualQueSQL lo recuerda.
package mantenimiento

// Estados en el orden del tablero.
var Estados = []string{"reportado", "validado", "presupuestado", "aprobado", "en_ejecucion", "terminado", "rechazado", "descartado"}

// Transiciones permitidas: desde → hacia.
var Transiciones = map[string][]string{
	"reportado":     {"validado", "descartado"},
	"validado":      {"presupuestado", "descartado"},
	"presupuestado": {"aprobado", "rechazado"},
	"rechazado":     {"presupuestado"},
	"aprobado":      {"en_ejecucion"},
	"en_ejecucion":  {"terminado"},
	"terminado":     {},
	"descartado":    {},
}

// EstadoValido dice si el estado existe.
func EstadoValido(e string) bool {
	_, ok := Transiciones[e]
	return ok
}

// PuedePasar dice si la transición está permitida.
func PuedePasar(desde, hacia string) bool {
	for _, h := range Transiciones[desde] {
		if h == hacia {
			return true
		}
	}
	return false
}

// PermisoPara devuelve el permiso que exige llegar a un estado.
func PermisoPara(hacia string) string {
	switch hacia {
	case "validado", "descartado":
		return "incidencias.validar"
	case "presupuestado":
		return "trabajos.presupuestar"
	case "aprobado", "rechazado":
		return "trabajos.aprobar"
	case "en_ejecucion", "terminado":
		return "trabajos.ejecutar"
	}
	return "incidencias.validar"
}

// Votacion calcula el resultado de la votación de la junta.
// modo "presidente": decide el voto del presidente. modo "mayoria": más de la mitad de los miembros activos.
func Votacion(modo string, miembros, aFavor, enContra int, presidenteVoto string) (resultado string, necesarios int) {
	if modo == "presidente" {
		switch presidenteVoto {
		case "aprueba":
			return "aprobado", 1
		case "rechaza":
			return "rechazado", 1
		}
		return "pendiente", 1
	}
	necesarios = miembros/2 + 1
	switch {
	case aFavor >= necesarios:
		return "aprobado", necesarios
	case enContra >= necesarios:
		return "rechazado", necesarios
	case aFavor+(miembros-aFavor-enContra) < necesarios:
		// Ya no hay votos suficientes para aprobar.
		return "rechazado", necesarios
	}
	return "pendiente", necesarios
}
