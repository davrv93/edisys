// Package correo arma mensajes MIME (HTML + texto + adjuntos) y los envía por SMTP.
// En local el SMTP es Mailpit (http://localhost:4726): nada sale a terceros.
package correo

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Adjunto de un mensaje.
type Adjunto struct {
	Nombre string
	Tipo   string
	Datos  []byte
}

// Mensaje a enviar.
type Mensaje struct {
	De       string // «Nombre <correo>»
	Para     string // un destinatario
	Asunto   string
	HTML     string
	Texto    string
	Adjuntos []Adjunto
}

// SMTP es la conexión al servidor de correo. Sin usuario no autentica (Mailpit).
type SMTP struct {
	Host    string
	Puerto  string
	Usuario string
	Clave   string
	Timeout time.Duration
}

// ValidarDireccion comprueba un correo.
func ValidarDireccion(d string) bool {
	a, err := mail.ParseAddress(strings.TrimSpace(d))
	return err == nil && strings.Contains(a.Address, "@")
}

func frontera() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "edisys-" + hex.EncodeToString(b)
}

// Armar devuelve el mensaje en formato RFC 5322 con MIME (multipart/mixed → alternative + adjuntos).
func Armar(m Mensaje, ahora time.Time) []byte {
	var b bytes.Buffer
	h := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	h("From", encabezadoDireccion(m.De))
	h("To", encabezadoDireccion(m.Para))
	h("Subject", mime.QEncoding.Encode("utf-8", m.Asunto))
	h("Date", ahora.Format(time.RFC1123Z))
	h("Message-ID", "<"+frontera()+"@edisys.local>")
	h("MIME-Version", "1.0")
	mixta := frontera()
	alt := frontera()
	h("Content-Type", `multipart/mixed; boundary="`+mixta+`"`)
	b.WriteString("\r\n")
	fmt.Fprintf(&b, "--%s\r\nContent-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", mixta, alt)
	parte := func(tipo, cuerpo string) {
		fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", alt, tipo)
		w := quotedprintable.NewWriter(&b)
		_, _ = w.Write([]byte(cuerpo))
		_ = w.Close()
		b.WriteString("\r\n")
	}
	texto := m.Texto
	if texto == "" {
		texto = "Este mensaje se ve mejor en un cliente de correo con HTML."
	}
	parte("text/plain", texto)
	if m.HTML != "" {
		parte("text/html", m.HTML)
	}
	fmt.Fprintf(&b, "--%s--\r\n", alt)
	for _, a := range m.Adjuntos {
		tipo := a.Tipo
		if tipo == "" {
			tipo = "application/octet-stream"
		}
		nombre := mime.QEncoding.Encode("utf-8", a.Nombre)
		fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; name=\"%s\"\r\nContent-Disposition: attachment; filename=\"%s\"\r\nContent-Transfer-Encoding: base64\r\n\r\n",
			mixta, tipo, nombre, nombre)
		enc := base64.StdEncoding.EncodeToString(a.Datos)
		for len(enc) > 76 {
			b.WriteString(enc[:76] + "\r\n")
			enc = enc[76:]
		}
		b.WriteString(enc + "\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", mixta)
	return b.Bytes()
}

func encabezadoDireccion(d string) string {
	a, err := mail.ParseAddress(d)
	if err != nil {
		return d
	}
	if a.Name == "" {
		return a.Address
	}
	return (&mail.Address{Name: a.Name, Address: a.Address}).String()
}

func direccion(d string) string {
	if a, err := mail.ParseAddress(d); err == nil {
		return a.Address
	}
	return strings.TrimSpace(d)
}

// Enviar entrega el mensaje por SMTP (STARTTLS si el servidor lo ofrece y hay credenciales).
func (s SMTP) Enviar(m Mensaje) error {
	if s.Host == "" {
		return errors.New("SMTP sin host")
	}
	to := s.Timeout
	if to == 0 {
		to = 15 * time.Second
	}
	addr := net.JoinHostPort(s.Host, s.Puerto)
	conn, err := net.DialTimeout("tcp", addr, to)
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(2 * to))
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if s.Usuario != "" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(nil); err != nil {
				return err
			}
		}
		if err := c.Auth(smtp.PlainAuth("", s.Usuario, s.Clave, s.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(direccion(m.De)); err != nil {
		return err
	}
	if err := c.Rcpt(direccion(m.Para)); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(Armar(m, time.Now())); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
