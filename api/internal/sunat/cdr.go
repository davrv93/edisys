package sunat

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// CDR: constancia de recepción.
type CDR struct {
	Codigo      string `json:"codigo"` // "0" aceptado; 2000–3999 rechazado; 4000+ observaciones
	Descripcion string `json:"descripcion"`
	XML         string `json:"-"`
	Ticket      string `json:"ticket,omitempty"` // comunicaciones de baja
}

// Aceptado dice si el CDR acepta el comprobante (código 0 u observaciones 4000+).
func (c CDR) Aceptado() bool {
	if c.Codigo == "0" {
		return true
	}
	var n int
	fmt.Sscanf(c.Codigo, "%d", &n)
	return n >= 4000
}

var nombreTipo = map[string]string{Factura: "La Factura", Boleta: "La Boleta", NotaCredito: "La Nota de Credito"}

// CDRSimulado: lo que devolvería SUNAT al aceptar el comprobante. No sale nada a la red.
func CDRSimulado(ruc, tipo, id, hash string, ahora time.Time) CDR {
	desc := fmt.Sprintf("%s numero %s, ha sido aceptada", nombreTipo[tipo], id)
	if tipo == "RA" {
		desc = fmt.Sprintf("La Comunicacion de baja %s, ha sido aceptada", id)
	}
	w := &x{}
	w.b.WriteString(`<ar:ApplicationResponse xmlns:ar="urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2" xmlns:cac="` + NSCac + `" xmlns:cbc="` + NSCbc + `">`)
	w.e("cbc:UBLVersionID", "2.0")
	w.e("cbc:CustomizationID", "1.0")
	w.e("cbc:ID", fmt.Sprintf("SIM-%d", ahora.UnixNano()))
	w.e("cbc:IssueDate", ahora.Format("2006-01-02"))
	w.e("cbc:IssueTime", ahora.Format("15:04:05"))
	w.e("cbc:Note", "CDR SIMULADO por EDISYS: no se envió a SUNAT")
	w.abre("cac:SenderParty")
	w.abre("cac:PartyIdentification")
	w.e("cbc:ID", "20131312955")
	w.cierra("cac:PartyIdentification")
	w.cierra("cac:SenderParty")
	w.abre("cac:DocumentResponse")
	w.abre("cac:Response")
	w.e("cbc:ReferenceID", id)
	w.e("cbc:ResponseCode", "0")
	w.e("cbc:Description", desc)
	w.cierra("cac:Response")
	w.abre("cac:DocumentReference")
	w.e("cbc:ID", id)
	w.e("cbc:DocumentHash", hash)
	w.cierra("cac:DocumentReference")
	w.cierra("cac:DocumentResponse")
	w.b.WriteString("</ar:ApplicationResponse>")
	return CDR{Codigo: "0", Descripcion: desc, XML: w.b.String()}
}

// OSE: cliente SOAP del servicio billService (SUNAT beta o un OSE compatible).
type OSE struct {
	URL, Usuario, Clave string // usuario = RUC + usuario SOL
	HTTP                *http.Client
}

// Zip empaqueta el XML como lo pide SUNAT (RUC-TIPO-SERIE-NUMERO.zip con el .xml adentro).
func Zip(nombre string, datos []byte) ([]byte, error) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, err := z.Create(nombre + ".xml")
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(datos); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (o OSE) sobre(metodo, cuerpo string) string {
	return `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ser="http://service.sunat.gob.pe" ` +
		`xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"><soapenv:Header><wsse:Security><wsse:UsernameToken>` +
		`<wsse:Username>` + esc(o.Usuario) + `</wsse:Username><wsse:Password>` + esc(o.Clave) + `</wsse:Password></wsse:UsernameToken></wsse:Security></soapenv:Header>` +
		`<soapenv:Body><ser:` + metodo + `>` + cuerpo + `</ser:` + metodo + `></soapenv:Body></soapenv:Envelope>`
}

