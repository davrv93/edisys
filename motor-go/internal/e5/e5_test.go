package e5

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// Estas pruebas necesitan los archivos reales del modelo y libonnxruntime:
//
//	E5_RUTA=…/e5-model_quantized.onnx E5_TOK=…/e5-tokenizer.json ORT_LIB=…/libonnxruntime.so
//
// Sin ellos se saltan (las pruebas del servidor usan un embebedor falso).
func cargar(t *testing.T) *Embebedor {
	t.Helper()
	modelo, tok := os.Getenv("E5_RUTA"), os.Getenv("E5_TOK")
	if modelo == "" || tok == "" {
		t.Skip("E5_RUTA/E5_TOK no definidos")
	}
	if _, err := os.Stat(modelo); err != nil {
		t.Skip("sin modelo e5")
	}
	e, err := Nuevo(modelo, tok, os.Getenv("ORT_LIB"), 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Cerrar)
	return e
}

func coseno(a []float32, b []float64) float64 {
	var p, na, nb float64
	for i := range a {
		p += float64(a[i]) * b[i]
		na += float64(a[i]) * float64(a[i])
		nb += b[i] * b[i]
	}
	return p / math.Sqrt(na*nb)
}

// Tokens y vectores de referencia sacados del motor Python (testdata/e5_ref.json):
// consultas cortas, un texto de >128 tokens (truncado) y emojis/acentos.
func TestIgualQuePython(t *testing.T) {
	e := cargar(t)
	data, err := os.ReadFile("../../testdata/e5_ref.json")
	if err != nil {
		t.Skip("sin e5_ref.json")
	}
	var refs []struct {
		Texto  string    `json:"texto"`
		IDs    []uint32  `json:"ids"`
		Vector []float64 `json:"vector"`
		// El propio Python no es estable entre lote y suelto: la cuantización
		// dinámica del ONNX calcula la escala con todo el tensor (relleno incluido).
		CosenoSoloLote float64   `json:"coseno_solo_vs_lote"`
		VectorLote     []float64 `json:"vector_lote"`
	}
	if err := json.Unmarshal(data, &refs); err != nil {
		t.Fatal(err)
	}
	for _, r := range refs {
		ids := e.Tokens(r.Texto)
		if len(ids) != len(r.IDs) {
			t.Errorf("%q: %d tokens, Python %d", corto(r.Texto), len(ids), len(r.IDs))
			continue
		}
		for i := range ids {
			if ids[i] != r.IDs[i] {
				t.Errorf("%q: token %d = %d, Python %d", corto(r.Texto), i, ids[i], r.IDs[i])
				break
			}
		}
		v, err := e.Embeber([]string{r.Texto})
		if err != nil {
			t.Fatal(err)
		}
		if d := distintos(v[0], r.Vector); d > 0 {
			t.Errorf("%d de %d componentes no son bit a bit iguales a Python (%q)", d, len(v[0]), corto(r.Texto))
		}
		c := coseno(v[0], r.Vector)
		t.Logf("coseno Go vs Python (mismo texto suelto) %.6f · Python suelto vs lote %.6f · %q", c, r.CosenoSoloLote, corto(r.Texto))
		if c < 0.999 {
			t.Errorf("coseno %.4f < 0,999 para %q", c, r.Texto)
		}
	}
	// El mismo lote que Python (los 5 textos juntos, con relleno) → mismos vectores.
	textos := make([]string, len(refs))
	for i, r := range refs {
		textos[i] = r.Texto
	}
	lote, err := e.Embeber(textos)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range refs {
		if d := distintos(lote[i], r.VectorLote); d > 0 {
			t.Errorf("lote: %d de %d componentes no son bit a bit iguales a Python (%q)", d, len(lote[i]), corto(r.Texto))
		}
		c := coseno(lote[i], r.VectorLote)
		t.Logf("lote: coseno Go vs Python %.6f · %q", c, corto(r.Texto))
		if c < 0.999 {
			t.Errorf("lote: coseno %.4f < 0,999 para %q", c, corto(r.Texto))
		}
	}
}

// Los vectores guardados en el faq.json real del volumen deben reproducirse:
// así el índice existente sigue valiendo sin reindexar.
func TestFaqRealCoseno(t *testing.T) {
	e := cargar(t)
	data, err := os.ReadFile("../../testdata/faq.json")
	if err != nil {
		t.Skip("sin faq.json")
	}
	var faq []struct {
		ID     string    `json:"id"`
		Texto  string    `json:"texto"`
		Vector []float64 `json:"vector"`
	}
	if err := json.Unmarshal(data, &faq); err != nil {
		t.Fatal(err)
	}
	// _indexar() vectorizó los 15 fragmentos en un solo lote: se repite igual.
	textos := make([]string, len(faq))
	for i, f := range faq {
		textos[i] = "passage: " + f.Texto
	}
	vecs, err := e.Embeber(textos)
	if err != nil {
		t.Fatal(err)
	}
	probados := 0
	for i, f := range faq {
		if len(f.Vector) == 0 {
			continue
		}
		if d := distintos(vecs[i], f.Vector); d > 0 {
			t.Errorf("%s: %d componentes distintos del faq.json real", f.ID, d)
		}
		c := coseno(vecs[i], f.Vector)
		t.Logf("%-28s coseno %.6f", f.ID, c)
		if c < 0.99 {
			t.Errorf("%s: coseno %.4f < 0,99", f.ID, c)
		}
		probados++
	}
	if probados < 5 {
		t.Fatalf("solo %d fragmentos con vector", probados)
	}
}

func corto(s string) string {
	r := []rune(s)
	if len(r) > 24 {
		r = r[:24]
	}
	return string(r)
}

// distintos cuenta las componentes que no coinciden bit a bit (Python guarda
// float32 convertidos a double: la conversión inversa es exacta).
func distintos(a []float32, b []float64) int {
	n := 0
	for i := range a {
		if a[i] != float32(b[i]) {
			n++
		}
	}
	return n
}
