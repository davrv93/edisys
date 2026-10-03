package app_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Bloque J2 · videollamadas: el enlace sale de JITSI_BASE_URL + código aleatorio,
// solo junta y administración lo ven, y la grabación acepta solo https.
func TestVideollamadas(t *testing.T) {
	t.Setenv("JITSI_BASE_URL", "https://meet.ejemplo.pe/")
	e := nuevo(t)
	adm := e.login("admin@demo.pe")
	junta := e.login("junta@demo.pe")
	prop := e.login("propietario201@demo.pe")
	base := "/api/v1/edificios/1/videollamadas"
	manana := time.Now().Add(24 * time.Hour).Format("2006-01-02T15:04")

	st, d := e.pedir("POST", base, junta, map[string]any{"titulo": "Junta ordinaria", "inicia_en": manana, "duracion_min": 90})
	if st != 201 {
		t.Fatalf("crear: %d %v", st, d)
	}
	id := int64(d["id"].(float64))
	enlace := d["enlace"].(string)
	if !strings.HasPrefix(enlace, "https://meet.ejemplo.pe/edisys-1-") || len(enlace) != len("https://meet.ejemplo.pe/edisys-1-")+20 {
		t.Fatalf("enlace: %q", enlace)
	}
	// Dos salas nunca comparten código.
	_, d2 := e.pedir("POST", base, adm, map[string]any{"titulo": "Otra", "inicia_en": manana})
	if d2["enlace"] == enlace {
		t.Fatal("códigos repetidos")
	}

	// El propietario no ve las salas ni su enlace.
	if st, _ := e.pedir("GET", base, prop, nil); st != 403 {
		t.Fatalf("propietario lista salas: %d", st)
	}
	if st, _ := e.pedir("POST", base, prop, map[string]any{"titulo": "x", "inicia_en": manana}); st != 403 {
		t.Fatalf("propietario crea sala: %d", st)
	}

	st, l := e.pedir("GET", base, adm, nil)
	if st != 200 || len(l["datos"].([]any)) < 2 {
		t.Fatalf("listar: %d %v", st, l)
	}
	primera := l["datos"].([]any)[0].(map[string]any)
	if primera["estado"] != "programada" || !strings.HasPrefix(primera["enlace"].(string), "https://meet.ejemplo.pe/") {
		t.Fatalf("fila: %v", primera)
	}

	// Validaciones: pasado, duración fuera de rango.
	if st, _ := e.pedir("POST", base, adm, map[string]any{"titulo": "x", "inicia_en": "2020-01-01T10:00"}); st != 422 {
		t.Fatalf("en el pasado: %d", st)
	}
	if st, _ := e.pedir("POST", base, adm, map[string]any{"titulo": "x", "inicia_en": manana, "duracion_min": 5}); st != 422 {
		t.Fatalf("duración corta: %d", st)
	}

	// Grabación: solo https.
	ruta := fmt.Sprintf("%s/%d", base, id)
	if st, _ := e.pedir("PUT", ruta+"/grabacion", adm, map[string]any{"url": "http://inseguro.pe/x"}); st != 422 {
		t.Fatalf("grabación http: %d", st)
	}
	if st, _ := e.pedir("PUT", ruta+"/grabacion", adm, map[string]any{"url": "https://grabaciones.ejemplo.pe/junta.mp4"}); st != 200 {
		t.Fatalf("grabación https: %d", st)
	}

	// Cancelar una vez; la segunda es conflicto.
	if st, _ := e.pedir("POST", ruta+"/cancelar", adm, nil); st != 200 {
		t.Fatalf("cancelar: %d", st)
	}
	if st, _ := e.pedir("POST", ruta+"/cancelar", adm, nil); st != 409 {
		t.Fatalf("cancelar dos veces: %d", st)
	}

	// Sin JITSI_BASE_URL válido cae a la instancia pública.
	t.Setenv("JITSI_BASE_URL", "ftp://raro")
	_, c := e.pedir("GET", base+"/config", junta, nil)
	if c["base_url"] != "https://meet.jit.si" || c["propia"] != false {
		t.Fatalf("config por defecto: %v", c)
	}
}
