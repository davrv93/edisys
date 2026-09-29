package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// falsoGemini: un generateContent falso que responde según el modelo pedido.
func falsoGemini(f func(modelo string) (int, string)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelo := ""
		if i := strings.Index(r.URL.Path, "/models/"); i >= 0 {
			modelo = strings.TrimSuffix(r.URL.Path[i+len("/models/"):], ":generateContent")
		}
		st, cuerpo := f(modelo)
		w.WriteHeader(st)
		_, _ = w.Write([]byte(cuerpo))
	}))
}

func sobre(texto string) string {
	return fmt.Sprintf(`{"candidates":[{"content":{"parts":[{"text":%q}]}}]}`, texto)
}

func TestClasificarConPrimario(t *testing.T) {
	var vistos []string
	srv := falsoGemini(func(modelo string) (int, string) {
		vistos = append(vistos, modelo)
		return 200, sobre(`{"intencion":"saldo"}`)
	})
	defer srv.Close()
	c := Clasificador{Clave: "k", Modelo: "m1", ModeloRespaldo: "m2", HTTP: srv.Client(), Timeout: 5 * time.Second, BaseURL: srv.URL}
	in, intentos := c.Clasificar(context.Background(), "cuanto debo este mes")
	if in != "saldo" || len(intentos) != 1 || intentos[0].Modelo != "m1" || intentos[0].Fallo != "" {
		t.Fatalf("primario: %q %v", in, intentos)
	}
}

func TestClasificarCaeAlRespaldo(t *testing.T) {
	srv := falsoGemini(func(modelo string) (int, string) {
		if modelo == "m1" {
			return 500, "mal"
		}
		return 200, sobre("pagar")
	})
	defer srv.Close()
	c := Clasificador{Clave: "k", Modelo: "m1", ModeloRespaldo: "m2", HTTP: srv.Client(), Timeout: 5 * time.Second, BaseURL: srv.URL}
	in, intentos := c.Clasificar(context.Background(), "como pago")
	if in != "pagar" || len(intentos) != 2 || intentos[0].Fallo == "" || intentos[1].Fallo != "" {
		t.Fatalf("respaldo: %q %v", in, intentos)
	}
}

func TestClasificarRechazaInventos(t *testing.T) {
	srv := falsoGemini(func(modelo string) (int, string) {
		if modelo == "m1" {
			return 200, sobre(`{"intencion":"clima"}`)
		}
		return 200, sobre(`{"intencion":"ninguna"}`)
	})
	defer srv.Close()
	c := Clasificador{Clave: "k", Modelo: "m1", ModeloRespaldo: "m2", HTTP: srv.Client(), Timeout: 5 * time.Second, BaseURL: srv.URL}
	in, intentos := c.Clasificar(context.Background(), "hola")
	if in != "" || len(intentos) != 2 || !strings.Contains(intentos[0].Fallo, "invalida") {
		t.Fatalf("inventos: %q %v", in, intentos)
	}
}

func TestClasificarSinClaveNoSale(t *testing.T) {
	c := Clasificador{}
	in, intentos := c.Clasificar(context.Background(), "hola")
	if in != "" || intentos != nil {
		t.Fatalf("sin clave: %q %v", in, intentos)
	}
}
