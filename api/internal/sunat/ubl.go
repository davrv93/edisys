// Package sunat arma comprobantes electrónicos UBL 2.1 (boleta 03, factura 01 y nota de crédito 07) según
// las guías de SUNAT, los firma con XMLDSig (RSA-SHA256, firma envuelta) y simula o envía el CDR.
//
// El XML se escribe ya en forma canónica (C14N 1.0 inclusiva): espacios de nombres en la raíz ordenados,
// atributos ordenados, sin etiquetas vacías abreviadas y sin espacios entre elementos. Así el resumen del
// documento sin la firma coincide byte a byte con lo que calcula cualquier verificador.
package sunat

import (
	"fmt"
	"sort"
	"strings"
)

// Espacios de nombres.
const (
	NSInvoice    = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	NSCreditNote = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	NSCac        = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	NSCbc        = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
	NSExt        = "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
	NSDs         = "http://www.w3.org/2000/09/xmldsig#"
)

// Tipos de comprobante (catálogo 01).
const (
	Factura     = "01"
	Boleta      = "03"
	NotaCredito = "07"
)

// Afectación al IGV (catálogo 07).
const (
	Gravado   = "gravado"   // 10
	Exonerado = "exonerado" // 20
	Inafecto  = "inafecto"  // 30
)

// IGVPorMil: 18 %.
const IGVPorMil = 180

// Emisor del comprobante.
type Emisor struct {
	RUC, RazonSocial, Direccion, Ubigeo string
}

// Cliente (catálogo 06: 1 DNI, 6 RUC, 0 sin documento).
type Cliente struct {
	TipoDoc, NumDoc, Nombre string
}

// Linea de un comprobante; MontoCts incluye el IGV si es gravada.
type Linea struct {
	Descripcion string
	MontoCts    int64
	Afectacion  string
}

// Comprobante a emitir.
type Comprobante struct {
	Tipo         string // 01 | 03 | 07
	Serie        string
	Numero       int64
	Fecha        string // AAAA-MM-DD
	Hora         string // HH:MM:SS
	Emisor       Emisor
	Cliente      Cliente
	Lineas       []Linea
	Referencia   string // nota de crédito: serie-número afectado
	TipoRef      string // nota de crédito: 01 | 03
	MotivoCodigo string // nota de crédito (catálogo 09): 01 anulación de la operación
	Motivo       string
}

// Totales calculados.
type Totales struct {
	GravadoCts, ExoneradoCts, InafectoCts, IGVCts, TotalCts int64
}

// LineaCalc es una línea con base e IGV separados.
type LineaCalc struct {
	Linea
	BaseCts, IGVCts int64
}

// Calcular separa base e IGV de cada línea (IGV incluido en las gravadas) y suma por afectación.
func Calcular(ls []Linea) ([]LineaCalc, Totales) {
	var t Totales
	out := make([]LineaCalc, 0, len(ls))
	for _, l := range ls {
		c := LineaCalc{Linea: l, BaseCts: l.MontoCts}
		switch l.Afectacion {
		case Gravado:
			// base = total / 1,18 redondeado al céntimo; el IGV es la diferencia (la suma cuadra siempre).
			c.BaseCts = (l.MontoCts*1000 + (1000+IGVPorMil)/2) / (1000 + IGVPorMil)
			c.IGVCts = l.MontoCts - c.BaseCts
			t.GravadoCts += c.BaseCts
			t.IGVCts += c.IGVCts
		case Exonerado:
			t.ExoneradoCts += l.MontoCts
		default:
			c.Afectacion = Inafecto
			t.InafectoCts += l.MontoCts
		}
		t.TotalCts += l.MontoCts
		out = append(out, c)
	}
	return out, t
}

// Normalizar junta las líneas negativas (abonos de ajustes) con la primera línea de su misma afectación:
// SUNAT no acepta importes negativos en las líneas.
func Normalizar(ls []Linea) []Linea {
	var pos []Linea
	neg := map[string]int64{}
	for _, l := range ls {
		if l.Afectacion == "" {
			l.Afectacion = Inafecto
		}
		if l.MontoCts < 0 {
			neg[l.Afectacion] += l.MontoCts
			continue
		}
		if l.MontoCts > 0 {
			pos = append(pos, l)
		}
	}
	for i := range pos {
		if d := neg[pos[i].Afectacion]; d != 0 {
			q := pos[i].MontoCts + d
			if q < 0 {
				neg[pos[i].Afectacion], q = q, 0
			} else {
				neg[pos[i].Afectacion] = 0
			}
			pos[i].MontoCts = q
		}
	}
	out := pos[:0]
	for _, l := range pos {
		if l.MontoCts > 0 {
			out = append(out, l)
		}
	}
	return out
}

