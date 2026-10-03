package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"edisys/api/internal/seed"
)

// Bloque I3 · dominio propio: el API resuelve la administradora por Host, el «ask» de Caddy
// solo aprueba hosts registrados y en un dominio propio no entra un usuario de otra administradora.
func TestDominioPropio(t *testing.T) {
	e := nuevo(t)
	adm := e.login("admin@demo.pe")
	base := "/api/v1/edificios/1/dominios"

	if st, _ := e.pedir("POST", base, adm, map[string]any{"host": "no es un host"}); st != 422 {
		t.Fatalf("host inválido: %d", st)
	}
	st, d := e.pedir("POST", base, adm, map[string]any{"host": "https://Intranet.Demo-Adm.PE:443/"})
	if st != 201 || d["host"] != "intranet.demo-adm.pe" {
		t.Fatalf("crear: %d %v", st, d)
	}
	id := int64(d["id"].(float64))
	if st, _ := e.pedir("POST", base, adm, map[string]any{"host": "intranet.demo-adm.pe"}); st != 409 {
		t.Fatalf("duplicado: %d", st)
	}
	if st, _ := e.pedir("GET", base, e.login("junta@demo.pe"), nil); st != 403 {
		t.Fatalf("junta administra dominios: %d", st)
	}

	// Resolución pública por Host (y por X-Forwarded-Host, que es lo que pone Caddy).
	conHost := func(metodo, ruta, host string, cuerpo any) (int, map[string]any) {
		var b []byte
		if cuerpo != nil {
			b, _ = json.Marshal(cuerpo)
		}
		req, _ := http.NewRequest(metodo, e.srv.URL+ruta, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Host = host
		return e.hacer(req)
	}
	st, d = conHost("GET", "/api/v1/publico/dominio", "intranet.demo-adm.pe", nil)
	if st != 200 || d["propio"] != true || d["administradora_id"] == nil {
		t.Fatalf("resolver por host: %d %v", st, d)
	}
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/publico/dominio", nil)
	req.Header.Set("X-Forwarded-Host", "INTRANET.demo-adm.pe")
	if st, d := e.hacer(req); st != 200 || d["propio"] != true {
		t.Fatalf("resolver por X-Forwarded-Host: %d %v", st, d)
	}
	if _, d := conHost("GET", "/api/v1/publico/dominio", "otro.pe", nil); d["propio"] != false {
		t.Fatalf("host ajeno: %v", d)
	}
	if st, _ := e.pedir("GET", "/api/v1/publico/dominio-permitido?domain=intranet.demo-adm.pe", "", nil); st != 200 {
		t.Fatalf("ask registrado: %d", st)
	}
	if st, _ := e.pedir("GET", "/api/v1/publico/dominio-permitido?domain=malicioso.pe", "", nil); st != 404 {
		t.Fatalf("ask no registrado: %d", st)
	}

	// Login en el dominio propio: el de la administradora entra; uno de otra administradora no.
	login := map[string]string{"correo": "admin@demo.pe", "clave": seed.ClaveDemo}
	if st, d := conHost("POST", "/api/v1/auth/login", "intranet.demo-adm.pe", login); st != 200 {
		t.Fatalf("login propio: %d %v", st, d)
	}
	var otraAdm, otroUsr int64
	if err := e.pool.QueryRow(t.Context(), `INSERT INTO administradora (nombre) VALUES ('Otra SAC') RETURNING id`).Scan(&otraAdm); err != nil {
		t.Fatal(err)
	}
	var hash string
	_ = e.pool.QueryRow(t.Context(), `SELECT clave_hash FROM usuario WHERE correo='admin@demo.pe'`).Scan(&hash)
	correo := fmt.Sprintf("ajeno%d@otra.pe", otraAdm)
	if err := e.pool.QueryRow(t.Context(), `INSERT INTO usuario (administradora_id, correo, nombre, clave_hash) VALUES ($1,$2,'Ajeno',$3) RETURNING id`,
		otraAdm, correo, hash).Scan(&otroUsr); err != nil {
		t.Fatal(err)
	}
	ajeno := map[string]string{"correo": correo, "clave": seed.ClaveDemo}
	if st, d := conHost("POST", "/api/v1/auth/login", "intranet.demo-adm.pe", ajeno); st != 401 || codigo(d) != "CREDENCIALES" {
		t.Fatalf("login ajeno en dominio propio: %d %v", st, d)
	}
	// En el dominio de la plataforma el mismo usuario sí autentica.
	if st, d := conHost("POST", "/api/v1/auth/login", "edisys.local", ajeno); st != 200 {
		t.Fatalf("login ajeno en dominio plataforma: %d %v", st, d)
	}

	// Desactivar el dominio lo saca del «ask».
	if st, _ := e.pedir("PATCH", fmt.Sprintf("%s/%d", base, id), adm, map[string]any{"activo": false}); st != 200 {
		t.Fatalf("desactivar: %d", st)
	}
	if st, _ := e.pedir("GET", "/api/v1/publico/dominio-permitido?domain=intranet.demo-adm.pe", "", nil); st != 404 {
		t.Fatalf("ask desactivado: %d", st)
	}
	if st, _ := e.pedir("DELETE", fmt.Sprintf("%s/%d", base, id), adm, nil); st != 200 {
		t.Fatalf("borrar: %d", st)
	}
}
