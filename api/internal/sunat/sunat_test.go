package sunat

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ejemplo(tipo string) Comprobante {
	cli, _ := ClienteDe("40000201", "María Demo")
	c := Comprobante{Tipo: tipo, Serie: "B001", Numero: 1, Fecha: "2026-09-28", Hora: "10:00:00",
		Emisor:  Emisor{RUC: "20600000001", RazonSocial: "Junta de Propietarios Edificio Demo", Direccion: "Av. José Larco 1234, Miraflores", Ubigeo: "150122"},
		Cliente: cli,
		Lineas: []Linea{{"Cuota de mantenimiento", 70560, Inafecto}, {"Agua (consumo propio 14,000 m³)", 19600, Inafecto}, {"Áreas comunes (agua)", 840, Inafecto},
			{"Reserva R-0412 Parrilla 1 & <terraza>", 8000, Gravado}}}
	if tipo == NotaCredito {
		c.Serie, c.Referencia, c.TipoRef, c.MotivoCodigo, c.Motivo = "BC01", "B001-1", Boleta, "01", "Anulación de la operación"
	}
	return c
}

func TestCalculoYLetras(t *testing.T) {
	ls, tot := Calcular(Normalizar(ejemplo(Boleta).Lineas))
	if tot.TotalCts != 99000 || tot.InafectoCts != 91000 || tot.GravadoCts != 6780 || tot.IGVCts != 1220 {
		t.Errorf("totales %+v", tot)
	}
	if ls[3].BaseCts+ls[3].IGVCts != 8000 {
		t.Errorf("la línea gravada no cuadra: %+v", ls[3])
	}
	for n, q := range map[int64]string{99000: "SON NOVECIENTOS NOVENTA CON 00/100 SOLES", 100: "SON UNO CON 00/100 SOLES", 3412050: "SON TREINTA Y CUATRO MIL CIENTO VEINTE CON 50/100 SOLES",
		2100000: "SON VEINTIUN MIL CON 00/100 SOLES", 10000000: "SON CIEN MIL CON 00/100 SOLES"} {
		if g := MontoEnLetras(n); g != q {
			t.Errorf("MontoEnLetras(%d) = %q", n, g)
		}
	}
	// Un abono se descuenta de la primera línea de su afectación.
	n := Normalizar([]Linea{{"Cuota", 70000, Inafecto}, {"Nota de abono", -500, Inafecto}, {"Reserva", 8000, Gravado}})
	if len(n) != 2 || n[0].MontoCts != 69500 {
		t.Errorf("normalizar: %+v", n)
	}
}

func TestBoletaOFacturaPorDocumento(t *testing.T) {
	if !RUCValido("20100070970") || RUCValido("20100070971") || RUCValido("40000201") {
		t.Error("RUCValido")
	}
	if _, tipo := ClienteDe("20100070970", "Empresa SAC"); tipo != Factura {
		t.Error("RUC → factura")
	}
	if c, tipo := ClienteDe("40000201", "María"); tipo != Boleta || c.TipoDoc != "1" {
		t.Error("DNI → boleta")
	}
	if c, tipo := ClienteDe("", "Sin doc"); tipo != Boleta || c.TipoDoc != "0" {
		t.Error("sin documento → boleta")
	}
}

func TestFirmaYVerificacion(t *testing.T) {
	cert, err := CertificadoPrueba()
	if err != nil {
		t.Fatal(err)
	}
	for _, tipo := range []string{Boleta, Factura, NotaCredito} {
		doc, _, err := XML(ejemplo(tipo))
		if err != nil {
			t.Fatal(err)
		}
		firmado, hash, err := Firmar(doc, cert)
		if err != nil || hash == "" {
			t.Fatal(err)
		}
		if err := Verificar(firmado, cert.Cert); err != nil {
			t.Errorf("%s: la firma no verifica: %v", tipo, err)
		}
		// Bien formado.
		d := xml.NewDecoder(strings.NewReader(firmado))
		for {
			if _, err := d.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s mal formado: %v", tipo, err)
			}
		}
		// Alterar un importe rompe la firma.
		if err := Verificar(strings.Replace(firmado, "990.00", "900.00", 1), cert.Cert); err == nil {
			t.Errorf("%s: la firma aceptó un documento alterado", tipo)
		}
		if dir := os.Getenv("SUNAT_SALIDA"); dir != "" {
			_ = os.WriteFile(filepath.Join(dir, tipo+".xml"), []byte(firmado), 0o644)
		}
	}
}

// Valida contra los XSD de UBL 2.1 si están (scripts/validar-ubl.sh los baja y corre xmllint).
func TestEsquemaUBL(t *testing.T) {
	dir := os.Getenv("UBL_XSD")
	if dir == "" {
		t.Skip("sin UBL_XSD: corre scripts/validar-ubl.sh")
	}
	cert, _ := CertificadoPrueba()
	for tipo, xsd := range map[string]string{Boleta: "UBL-Invoice-2.1.xsd", Factura: "UBL-Invoice-2.1.xsd", NotaCredito: "UBL-CreditNote-2.1.xsd"} {
		doc, _, _ := XML(ejemplo(tipo))
		firmado, _, _ := Firmar(doc, cert)
		f := filepath.Join(t.TempDir(), tipo+".xml")
		_ = os.WriteFile(f, []byte(firmado), 0o644)
		out, err := exec.Command("xmllint", "--noout", "--schema", filepath.Join(dir, "maindoc", xsd), f).CombinedOutput()
		if err != nil {
			t.Errorf("%s no valida contra %s: %s", tipo, xsd, out)
		}
	}
}

// El modo beta habla SOAP con el servicio; aquí contra un SUNAT falso que devuelve un CDR aceptado.
func TestEnvioBetaContraServicioFalso(t *testing.T) {
	var recibido string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		recibido = string(b)
		var zb bytes.Buffer
		z := zip.NewWriter(&zb)
		f, _ := z.Create("R-20600000001-03-B001-1.xml")
		_, _ = f.Write([]byte(CDRSimulado("20600000001", Boleta, "B001-1", "x", time.Now()).XML))
		_ = z.Close()
		_, _ = w.Write([]byte(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><br:sendBillResponse xmlns:br="http://service.sunat.gob.pe"><applicationResponse>` +
			base64.StdEncoding.EncodeToString(zb.Bytes()) + `</applicationResponse></br:sendBillResponse></soap:Body></soap:Envelope>`))
	}))
	defer srv.Close()
	o := OSE{URL: srv.URL, Usuario: "20600000001MODDATOS", Clave: "moddatos"}
	cdr, err := o.EnviarComprobante(context.Background(), "20600000001-03-B001-1", []byte("<Invoice/>"))
	if err != nil || !cdr.Aceptado() || !strings.Contains(cdr.Descripcion, "aceptada") {
		t.Fatalf("cdr %+v %v", cdr, err)
	}
	if !strings.Contains(recibido, "<fileName>20600000001-03-B001-1.zip</fileName>") || !strings.Contains(recibido, "<wsse:Username>20600000001MODDATOS</wsse:Username>") {
		t.Errorf("sobre SOAP: %s", recibido)
	}
	if _, err := (OSE{URL: srv.URL}).EnviarComprobante(context.Background(), "x", nil); err == nil {
		t.Error("sin credenciales no debería enviar")
	}
}