// ID del comprobante: B001-123.
func (c Comprobante) ID() string { return fmt.Sprintf("%s-%d", c.Serie, c.Numero) }

func m(cts int64) string { return fmt.Sprintf("%d.%02d", cts/100, cts%100) }

// esc escapa texto como la forma canónica.
func esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\r", "&#xD;").Replace(s)
}

func escAttr(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", "\"", "&quot;", "\t", "&#x9;", "\n", "&#xA;", "\r", "&#xD;").Replace(s)
}

// x escribe XML canónico: e("cbc:ID", texto, "schemeID", "6").
type x struct{ b strings.Builder }

func (w *x) abre(tag string, attrs ...string) {
	w.b.WriteString("<" + tag)
	type kv struct{ k, v string }
	var as []kv
	for i := 0; i+1 < len(attrs); i += 2 {
		as = append(as, kv{attrs[i], attrs[i+1]})
	}
	sort.Slice(as, func(i, j int) bool { return as[i].k < as[j].k }) // atributos sin prefijo: orden por nombre
	for _, a := range as {
		w.b.WriteString(" " + a.k + "=\"" + escAttr(a.v) + "\"")
	}
	w.b.WriteString(">")
}
func (w *x) cierra(tag string) { w.b.WriteString("</" + tag + ">") }
func (w *x) e(tag, texto string, attrs ...string) {
	w.abre(tag, attrs...)
	w.b.WriteString(esc(texto))
	w.cierra(tag)
}

// Marcador donde va la firma; al firmar se reemplaza por <ds:Signature>.
const marcaFirma = "<ext:ExtensionContent></ext:ExtensionContent>"

func raiz(w *x, ns string, tag string) {
	// Forma canónica: xmlns primero y luego por prefijo.
	w.b.WriteString("<" + tag + ` xmlns="` + ns + `" xmlns:cac="` + NSCac + `" xmlns:cbc="` + NSCbc + `" xmlns:ds="` + NSDs + `" xmlns:ext="` + NSExt + `">`)
	w.b.WriteString("<ext:UBLExtensions><ext:UBLExtension>" + marcaFirma + "</ext:UBLExtension></ext:UBLExtensions>")
}

func partes(w *x, c Comprobante) {
	// Referencia a la firma.
	w.abre("cac:Signature")
	w.e("cbc:ID", c.Emisor.RUC)
	w.abre("cac:SignatoryParty")
	w.abre("cac:PartyIdentification")
	w.e("cbc:ID", c.Emisor.RUC)
	w.cierra("cac:PartyIdentification")
	w.abre("cac:PartyName")
	w.e("cbc:Name", c.Emisor.RazonSocial)
	w.cierra("cac:PartyName")
	w.cierra("cac:SignatoryParty")
	w.abre("cac:DigitalSignatureAttachment")
	w.abre("cac:ExternalReference")
	w.e("cbc:URI", "#SignatureSP")
	w.cierra("cac:ExternalReference")
	w.cierra("cac:DigitalSignatureAttachment")
	w.cierra("cac:Signature")
	// Emisor.
	w.abre("cac:AccountingSupplierParty")
	w.abre("cac:Party")
	w.abre("cac:PartyIdentification")
	w.e("cbc:ID", c.Emisor.RUC, "schemeID", "6")
	w.cierra("cac:PartyIdentification")
	w.abre("cac:PartyName")
	w.e("cbc:Name", c.Emisor.RazonSocial)
	w.cierra("cac:PartyName")
	w.abre("cac:PartyLegalEntity")
	w.e("cbc:RegistrationName", c.Emisor.RazonSocial)
	w.abre("cac:RegistrationAddress")
	if c.Emisor.Ubigeo != "" {
		w.e("cbc:ID", c.Emisor.Ubigeo)
	}
	w.e("cbc:AddressTypeCode", "0000")
	if c.Emisor.Direccion != "" {
		w.abre("cac:AddressLine")
		w.e("cbc:Line", c.Emisor.Direccion)
		w.cierra("cac:AddressLine")
	}
	w.abre("cac:Country")
	w.e("cbc:IdentificationCode", "PE")
	w.cierra("cac:Country")
	w.cierra("cac:RegistrationAddress")
	w.cierra("cac:PartyLegalEntity")
	w.cierra("cac:Party")
	w.cierra("cac:AccountingSupplierParty")
	// Cliente.
	w.abre("cac:AccountingCustomerParty")
	w.abre("cac:Party")
	w.abre("cac:PartyIdentification")
	doc := c.Cliente.NumDoc
	if doc == "" {
		doc = "-"
	}
	w.e("cbc:ID", doc, "schemeID", c.Cliente.TipoDoc)
	w.cierra("cac:PartyIdentification")
	w.abre("cac:PartyLegalEntity")
	w.e("cbc:RegistrationName", c.Cliente.Nombre)
	w.cierra("cac:PartyLegalEntity")
	w.cierra("cac:Party")
	w.cierra("cac:AccountingCustomerParty")
}