var reEtiqueta = func(tag string) *regexp.Regexp {
	return regexp.MustCompile(`<(?:\w+:)?` + tag + `>([^<]*)</(?:\w+:)?` + tag + `>`)
}

func (o OSE) llamar(ctx context.Context, metodo, cuerpo string) (string, error) {
	if o.URL == "" || o.Usuario == "" || o.Clave == "" {
		return "", errors.New("faltan las credenciales del OSE/SUNAT beta")
	}
	cli := o.HTTP
	if cli == nil {
		cli = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.URL, strings.NewReader(o.sobre(metodo, cuerpo)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "urn:"+metodo)
	res, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 5<<20))
	s := string(b)
	if f := reEtiqueta("faultstring").FindStringSubmatch(s); f != nil {
		cod := ""
		if c := reEtiqueta("faultcode").FindStringSubmatch(s); c != nil {
			cod = c[1]
		}
		return "", &Rechazo{Codigo: strings.TrimPrefix(strings.TrimPrefix(cod, "soap-env:Client."), "Client."), Mensaje: f[1]}
	}
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("el servicio respondió %d", res.StatusCode)
	}
	return s, nil
}

// Rechazo: SOAP fault del servicio (p. ej. 0111 credenciales, 2335 comprobante ya informado).
type Rechazo struct{ Codigo, Mensaje string }

func (r *Rechazo) Error() string { return "SUNAT " + r.Codigo + ": " + r.Mensaje }

// EnviarComprobante: sendBill. Devuelve el CDR leído del zip de respuesta.
func (o OSE) EnviarComprobante(ctx context.Context, nombre string, xmlFirmado []byte) (CDR, error) {
	zb, err := Zip(nombre, xmlFirmado)
	if err != nil {
		return CDR{}, err
	}
	resp, err := o.llamar(ctx, "sendBill", `<fileName>`+nombre+`.zip</fileName><contentFile>`+base64.StdEncoding.EncodeToString(zb)+`</contentFile>`)
	if err != nil {
		if r := (&Rechazo{}); errors.As(err, &r) {
			return CDR{Codigo: r.Codigo, Descripcion: r.Mensaje}, nil
		}
		return CDR{}, err
	}
	m := reEtiqueta("applicationResponse").FindStringSubmatch(resp)
	if m == nil {
		return CDR{}, errors.New("la respuesta no trae el CDR")
	}
	return LeerCDRZip(m[1])
}

// EnviarResumen: sendSummary (comunicación de baja). Devuelve el ticket para consultar después.
func (o OSE) EnviarResumen(ctx context.Context, nombre string, xmlFirmado []byte) (string, error) {
	zb, err := Zip(nombre, xmlFirmado)
	if err != nil {
		return "", err
	}
	resp, err := o.llamar(ctx, "sendSummary", `<fileName>`+nombre+`.zip</fileName><contentFile>`+base64.StdEncoding.EncodeToString(zb)+`</contentFile>`)
	if err != nil {
		return "", err
	}
	m := reEtiqueta("ticket").FindStringSubmatch(resp)
	if m == nil {
		return "", errors.New("la respuesta no trae el ticket")
	}
	return m[1], nil
}

// LeerCDRZip abre el applicationResponse (zip en base64) y lee código y descripción.
func LeerCDRZip(b64 string) (CDR, error) {
	zb, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return CDR{}, err
	}
	z, err := zip.NewReader(bytes.NewReader(zb), int64(len(zb)))
	if err != nil {
		return CDR{}, err
	}
	for _, f := range z.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return CDR{}, err
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		var ar struct {
			Resp struct {
				Codigo string `xml:"Response>ResponseCode"`
				Desc   string `xml:"Response>Description"`
			} `xml:"DocumentResponse"`
		}
		if err := xml.Unmarshal(b, &ar); err != nil {
			return CDR{}, err
		}
		return CDR{Codigo: ar.Resp.Codigo, Descripcion: ar.Resp.Desc, XML: string(b)}, nil
	}
	return CDR{}, errors.New("el zip del CDR no trae XML")
}
