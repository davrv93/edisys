package mensajeria

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"edisys/api/internal/whatsapp"
)

const tokenPrueba = "123456:ABC-token-secreto"

// botFalso imita la Bot API: guarda lo que recibe y contesta como Telegram.
func botFalso(t *testing.T, responder func(w http.ResponseWriter, metodo string, cuerpo map[string]any)) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var recibidos []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pref := "/bot" + tokenPrueba + "/"
		if !strings.HasPrefix(r.URL.Path, pref) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":404,"description":"Not Found"}`))
			return
		}
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		c["_metodo"] = strings.TrimPrefix(r.URL.Path, pref)
		recibidos = append(recibidos, c)
		responder(w, c["_metodo"].(string), c)
	}))
	t.Cleanup(srv.Close)
	return srv, &recibidos
}

func TestTelegramEnviar(t *testing.T) {
	srv, rec := botFalso(t, func(w http.ResponseWriter, _ string, _ map[string]any) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":77,"chat":{"id":-100123}}}`))
	})
	var c Canal = Telegram{Token: tokenPrueba, URL: srv.URL}
	id, err := c.Enviar(context.Background(), "-100123", "Corte de agua el sábado")
	if err != nil {
		t.Fatal(err)
	}
	if id != "77" {
		t.Fatalf("id del proveedor: %q", id)
	}
	if len(*rec) != 1 || (*rec)[0]["_metodo"] != "sendMessage" || (*rec)[0]["chat_id"] != "-100123" || (*rec)[0]["text"] != "Corte de agua el sábado" {
		t.Fatalf("cuerpo enviado: %v", *rec)
	}
	if c.Nombre() != "telegram" {
		t.Fatal("nombre del canal")
	}
}

func TestTelegramErrorNoFiltraToken(t *testing.T) {
	srv, _ := botFalso(t, func(w http.ResponseWriter, _ string, _ map[string]any) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`))
	})
	_, err := Telegram{Token: tokenPrueba, URL: srv.URL}.Enviar(context.Background(), "42", "hola")
	if err == nil || !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("error esperado de Telegram: %v", err)
	}
	// Servidor caído: el error de red trae la URL con el token; debe salir tachado.
	srv.Close()
	_, err = Telegram{Token: tokenPrueba, URL: srv.URL}.Enviar(context.Background(), "42", "hola")
	if err == nil {
		t.Fatal("esperaba error de red")
	}
	if strings.Contains(err.Error(), tokenPrueba) {
		t.Fatalf("el error filtra el token: %v", err)
	}
}

func TestTelegramYoYValidaciones(t *testing.T) {
	srv, _ := botFalso(t, func(w http.ResponseWriter, metodo string, _ map[string]any) {
		if metodo == "getMe" {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"username":"edisys_demo_bot"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	})
	u, err := Telegram{Token: tokenPrueba, URL: srv.URL}.Yo(context.Background())
	if err != nil || u != "edisys_demo_bot" {
		t.Fatalf("getMe: %q %v", u, err)
	}
	if _, err := (Telegram{URL: srv.URL}).Enviar(context.Background(), "1", "x"); err == nil {
		t.Fatal("sin token no debe enviar")
	}
	if _, err := (Telegram{Token: tokenPrueba, URL: srv.URL}).Enviar(context.Background(), " ", "x"); err == nil {
		t.Fatal("sin chat_id no debe enviar")
	}
}

func TestSimuladoNoEnvia(t *testing.T) {
	var c Canal = Simulado{Canal: "telegram"}
	if _, err := c.Enviar(context.Background(), "1", "x"); !errors.Is(err, ErrSimulado) {
		t.Fatalf("simulado debe devolver ErrSimulado: %v", err)
	}
}

func TestWhatsAppAdaptador(t *testing.T) {
	var ruta, numero string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta = r.URL.Path
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		numero, _ = c["number"].(string)
		_, _ = w.Write([]byte(`{"key":{"id":"ABC"}}`))
	}))
	defer srv.Close()
	var c Canal = WhatsApp{Evolution: whatsapp.Evolution{URL: srv.URL, Instancia: "demo", APIKey: "k"}}
	id, err := c.Enviar(context.Background(), "987 654 321", "hola")
	if err != nil || id != "ABC" || ruta != "/message/sendText/demo" || numero != "51987654321" {
		t.Fatalf("adaptador WhatsApp: id=%q err=%v ruta=%q numero=%q", id, err, ruta, numero)
	}
}

func TestChatIDValido(t *testing.T) {
	for _, s := range []string{"123", "-1001234567890", "@edificio_demo"} {
		if !ChatIDValido(s) {
			t.Errorf("%q debería valer", s)
		}
	}
	for _, s := range []string{"", "-", "abc", "@ab", "12 3", "@con espacio"} {
		if ChatIDValido(s) {
			t.Errorf("%q no debería valer", s)
		}
	}
}
