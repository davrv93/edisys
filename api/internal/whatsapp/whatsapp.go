// Package whatsapp: plantillas de mensajes y cliente de Evolution API.
// Por defecto el modo es «simulado»: se registra el mensaje y NO sale nada.
package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Plantillas disponibles. Las variables van entre {{ }}.
var Plantillas = map[string]string{
	"recibo":                 "Hola {{nombre}}, tu recibo de {{periodo}} del Dpto {{unidad}} es de {{total}} y vence el {{vence}}. {{estado}} Revísalo en {{enlace}}",
	"recordatorio_deuda":     "Hola {{nombre}}, el Dpto {{unidad}} tiene un saldo pendiente de {{saldo}}. Puedes pagar por Yape al {{yape}} o por transferencia y enviarnos el voucher desde la app. Si ya pagaste, no tomes en cuenta este mensaje. ¡Gracias!",
	"reserva_confirmada":     "Hola {{nombre}}, tu reserva {{codigo}} de {{area}} para el {{fecha}} está confirmada. Recuerda las normas del área. ¡Que la disfrutes!",
	"incidencia_actualizada": "Hola {{nombre}}, tu reporte {{codigo}} ({{titulo}}) ahora está: {{estado}}.",
	"aviso_general":          "{{mensaje}}",
	"libre":                  "{{texto}}",
	"chatbot":                "{{texto}}",
}

var reVar = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)

// Variables devuelve las variables que pide una plantilla.
func Variables(plantilla string) []string {
	vistos := map[string]bool{}
	var out []string
	for _, m := range reVar.FindAllStringSubmatch(Plantillas[plantilla], -1) {
		if !vistos[m[1]] {
			vistos[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

// Renderizar llena la plantilla. Devuelve las variables que faltan.
func Renderizar(plantilla string, vars map[string]string) (string, []string) {
	tpl, ok := Plantillas[plantilla]
	if !ok {
		return "", []string{"plantilla"}
	}
	var faltan []string
	out := reVar.ReplaceAllStringFunc(tpl, func(m string) string {
		k := reVar.FindStringSubmatch(m)[1]
		v, ok := vars[k]
		if !ok || strings.TrimSpace(v) == "" {
			faltan = append(faltan, k)
			return m
		}
		return v
	})
	sort.Strings(faltan)
	return strings.Join(strings.Fields(out), " "), faltan
}

// NormalizarTelefono deja solo dígitos y antepone 51 a un celular peruano de 9 dígitos.
func NormalizarTelefono(t string) string {
	var b strings.Builder
	for _, r := range t {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) == 9 && strings.HasPrefix(s, "9") {
		s = "51" + s
	}
	return s
}

// Evolution es el cliente de Evolution API (evolution-go o la versión Node).
type Evolution struct {
	URL, Instancia, APIKey string
	HTTP                   *http.Client
}

// Enviar hace POST {url}/message/sendText/{instancia} con la cabecera apikey.
func (e Evolution) Enviar(ctx context.Context, telefono, texto string) (string, error) {
	if e.URL == "" || e.Instancia == "" {
		return "", fmt.Errorf("falta la URL o la instancia de Evolution")
	}
	cuerpo, _ := json.Marshal(map[string]any{"number": telefono, "text": texto})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(e.URL, "/")+"/message/sendText/"+e.Instancia, bytes.NewReader(cuerpo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", e.APIKey)
	cli := e.HTTP
	if cli == nil {
		cli = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	datos, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("evolution respondió %d: %s", res.StatusCode, strings.TrimSpace(string(datos)))
	}
	var r struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	_ = json.Unmarshal(datos, &r)
	return r.Key.ID, nil
}

// Entrante es lo que interesa del webhook messages.upsert de Evolution.
type Entrante struct {
	Telefono string
	Texto    string
	Nombre   string
	DeMi     bool
}

// LeerWebhook interpreta el cuerpo del webhook (formato Evolution v2). Devuelve ok=false si no es texto entrante.
func LeerWebhook(cuerpo []byte) (Entrante, bool) {
	var p struct {
		Event string `json:"event"`
		Data  struct {
			Key struct {
				RemoteJid string `json:"remoteJid"`
				FromMe    bool   `json:"fromMe"`
			} `json:"key"`
			PushName string `json:"pushName"`
			Message  struct {
				Conversation        string `json:"conversation"`
				ExtendedTextMessage struct {
					Text string `json:"text"`
				} `json:"extendedTextMessage"`
			} `json:"message"`
		} `json:"data"`
		// Forma simple para pruebas: {"telefono": "...", "texto": "..."}
		Telefono string `json:"telefono"`
		Texto    string `json:"texto"`
	}
	if err := json.Unmarshal(cuerpo, &p); err != nil {
		return Entrante{}, false
	}
	if p.Telefono != "" && p.Texto != "" {
		return Entrante{Telefono: NormalizarTelefono(p.Telefono), Texto: p.Texto}, true
	}
	jid := p.Data.Key.RemoteJid
	if jid == "" || strings.HasSuffix(jid, "@g.us") {
		return Entrante{}, false // grupos no
	}
	texto := p.Data.Message.Conversation
	if texto == "" {
		texto = p.Data.Message.ExtendedTextMessage.Text
	}
	if texto == "" {
		return Entrante{}, false
	}
	tel := jid
	if i := strings.IndexByte(tel, '@'); i >= 0 {
		tel = tel[:i]
	}
	if i := strings.IndexByte(tel, ':'); i >= 0 {
		tel = tel[:i]
	}
	return Entrante{Telefono: NormalizarTelefono(tel), Texto: texto, Nombre: p.Data.PushName, DeMi: p.Data.Key.FromMe}, true
}
