package motor

import (
	"encoding/json"
	"os"
	"testing"

	"edisys/motor-go/internal/pyjson"
)

// Salidas de referencia generadas ejecutando las funciones de motor/app.py
// (testdata/py_ref.json). Se comparan byte a byte.
type refPy struct {
	Ensamblar []struct {
		Mensajes []Mensaje      `json:"mensajes"`
		Frag     []any          `json:"frag"`
		Mem      []any          `json:"mem"`
		Datos    *pyjson.Value  `json:"datos"`
		Salida   []mensajeLlama `json:"salida"`
	} `json:"ensamblar"`
	Jaccard []struct {
		A, B string
		J    float64
	} `json:"jaccard"`
	Filtro []struct{ Texto, Salida string } `json:"filtro"`
	Frag   []any                            `json:"frag"`
}

func (m *Mensaje) UnmarshalJSON(b []byte) error {
	var x mensajeLlama
	if err := json.Unmarshal(b, &x); err != nil {
		return err
	}
	*m = Mensaje{x.Role, x.Content}
	return nil
}

func cargarRef(t *testing.T) refPy {
	t.Helper()
	data, err := os.ReadFile("../../testdata/py_ref.json")
	if err != nil {
		t.Fatal(err)
	}
	var r refPy
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func itemsDe(t *testing.T, xs []any) []*Item {
	t.Helper()
	b, _ := json.Marshal(xs)
	v, err := pyjson.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	its, err := items(v)
	if err != nil {
		t.Fatal(err)
	}
	return its
}

func TestEnsamblarIgualQuePython(t *testing.T) {
	r := cargarRef(t)
	for i, c := range r.Ensamblar {
		got, err := ensamblar(c.Mensajes, itemsDe(t, c.Frag), itemsDe(t, c.Mem), c.Datos)
		if err != nil {
			t.Fatalf("caso %d: %v", i, err)
		}
		if len(got) != len(c.Salida) {
			t.Fatalf("caso %d: %d mensajes, Python %d", i, len(got), len(c.Salida))
		}
		for j := range got {
			if got[j].Role != c.Salida[j].Role || got[j].Content != c.Salida[j].Content {
				t.Errorf("caso %d, mensaje %d difiere:\nGo:     %q\nPython: %q", i, j, got[j].Content, c.Salida[j].Content)
			}
		}
	}
}

func TestJaccardIgualQuePython(t *testing.T) {
	for _, c := range cargarRef(t).Jaccard {
		if got := jaccard(c.A, c.B); got != c.J {
			t.Errorf("jaccard(%q,%q) = %v, Python %v", c.A, c.B, got, c.J)
		}
	}
}

func TestFiltroIgualQuePython(t *testing.T) {
	r := cargarRef(t)
	frag := itemsDe(t, r.Frag)
	m := Nuevo(t.TempDir(), "http://x", nil)
	m.votarFiltro(idsDe(frag), -1)
	for _, c := range r.Filtro {
		got, err := m.aplicarFiltro(c.Texto, frag)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.Salida {
			t.Errorf("filtro(%q) = %q, Python %q", c.Texto, got, c.Salida)
		}
	}
}