type esquema struct{ id, nombre, tipo, motivo string }

var esquemas = map[string]esquema{
	Gravado:   {"1000", "IGV", "VAT", "10"},
	Exonerado: {"9997", "EXO", "VAT", "20"},
	Inafecto:  {"9998", "INA", "FRE", "30"},
}

func taxScheme(w *x, af string) {
	e := esquemas[af]
	w.abre("cac:TaxScheme")
	w.e("cbc:ID", e.id)
	w.e("cbc:Name", e.nombre)
	w.e("cbc:TaxTypeCode", e.tipo)
	w.cierra("cac:TaxScheme")
}

func totales(w *x, ls []LineaCalc, t Totales) {
	w.abre("cac:TaxTotal")
	w.e("cbc:TaxAmount", m(t.IGVCts), "currencyID", "PEN")
	for _, af := range []string{Gravado, Exonerado, Inafecto} {
		var base, igv int64
		hay := false
		for _, l := range ls {
			if l.Afectacion == af {
				hay = true
				base += l.BaseCts
				igv += l.IGVCts
			}
		}
		if !hay {
			continue
		}
		w.abre("cac:TaxSubtotal")
		w.e("cbc:TaxableAmount", m(base), "currencyID", "PEN")
		w.e("cbc:TaxAmount", m(igv), "currencyID", "PEN")
		w.abre("cac:TaxCategory")
		taxScheme(w, af)
		w.cierra("cac:TaxCategory")
		w.cierra("cac:TaxSubtotal")
	}
	w.cierra("cac:TaxTotal")
	w.abre("cac:LegalMonetaryTotal")
	w.e("cbc:LineExtensionAmount", m(t.GravadoCts+t.ExoneradoCts+t.InafectoCts), "currencyID", "PEN")
	w.e("cbc:TaxInclusiveAmount", m(t.TotalCts), "currencyID", "PEN")
	w.e("cbc:PayableAmount", m(t.TotalCts), "currencyID", "PEN")
	w.cierra("cac:LegalMonetaryTotal")
}

func lineas(w *x, tag, cantidad string, ls []LineaCalc) {
	for i, l := range ls {
		w.abre(tag)
		w.e("cbc:ID", fmt.Sprint(i+1))
		w.e(cantidad, "1", "unitCode", "ZZ")
		w.e("cbc:LineExtensionAmount", m(l.BaseCts), "currencyID", "PEN")
		w.abre("cac:PricingReference")
		w.abre("cac:AlternativeConditionPrice")
		w.e("cbc:PriceAmount", m(l.MontoCts), "currencyID", "PEN")
		w.e("cbc:PriceTypeCode", "01")
		w.cierra("cac:AlternativeConditionPrice")
		w.cierra("cac:PricingReference")
		w.abre("cac:TaxTotal")
		w.e("cbc:TaxAmount", m(l.IGVCts), "currencyID", "PEN")
		w.abre("cac:TaxSubtotal")
		w.e("cbc:TaxableAmount", m(l.BaseCts), "currencyID", "PEN")
		w.e("cbc:TaxAmount", m(l.IGVCts), "currencyID", "PEN")
		w.abre("cac:TaxCategory")
		pct := "0"
		if l.Afectacion == Gravado {
			pct = "18"
		}
		w.e("cbc:Percent", pct)
		w.e("cbc:TaxExemptionReasonCode", esquemas[l.Afectacion].motivo)
		taxScheme(w, l.Afectacion)
		w.cierra("cac:TaxCategory")
		w.cierra("cac:TaxSubtotal")
		w.cierra("cac:TaxTotal")
		w.abre("cac:Item")
		w.e("cbc:Description", l.Descripcion)
		w.cierra("cac:Item")
		w.abre("cac:Price")
		w.e("cbc:PriceAmount", m(l.BaseCts), "currencyID", "PEN")
		w.cierra("cac:Price")
		w.cierra(tag)
	}
}

