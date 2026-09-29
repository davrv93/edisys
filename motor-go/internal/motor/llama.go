package motor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"edisys/motor-go/internal/pyjson"
)

// Tiempos de app.py: CHAT_TIMEOUT = 120 s; /health con timeout 5 s.
var (
	ChatTimeout  = 120 * time.Second
	SaludTimeout = 5 * time.Second
)

type salidaHTTP struct{ chat, salud *http.Client }

func nuevoCliente() *salidaHTTP {
	return &salidaHTTP{
		chat:  &http.Client{Timeout: ChatTimeout},
		salud: &http.Client{Timeout: SaludTimeout},
	}
}

type mensajeLlama struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// llama = _llama: POST /v1/chat/completions (OpenAI-compatible) y devuelve el
// contenido sin espacios en los bordes y usage.completion_tokens (o null).
func (m *Motor) llama(mensajes []Mensaje, maxTokens int, temperatura float64) (string, *pyjson.Value, error) {
	ms := make([]mensajeLlama, len(mensajes))
	for i, x := range mensajes {
		ms[i] = mensajeLlama{x.Role, x.Content}
	}
	cuerpo, _ := json.Marshal(struct {
		Messages    []mensajeLlama `json:"messages"`
		Temperature float64        `json:"temperature"`
		MaxTokens   int            `json:"max_tokens"`
		CachePrompt bool           `json:"cache_prompt"`
	}{ms, temperatura, maxTokens, true})
	resp, err := m.Cliente.chat.Post(m.LlamaURL+"/v1/chat/completions", "application/json", bytes.NewReader(cuerpo))
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	datos, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { // urllib levanta HTTPError
		return "", nil, fmt.Errorf("HTTP Error %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	v, err := pyjson.Parse(datos)
	if err != nil {
		return "", nil, err
	}
	ch := v.Get("choices")
	if ch == nil || ch.Kind != pyjson.Array || len(ch.Arr) == 0 {
		return "", nil, errors.New("respuesta sin choices")
	}
	contenido := ch.Arr[0].Get("message").Get("content")
	if contenido == nil || contenido.Kind != pyjson.String {
		return "", nil, errors.New("respuesta sin message.content")
	}
	tokens := pyjson.NewNull()
	if t := v.Get("usage").Get("completion_tokens"); t != nil {
		tokens = t
	}
	return pyStrip(contenido.S), tokens, nil
}

// SaludLlama = el bloque "llama" de /v1/salud.
func (m *Motor) SaludLlama() *pyjson.Value {
	o := pyjson.NewObject()
	resp, err := m.Cliente.salud.Get(m.LlamaURL + "/health")
	if err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		switch {
		case resp.StatusCode == 200:
			o.Set("estado", pyjson.NewString("ok"))
			return o
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			o.Set("estado", pyjson.NewString(fmt.Sprintf("http %d", resp.StatusCode)))
			return o
		}
		err = fmt.Errorf("HTTP Error %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	o.Set("estado", pyjson.NewString("caido"))
	o.Set("error", pyjson.NewString(cortarRunas(err.Error(), 120)))
	return o
}
