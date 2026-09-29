package e5

import (
	"encoding/json"
	"os"
	"testing"
)

// 3 126 textos (golden, benchmark, FAQ, casos límite y 3 000 cadenas Unicode
// aleatorias) tokenizados por tokenizers 0.20 de Python con truncado a 128:
// el tokenizador en Go tiene que dar exactamente los mismos ids.
func TestUnigramIgualQueHF(t *testing.T) {
	ruta := os.Getenv("E5_TOK")
	if ruta == "" {
		t.Skip("E5_TOK no definido")
	}
	u, err := CargarUnigram(ruta, MaxTokens)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../testdata/tokens_ref.json")
	if err != nil {
		t.Fatal(err)
	}
	var refs []struct {
		T   string   `json:"t"`
		IDs []uint32 `json:"ids"`
	}
	if err := json.Unmarshal(data, &refs); err != nil {
		t.Fatal(err)
	}
	fallos := 0
	for _, r := range refs {
		got := u.Encode(r.T)
		igual := len(got) == len(r.IDs)
		for i := 0; igual && i < len(got); i++ {
			igual = got[i] == r.IDs[i]
		}
		if !igual {
			fallos++
			if fallos <= 15 {
				t.Errorf("%q:\n  Go: %v\n  HF: %v", r.T, got, r.IDs)
			}
		}
	}
	t.Logf("%d/%d textos idénticos", len(refs)-fallos, len(refs))
}