// XML arma el comprobante sin firmar (con el marcador de la firma). Devuelve también los totales.
func XML(c Comprobante) (string, Totales, error) {
	if len(c.Lineas) == 0 {
		return "", Totales{}, fmt.Errorf("el comprobante no tiene líneas")
	}
	ls, t := Calcular(c.Lineas)
	w := &x{}
	root, cant, linea := "Invoice", "cbc:InvoicedQuantity", "cac:InvoiceLine"
	ns := NSInvoice
	if c.Tipo == NotaCredito {
		root, cant, linea, ns = "CreditNote", "cbc:CreditedQuantity", "cac:CreditNoteLine", NSCreditNote
	}
	raiz(w, ns, root)
	w.e("cbc:UBLVersionID", "2.1")
	w.e("cbc:CustomizationID", "2.0")
	w.e("cbc:ID", c.ID())
	w.e("cbc:IssueDate", c.Fecha)
	w.e("cbc:IssueTime", c.Hora)
	if c.Tipo != NotaCredito {
		w.e("cbc:InvoiceTypeCode", c.Tipo, "listID", "0101")
	}
	w.e("cbc:Note", MontoEnLetras(t.TotalCts), "languageLocaleID", "1000")
	w.e("cbc:DocumentCurrencyCode", "PEN")
	if c.Tipo == NotaCredito {
		w.abre("cac:DiscrepancyResponse")
		w.e("cbc:ReferenceID", c.Referencia)
		w.e("cbc:ResponseCode", c.MotivoCodigo)
		w.e("cbc:Description", c.Motivo)
		w.cierra("cac:DiscrepancyResponse")
		w.abre("cac:BillingReference")
		w.abre("cac:InvoiceDocumentReference")
		w.e("cbc:ID", c.Referencia)
		w.e("cbc:DocumentTypeCode", c.TipoRef)
		w.cierra("cac:InvoiceDocumentReference")
		w.cierra("cac:BillingReference")
	}
	partes(w, c)
	if c.Tipo == Factura {
		w.abre("cac:PaymentTerms")
		w.e("cbc:ID", "FormaPago")
		w.e("cbc:PaymentMeansID", "Contado")
		w.cierra("cac:PaymentTerms")
	}
	totales(w, ls, t)
	lineas(w, linea, cant, ls)
	w.cierra(root)
	return w.b.String(), t, nil
}

// Baja arma la comunicación de baja (VoidedDocuments, esquema propio de SUNAT) de una factura.
func Baja(e Emisor, id, fecha, fechaDoc, tipo, serie string, numero int64, motivo string) string {
	w := &x{}
	w.b.WriteString(`<VoidedDocuments xmlns="urn:sunat:names:specification:ubl:peru:schema:xsd:VoidedDocuments-1" xmlns:cac="` + NSCac + `" xmlns:cbc="` + NSCbc +
		`" xmlns:ds="` + NSDs + `" xmlns:ext="` + NSExt + `" xmlns:sac="urn:sunat:names:specification:ubl:peru:schema:xsd:SunatAggregateComponents-1">`)
	w.b.WriteString("<ext:UBLExtensions><ext:UBLExtension>" + marcaFirma + "</ext:UBLExtension></ext:UBLExtensions>")
	w.e("cbc:UBLVersionID", "2.0")
	w.e("cbc:CustomizationID", "1.0")
	w.e("cbc:ID", id)
	w.e("cbc:ReferenceDate", fechaDoc)
	w.e("cbc:IssueDate", fecha)
	partes(w, Comprobante{Emisor: e, Cliente: Cliente{TipoDoc: "6", NumDoc: e.RUC, Nombre: e.RazonSocial}})
	w.abre("sac:VoidedDocumentsLine")
	w.e("cbc:LineID", "1")
	w.e("cbc:DocumentTypeCode", tipo)
	w.e("sac:DocumentSerialID", serie)
	w.e("sac:DocumentNumberID", fmt.Sprint(numero))
	w.e("sac:VoidReasonDescription", motivo)
	w.cierra("sac:VoidedDocumentsLine")
	w.cierra("VoidedDocuments")
	return w.b.String()
}

// ---------- validaciones ----------

// RUCValido: 11 dígitos, empieza en 10, 15, 16, 17 o 20, y dígito verificador módulo 11.
func RUCValido(r string) bool {
	if len(r) != 11 {
		return false
	}
	for _, c := range r {
		if c < '0' || c > '9' {
			return false
		}
	}
	switch r[:2] {
	case "10", "15", "16", "17", "20":
	default:
		return false
	}
	pesos := []int{5, 4, 3, 2, 7, 6, 5, 4, 3, 2}
	s := 0
	for i, p := range pesos {
		s += int(r[i]-'0') * p
	}
	d := 11 - s%11
	if d == 10 {
		d = 0
	} else if d == 11 {
		d = 1
	}
	return int(r[10]-'0') == d
}

// ClienteDe decide el tipo de comprobante por el documento: RUC → factura, DNI → boleta, sin documento → boleta.
func ClienteDe(doc, nombre string) (Cliente, string) {
	d := strings.TrimSpace(doc)
	if RUCValido(d) {
		return Cliente{TipoDoc: "6", NumDoc: d, Nombre: nombre}, Factura
	}
	if len(d) == 8 && strings.Trim(d, "0123456789") == "" {
		return Cliente{TipoDoc: "1", NumDoc: d, Nombre: nombre}, Boleta
	}
	return Cliente{TipoDoc: "0", NumDoc: "-", Nombre: nombre}, Boleta
}
