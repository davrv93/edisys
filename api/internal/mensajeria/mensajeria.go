// Package mensajeria es la interfaz común de los canales de mensajes salientes (bloque E4).
// Cada canal (WhatsApp por Evolution, Telegram por la Bot API) se adapta a Canal, así la
// bandeja y los anuncios no saben con quién hablan. Por defecto todo es «simulado»: se
// registra el mensaje y NO sale nada; el envío real lo enciende un interruptor del servidor.
package mensajeria

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"edisys/api/internal/whatsapp"
)

// Canal envía un texto a un destino (teléfono, chat_id…) y devuelve el id del proveedor.
type Canal interface {
	Nombre() string
	Enviar(ctx context.Context, destino, texto string) (string, error)
}

// ErrSimulado no es un fallo: avisa que el canal está en simulado y el mensaje no salió.
var ErrSimulado = errors.New("canal en modo simulado: no se envió nada")

// Simulado es el canal que no envía nada. La bandeja marca el mensaje como «simulado».
type Simulado struct{ Canal string }

func (s Simulado) Nombre() string { return s.Canal }

func (s Simulado) Enviar(context.Context, string, string) (string, error) { return "", ErrSimulado }

// WhatsApp adapta el cliente de Evolution a la interfaz común.
type WhatsApp struct{ Evolution whatsapp.Evolution }

func (w WhatsApp) Nombre() string { return "whatsapp" }

func (w WhatsApp) Enviar(ctx context.Context, telefono, texto string) (string, error) {
	return w.Evolution.Enviar(ctx, whatsapp.NormalizarTelefono(telefono), texto)
}

// TelegramURL es la Bot API pública; las pruebas la cambian por un httptest.
const TelegramURL = "https://api.telegram.org"

// Telegram es el cliente mínimo de la Bot API (sendMessage y getMe). El token va en la ruta
// (/bot<token>/…), así que NUNCA aparece en un error: se tacha antes de devolverlo.
type Telegram struct {
	Token string
	URL   string // vacío = TelegramURL
	HTTP  *http.Client
}

func (t Telegram) Nombre() string { return "telegram" }

// respuestaTG es el sobre común de la Bot API: {ok, result, description}.
type respuestaTG struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

// llamar hace POST {url}/bot{token}/{metodo} con cuerpo JSON y devuelve result.
func (t Telegram) llamar(ctx context.Context, metodo string, cuerpo any) (json.RawMessage, error) {
	if strings.TrimSpace(t.Token) == "" {
		return nil, errors.New("falta el token del bot de Telegram")
	}
	base := strings.TrimRight(t.URL, "/")
	if base == "" {
		base = TelegramURL
	}
	b, _ := json.Marshal(cuerpo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/bot"+t.Token+"/"+metodo, bytes.NewReader(b))
	if err != nil {
		return nil, t.tachar(err)
	}
	req.Header.Set("Content-Type", "application/json")
	cli := t.HTTP
	if cli == nil {
		cli = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := cli.Do(req)
	if err != nil {
		return nil, t.tachar(err)
	}
	defer res.Body.Close()
	datos, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	var r respuestaTG
	_ = json.Unmarshal(datos, &r)
	if res.StatusCode >= 300 || !r.OK {
		desc := strings.TrimSpace(r.Description)
		if desc == "" {
			desc = strings.TrimSpace(string(datos))
		}
		return nil, t.tachar(fmt.Errorf("telegram respondió %d: %s", res.StatusCode, desc))
	}
	return r.Result, nil
}

// tachar quita el token de cualquier texto de error (url.Error imprime la URL completa).
func (t Telegram) tachar(err error) error {
	if err == nil || t.Token == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), t.Token, "***"))
}

// Enviar manda un texto a un chat (usuario o grupo). El destino es el chat_id numérico o @canal.
func (t Telegram) Enviar(ctx context.Context, chatID, texto string) (string, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return "", errors.New("falta el chat_id de Telegram")
	}
	res, err := t.llamar(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": texto, "disable_web_page_preview": true})
	if err != nil {
		return "", err
	}
	var m struct {
		MessageID int64 `json:"message_id"`
	}
	_ = json.Unmarshal(res, &m)
	return fmt.Sprint(m.MessageID), nil
}

// Yo pregunta a Telegram quién es el bot (getMe): sirve para validar el alta del bot.
func (t Telegram) Yo(ctx context.Context) (string, error) {
	res, err := t.llamar(ctx, "getMe", map[string]any{})
	if err != nil {
		return "", err
	}
	var u struct {
		Username string `json:"username"`
	}
	_ = json.Unmarshal(res, &u)
	return u.Username, nil
}

// ChatIDValido acepta un id numérico (los grupos son negativos) o un @canal público.
func ChatIDValido(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 64 {
		return false
	}
	if strings.HasPrefix(s, "@") {
		for _, c := range s[1:] {
			if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
				return false
			}
		}
		return len(s) >= 6
	}
	if strings.HasPrefix(s, "-") {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
