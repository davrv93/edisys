// Package llm clasifica mensajes del chatbot con Gemini, con la misma forma que
// PjgFactSalud (Gerencia Bot): las reglas mandan y el modelo solo clasifica lo que
// nadie entiende. Garantías contra loops y vacíos:
//   - una sola pasada por mensaje: un intento por modelo (principal y respaldo), sin
//     reintentos ni recursión;
//   - timeout por intento (LLM_TIMEOUT_SEG, 8 s);
//   - el modelo solo devuelve una intención del conjunto cerrado; cualquier otra
//     cosa (o "ninguna") es no-entendido y cae al menú de siempre;
//   - sin clave, apagado: Clasificar devuelve ok=false sin salir a la red.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Modelos free con mayor cuota (verificados 2026-09 en la doc de Gemini API):
// gemini-2.5-flash-lite (15 RPM / 1000 RPD) y gemini-3.1-flash-lite (el de PjgFactSalud).
// Van por entorno (LLM_MODELO, LLM_MODELO_FALLBACK); aquí solo los defectos.
const (
	ModeloDefecto  = "gemini-2.5-flash-lite"
	ModeloRespaldo = "gemini-3.1-flash-lite"
)

// Intenciones que el modelo puede devolver. Deben existir en el switch del bot;
// "ninguna" significa "entendido que no es nada" (también cae al menú, sin reintentar).
var Intenciones = []string{
	"saludo", "menu", "saldo", "ultimo_recibo", "pagar", "reservar",
	"reportar_incidencia", "horarios", "hablar_admin", "ninguna",
}

// Intento: lo que pasó con un modelo (uno por llamada como máximo dos).
type Intento struct {
	Modelo string
	Fallo  string // "" = el modelo contestó bien (aunque sea "ninguna")
}

// Clasificador llama a generateContent de Gemini.
type Clasificador struct {
	Clave          string
	ClaveRespaldo  string
	Modelo         string
	ModeloRespaldo string
	HTTP           *http.Client
	Timeout        time.Duration
	// BaseURL solo existe para probar con un servidor falso; en producción es Google.
	BaseURL string
}

func (c Clasificador) base() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return "https://generativelanguage.googleapis.com"
}

func (c Clasificador) modelos() []string {
	prin, resp := c.Modelo, c.ModeloRespaldo
	if prin == "" {
		prin = ModeloDefecto
	}
	if resp == "" {
		resp = ModeloRespaldo
	}
	if resp == "" || resp == prin {
		return []string{prin}
	}
	return []string{prin, resp}
}

func promptClasificador(texto string) string {
	return "Eres el clasificador del asistente de WhatsApp de una administración de edificios en Perú.\n" +
		"Clasifica el mensaje del propietario en UNA de estas intenciones:\n" +
		"saludo · menu · saldo (cuánto debe) · ultimo_recibo · pagar (cómo pagar) · reservar (áreas comunes) · " +
		"reportar_incidencia · horarios (áreas y normas) · hablar_admin · ninguna\n" +
		"Responde SOLO este JSON: {\"intencion\":\"...\"}\n" +
		"Mensaje: «" + texto + "»"
}

// Clasificar devuelve la intención ("" si ninguna o si no se pudo) y un Intento
// por modelo probado. Nunca falla: sin clave o con error, ("", intentos).
func (c Clasificador) Clasificar(ctx context.Context, texto string) (string, []Intento) {
	texto = strings.TrimSpace(texto)
	if c.Clave == "" || texto == "" {
		return "", nil
	}
	modelos := c.modelos()
	var intentos []Intento
	for i, m := range modelos {
		clave := c.Clave
		if i > 0 && c.ClaveRespaldo != "" {
			clave = c.ClaveRespaldo
		}
		in, fallo := c.intento(ctx, m, clave, texto)
		intentos = append(intentos, Intento{Modelo: m, Fallo: fallo})
		if fallo != "" {
			continue
		}
		if in == "" || in == "ninguna" {
			return "", intentos
		}
		for _, v := range Intenciones {
			if in == v {
				return in, intentos
			}
		}
		// Intención inventada: se trata como fallo y se prueba el respaldo.
		intentos[len(intentos)-1].Fallo = "intencion_invalida:" + in
	}
	return "", intentos
}

func (c Clasificador) intento(ctx context.Context, modelo, clave, texto string) (string, string) {
	cli := c.HTTP
	if cli == nil {
		cli = &http.Client{Timeout: 30 * time.Second}
	}
	to := c.Timeout
	if to <= 0 {
		to = 8 * time.Second
	}
	cta, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	cuerpo, _ := json.Marshal(map[string]any{
		"contents":         []any{map[string]any{"parts": []any{map[string]any{"text": promptClasificador(texto)}}}},
		"generationConfig": map[string]any{"maxOutputTokens": 120, "temperature": 0},
	})
	url := c.base() + "/v1beta/models/" + modelo + ":generateContent?key=" + clave
	peticion, err := http.NewRequestWithContext(cta, http.MethodPost, url, bytes.NewReader(cuerpo))
	if err != nil {
		return "", "peticion:" + err.Error()
	}
	peticion.Header.Set("Content-Type", "application/json")
	res, err := cli.Do(peticion)
	if err != nil {
		return "", "red:" + err.Error()
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Sprintf("http_%d", res.StatusCode)
	}
	var fuera struct {
		Candidatos []struct {
			Contenido struct {
				Partes []struct {
					Texto string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(b, &fuera); err != nil || len(fuera.Candidatos) == 0 || len(fuera.Candidatos[0].Contenido.Partes) == 0 {
		return "", "respuesta_invalida"
	}
	crudo := strings.TrimSpace(fuera.Candidatos[0].Contenido.Partes[0].Texto)
	crudo = strings.TrimPrefix(strings.TrimSuffix(crudo, "```"), "```json")
	crudo = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(crudo), "```"), "```")
	var j struct {
		Intencion string `json:"intencion"`
	}
	if err := json.Unmarshal([]byte(crudo), &j); err != nil {
		// Acepta la intención pelada ("saldo") sin JSON.
		j.Intencion = strings.Trim(strings.ToLower(crudo), "\"' \n")
	}
	return strings.TrimSpace(j.Intencion), ""
}
