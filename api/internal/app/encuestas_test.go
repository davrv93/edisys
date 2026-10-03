package app_test

import (
	"fmt"
	"testing"
)

// Bloque J1 · encuestas: borrador → abierta → cerrada, una respuesta por usuario,
// opciones de su pregunta, resultados agregados y anonimato.
func TestEncuestas(t *testing.T) {
	e := nuevo(t)
	adm := e.login("admin@demo.pe")
	prop := e.login("propietario201@demo.pe")
	junta := e.login("junta@demo.pe")
	inq := e.login("inquilino@demo.pe")
	base := "/api/v1/edificios/1/encuestas"

	// Validación: una pregunta de opción con una sola opción no pasa.
	if st, d := e.pedir("POST", base, adm, map[string]any{"titulo": "X", "preguntas": []any{
		map[string]any{"texto": "¿?", "tipo": "unica", "opciones": []string{"Sí", " sí "}},
	}}); st != 422 {
		t.Fatalf("opciones repetidas deberían fallar: %d %v", st, d)
	}
	// El propietario no crea encuestas.
	if st, _ := e.pedir("POST", base, prop, map[string]any{"titulo": "X"}); st != 403 {
		t.Fatalf("propietario crea encuesta: %d", st)
	}

	st, d := e.pedir("POST", base, adm, map[string]any{
		"titulo": "Pintado de fachada", "anonima": false,
		"preguntas": []any{
			map[string]any{"texto": "¿Qué color?", "tipo": "unica", "opciones": []string{"Blanco", "Gris", "Beige"}},
			map[string]any{"texto": "¿Qué áreas mejorar?", "tipo": "multiple", "opciones": []string{"Lobby", "Gimnasio", "Azotea"}},
			map[string]any{"texto": "Comentarios", "tipo": "texto", "obligatoria": false},
		},
	})
	if st != 201 {
		t.Fatalf("crear: %d %v", st, d)
	}
	id := int64(d["id"].(float64))
	ruta := fmt.Sprintf("%s/%d", base, id)

	// En borrador: el propietario no la ve ni responde.
	if st, _ := e.pedir("GET", ruta, prop, nil); st != 404 {
		t.Fatalf("propietario ve borrador: %d", st)
	}
	_, det := e.pedir("GET", ruta, adm, nil)
	pregs := det["preguntas"].([]any)
	if len(pregs) != 3 {
		t.Fatalf("preguntas: %v", pregs)
	}
	p1 := pregs[0].(map[string]any)
	p2 := pregs[1].(map[string]any)
	p3 := pregs[2].(map[string]any)
	op := func(p map[string]any, i int) int64 {
		return int64(p["opciones"].([]any)[i].(map[string]any)["id"].(float64))
	}
	pid := func(p map[string]any) int64 { return int64(p["id"].(float64)) }

	if st, _ := e.pedir("POST", ruta+"/respuestas", prop, map[string]any{"respuestas": []any{}}); st != 404 {
		t.Fatalf("responder borrador: %d", st)
	}
	if st, _ := e.pedir("POST", ruta+"/abrir", adm, nil); st != 200 {
		t.Fatalf("abrir: %d", st)
	}
	// Abierta ya no se edita ni se borra.
	if st, _ := e.pedir("PUT", ruta, adm, map[string]any{"titulo": "Otro", "preguntas": []any{map[string]any{"texto": "a", "tipo": "texto"}}}); st != 409 {
		t.Fatalf("editar abierta: %d", st)
	}
	if st, _ := e.pedir("DELETE", ruta, adm, nil); st != 409 {
		t.Fatalf("borrar abierta: %d", st)
	}

	// Opción de otra pregunta → 422; dos opciones en pregunta única → 422; falta obligatoria → 422.
	if st, _ := e.pedir("POST", ruta+"/respuestas", prop, map[string]any{"respuestas": []any{
		map[string]any{"pregunta_id": pid(p1), "opcion_ids": []int64{op(p2, 0)}},
		map[string]any{"pregunta_id": pid(p2), "opcion_ids": []int64{op(p2, 0)}},
	}}); st != 422 {
		t.Fatalf("opción ajena: %d", st)
	}
	if st, _ := e.pedir("POST", ruta+"/respuestas", prop, map[string]any{"respuestas": []any{
		map[string]any{"pregunta_id": pid(p1), "opcion_ids": []int64{op(p1, 0), op(p1, 1)}},
		map[string]any{"pregunta_id": pid(p2), "opcion_ids": []int64{op(p2, 0)}},
	}}); st != 422 {
		t.Fatalf("dos en única: %d", st)
	}
	if st, _ := e.pedir("POST", ruta+"/respuestas", prop, map[string]any{"respuestas": []any{
		map[string]any{"pregunta_id": pid(p1), "opcion_ids": []int64{op(p1, 0)}},
	}}); st != 422 {
		t.Fatalf("falta obligatoria: %d", st)
	}

	// Tres respuestas válidas: Blanco, Blanco, Gris; múltiple con repeticiones.
	responder := func(tok string, color int, areas []int, texto string) int {
		ids := []int64{}
		for _, a := range areas {
			ids = append(ids, op(p2, a))
		}
		st, d := e.pedir("POST", ruta+"/respuestas", tok, map[string]any{"respuestas": []any{
			map[string]any{"pregunta_id": pid(p1), "opcion_ids": []int64{op(p1, color)}},
			map[string]any{"pregunta_id": pid(p2), "opcion_ids": ids},
			map[string]any{"pregunta_id": pid(p3), "texto": texto},
		}})
		if st != 201 && st != 409 {
			t.Fatalf("responder: %d %v", st, d)
		}
		return st
	}
	if responder(prop, 0, []int{0, 1}, "Que sea pronto") != 201 {
		t.Fatal("primera respuesta")
	}
	if responder(prop, 1, []int{2}, "") != 409 {
		t.Fatal("doble respuesta debería ser 409")
	}
	responder(junta, 0, []int{0}, "")
	responder(inq, 1, []int{0, 2}, "")

	// El propietario no ve resultados antes del cierre; la junta sí.
	if st, _ := e.pedir("GET", ruta+"/resultados", prop, nil); st != 403 {
		t.Fatalf("propietario ve resultados abiertos: %d", st)
	}
	st, res := e.pedir("GET", ruta+"/resultados", junta, nil)
	if st != 200 || res["respuestas"].(float64) != 3 {
		t.Fatalf("resultados junta: %d %v", st, res)
	}
	rp := res["preguntas"].([]any)
	color := rp[0].(map[string]any)["opciones"].([]any)
	votos := func(o any) (float64, float64) {
		m := o.(map[string]any)
		return m["votos"].(float64), m["porcentaje"].(float64)
	}
	if v, pct := votos(color[0]); v != 2 || pct != 67 {
		t.Fatalf("Blanco: %v %v", v, pct)
	}
	if v, pct := votos(color[1]); v != 1 || pct != 33 {
		t.Fatalf("Gris: %v %v", v, pct)
	}
	if v, _ := votos(color[2]); v != 0 {
		t.Fatalf("Beige: %v", v)
	}
	areas := rp[1].(map[string]any)["opciones"].([]any)
	if v, pct := votos(areas[0]); v != 3 || pct != 100 { // Lobby: los tres
		t.Fatalf("Lobby: %v %v", v, pct)
	}
	textos := rp[2].(map[string]any)["textos"].([]any)
	if len(textos) != 1 || textos[0].(map[string]any)["autor"] != "María Demo" {
		t.Fatalf("textos no anónimos: %v", textos)
	}

	// Cerrada: ya no se responde, y el propietario ve resultados.
	if st, _ := e.pedir("POST", ruta+"/cerrar", adm, nil); st != 200 {
		t.Fatalf("cerrar: %d", st)
	}
	if st, _ := e.pedir("POST", ruta+"/cerrar", adm, nil); st != 409 {
		t.Fatalf("cerrar dos veces: %d", st)
	}
	if st, d := e.pedir("POST", ruta+"/respuestas", e.login("junta2@demo.pe"), map[string]any{"respuestas": []any{
		map[string]any{"pregunta_id": pid(p1), "opcion_ids": []int64{op(p1, 0)}},
		map[string]any{"pregunta_id": pid(p2), "opcion_ids": []int64{op(p2, 0)}},
	}}); st != 409 || codigo(d) != "ENCUESTA_CERRADA" {
		t.Fatalf("responder cerrada: %d %v", st, d)
	}
	if st, _ := e.pedir("GET", ruta+"/resultados", prop, nil); st != 200 {
		t.Fatalf("propietario ve resultados al cierre: %d", st)
	}

	// Encuesta anónima: los textos salen sin autor.
	st, d = e.pedir("POST", base, adm, map[string]any{"titulo": "Clima", "preguntas": []any{
		map[string]any{"texto": "Opinión", "tipo": "texto"},
	}})
	if st != 201 {
		t.Fatalf("crear anónima: %d %v", st, d)
	}
	ra := fmt.Sprintf("%s/%d", base, int64(d["id"].(float64)))
	e.pedir("POST", ra+"/abrir", adm, nil)
	_, da := e.pedir("GET", ra, prop, nil)
	pa := int64(da["preguntas"].([]any)[0].(map[string]any)["id"].(float64))
	if st, _ := e.pedir("POST", ra+"/respuestas", prop, map[string]any{"respuestas": []any{map[string]any{"pregunta_id": pa, "texto": "Bien"}}}); st != 201 {
		t.Fatalf("responder anónima: %d", st)
	}
	_, resA := e.pedir("GET", ra+"/resultados", adm, nil)
	ta := resA["preguntas"].([]any)[0].(map[string]any)["textos"].([]any)
	if len(ta) != 1 || ta[0].(map[string]any)["autor"] != nil {
		t.Fatalf("la anónima no debe mostrar autor: %v", ta)
	}

	// Una encuesta de otro edificio no existe para este usuario (aislamiento por edificio).
	if st, _ := e.pedir("GET", "/api/v1/edificios/999/encuestas", adm, nil); st != 404 {
		t.Fatalf("otro edificio: %d", st)
	}
}
