package pyjson

import (
	"math"
	"os"
	"testing"
)

func TestFloatRepr(t *testing.T) {
	// Valores de referencia sacados de repr() de CPython 3.
	casos := map[float64]string{
		0.0: "0.0", 1.0: "1.0", 0.1: "0.1", 1e-05: "1e-05", 0.0001: "0.0001",
		123456789012345.6: "123456789012345.6", 1e16: "1e+16", 1.5e-07: "1.5e-07",
		-0.04701545834541321: "-0.04701545834541321", 1e22: "1e+22", 3.0e-5: "3e-05",
		100.0: "100.0", 2.5e+20: "2.5e+20",
	}
	for f, want := range casos {
		if got := FloatRepr(f); got != want {
			t.Errorf("FloatRepr(%v) = %q, quiero %q", f, got, want)
		}
	}
	if got := FloatRepr(math.Copysign(0, -1)); got != "-0.0" {
		t.Errorf("-0.0 → %q", got)
	}
}

func TestDumpsComoPython(t *testing.T) {
	v, err := Parse([]byte(`{"a":[1,{"b":"á\"\\\n\u0001"}],"c":{},"d":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n \"a\": [\n  1,\n  {\n   \"b\": \"á\\\"\\\\\\n\\u0001\"\n  }\n ],\n \"c\": {},\n \"d\": []\n}"
	if got := DumpsIndent(v, 1); got != want {
		t.Errorf("indent=1:\n%s\nquiero:\n%s", got, want)
	}
	v, _ = Parse([]byte(`{"a":[1,2.50],"b":null}`))
	if got := Dumps(v); got != `{"a": [1, 2.5], "b": null}` {
		t.Errorf("Dumps = %s", got)
	}
}

// El faq.json real del volumen (escrito por Python) debe salir byte a byte igual
// al releerlo y volcarlo: así el motor Go puede reescribirlo sin romper nada.
func TestFaqRealIdaYVuelta(t *testing.T) {
	data, err := os.ReadFile("../../testdata/faq.json")
	if err != nil {
		t.Skip("sin testdata/faq.json")
	}
	v, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := DumpsIndent(v, 1); got != string(data) {
		t.Fatalf("el volcado difiere del original (%d vs %d bytes)", len(got), len(data))
	}
}

func TestOrdenYSet(t *testing.T) {
	v, _ := Parse([]byte(`{"id":"x","vector":null,"texto":"t"}`))
	v.Set("vector", NewArray(NewFloat(0.5)))
	v.Set("nuevo", NewBool(true))
	if got := Dumps(v); got != `{"id": "x", "vector": [0.5], "texto": "t", "nuevo": true}` {
		t.Errorf("orden: %s", got)
	}
	if NewNull().Truthy() || NewArray().Truthy() || !NewString("a").Truthy() {
		t.Error("Truthy")
	}
	n, _ := Parse([]byte(`5`))
	if n.PyStr() != "5" {
		t.Error("PyStr int")
	}
}
