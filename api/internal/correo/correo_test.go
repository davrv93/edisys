package correo

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"
)

// servidorSMTP es un SMTP mínimo en memoria: acepta un mensaje y lo devuelve por el canal.
func servidorSMTP(t *testing.T) (string, <-chan []byte) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan []byte, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		defer ln.Close()
		r := bufio.NewReader(c)
		w := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		w("220 prueba")
		for {
			l, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(l))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				w("250 hola")
			case strings.HasPrefix(cmd, "DATA"):
				w("354 dale")
				var b bytes.Buffer
				for {
					l, err := r.ReadString('\n')
					if err != nil || l == ".\r\n" {
						break
					}
					b.WriteString(l)
				}
				ch <- b.Bytes()
				w("250 ok")
			case strings.HasPrefix(cmd, "QUIT"):
				w("221 chao")
				return
			default:
				w("250 ok")
			}
		}
	}()
	return ln.Addr().String(), ch
}

func TestEnviarConAdjunto(t *testing.T) {
	addr, ch := servidorSMTP(t)
	host, puerto, _ := net.SplitHostPort(addr)
	pdf := []byte("%PDF-1.4 prueba con ñ")
	m := Mensaje{De: "EDISYS <no-responder@edisys.local>", Para: "María Demo <propietario201@demo.pe>", Asunto: "Tu recibo de setiembre 2026",
		HTML: "<p>Hola, María</p>", Texto: "Hola, María", Adjuntos: []Adjunto{{Nombre: "recibo.pdf", Tipo: "application/pdf", Datos: pdf}}}
	if err := (SMTP{Host: host, Puerto: puerto}).Enviar(m); err != nil {
		t.Fatal(err)
	}
	var crudo []byte
	select {
	case crudo = <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("el servidor no recibió nada")
	}
	msg, err := mail.ReadMessage(bytes.NewReader(crudo))
	if err != nil {
		t.Fatal(err)
	}
	asunto, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if asunto != "Tu recibo de setiembre 2026" {
		t.Errorf("asunto %q", asunto)
	}
	_, ps, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	mr := multipart.NewReader(msg.Body, ps["boundary"])
	var hallado bool
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if p.FileName() == "recibo.pdf" {
			b, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, p))
			hallado = bytes.Equal(b, pdf)
		}
	}
	if !hallado {
		t.Errorf("el adjunto no llegó íntegro:\n%s", crudo)
	}
}

func TestValidarDireccion(t *testing.T) {
	if !ValidarDireccion("a@b.pe") || ValidarDireccion("") || ValidarDireccion("sin-arroba") {
		t.Error("ValidarDireccion")
	}
}
