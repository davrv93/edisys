package app_test

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"testing"
)

// Bloque E1 · documentos por categorías.
func TestDocumentos(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")

	st, d := e.pedir("POST", "/api/v1/edificios/1/documentos/categorias", tok, map[string]any{"nombre": "Actas de junta"})
	if st != 201 {
		t.Fatalf("crear categoría: %d %v", st, d)
	}
	catID := int64(d["id"].(float64))

	// Subir un documento.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("categoria_id", fmt.Sprint(catID))
	_ = mw.WriteField("titulo", "Acta de setiembre")
	_ = mw.WriteField("numero", "2026-09")
	fw, _ := mw.CreateFormFile("archivo", "acta.pdf")
	_, _ = fw.Write([]byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF"))
	mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/edificios/1/documentos", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	st, d = e.hacer(req)
	if st != 201 {
		t.Fatalf("crear documento: %d %v", st, d)
	}
	docID := int64(d["id"].(float64))

	if st, l := e.pedir("GET", "/api/v1/edificios/1/documentos", tok, nil); st != 200 || len(l["datos"].([]any)) < 1 {
		t.Fatalf("listar como admin: %d %v", st, l)
	}

	// Despublicar y comprobar que el propietario no lo ve.
	if st, _ := e.pedir("POST", fmt.Sprintf("/api/v1/edificios/1/documentos/%d/publicar", docID), tok, map[string]any{"publicado": false}); st != 200 {
		t.Fatalf("despublicar: %d", st)
	}
	prop := e.login("propietario201@demo.pe")
	_, lp := e.pedir("GET", "/api/v1/edificios/1/documentos", prop, nil)
	for _, it := range lp["datos"].([]any) {
		if int64(it.(map[string]any)["id"].(float64)) == docID {
			t.Fatal("el propietario no debería ver un documento despublicado")
		}
	}
}
