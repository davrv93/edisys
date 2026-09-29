package sunat

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/pkcs12"
)

// Certificado para firmar.
type Certificado struct {
	Clave  *rsa.PrivateKey
	Cert   *x509.Certificate
	Prueba bool // autofirmado de EDISYS (modo simulado sin .pfx)
}

// LeerPFX abre un .pfx/.p12 con su clave.
func LeerPFX(datos []byte, clave string) (*Certificado, error) {
	k, c, err := pkcs12.Decode(datos, clave)
	if err != nil {
		return nil, fmt.Errorf("no pude abrir el certificado: %w", err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("el certificado no trae una clave RSA")
	}
	return &Certificado{Clave: rk, Cert: c}, nil
}

var (
	pruebaUna  sync.Once
	pruebaCert *Certificado
	pruebaErr  error
)

// CertificadoPrueba: un certificado autofirmado de EDISYS, solo para el modo simulado (se genera una vez por proceso).
func CertificadoPrueba() (*Certificado, error) {
	pruebaUna.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			pruebaErr = err
			return
		}
		tpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "EDISYS certificado de prueba (no válido para SUNAT)", Country: []string{"PE"}},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(2, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature}
		der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
		if err != nil {
			pruebaErr = err
			return
		}
		c, err := x509.ParseCertificate(der)
		pruebaCert, pruebaErr = &Certificado{Clave: k, Cert: c, Prueba: true}, err
	})
	return pruebaCert, pruebaErr
}

// signedInfo en forma canónica dentro del documento (hereda los espacios de nombres de la raíz).
func signedInfo(digest string) string {
	return `<ds:SignedInfo><ds:CanonicalizationMethod Algorithm="http://www.w3.org/TR/2001/REC-xml-c14n-20010315"></ds:CanonicalizationMethod>` +
		`<ds:SignatureMethod Algorithm="http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"></ds:SignatureMethod>` +
		`<ds:Reference URI=""><ds:Transforms><ds:Transform Algorithm="http://www.w3.org/2000/09/xmldsig#enveloped-signature"></ds:Transform></ds:Transforms>` +
		`<ds:DigestMethod Algorithm="http://www.w3.org/2001/04/xmlenc#sha256"></ds:DigestMethod><ds:DigestValue>` + digest + `</ds:DigestValue></ds:Reference></ds:SignedInfo>`
}

// nsRaiz: las declaraciones de la raíz, que la forma canónica de SignedInfo repite (C14N inclusiva).
func nsRaiz(doc string) string {
	i := strings.Index(doc, " xmlns=")
	j := strings.Index(doc, ">")
	if i < 0 || j < i {
		return ""
	}
	return doc[i:j]
}

// Firmar inserta la firma XMLDSig envuelta en ext:ExtensionContent. Devuelve el XML firmado y el
// DigestValue (el «hash» que va en el QR y en la representación impresa).
func Firmar(doc string, c *Certificado) (string, string, error) {
	if !strings.Contains(doc, marcaFirma) {
		return "", "", errors.New("el XML no tiene dónde poner la firma")
	}
	// Con la firma envuelta quitada, el documento canónico es el mismo XML con ExtensionContent vacío.
	sum := sha256.Sum256([]byte(doc))
	digest := base64.StdEncoding.EncodeToString(sum[:])
	si := signedInfo(digest)
	canonico := strings.Replace(si, "<ds:SignedInfo>", "<ds:SignedInfo"+nsRaiz(doc)+">", 1)
	h := sha256.Sum256([]byte(canonico))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.Clave, crypto.SHA256, h[:])
	if err != nil {
		return "", "", err
	}
	firma := `<ds:Signature Id="SignatureSP">` + si + `<ds:SignatureValue>` + base64.StdEncoding.EncodeToString(sig) + `</ds:SignatureValue>` +
		`<ds:KeyInfo><ds:X509Data><ds:X509Certificate>` + base64.StdEncoding.EncodeToString(c.Cert.Raw) + `</ds:X509Certificate></ds:X509Data></ds:KeyInfo></ds:Signature>`
	firmado := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + strings.Replace(doc, marcaFirma, "<ext:ExtensionContent>"+firma+"</ext:ExtensionContent>", 1)
	return firmado, digest, nil
}

// Verificar comprueba un XML firmado por Firmar (resumen del documento y firma de SignedInfo).
func Verificar(firmado string, cert *x509.Certificate) error {
	doc := strings.TrimPrefix(firmado, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")
	i := strings.Index(doc, `<ds:Signature Id="SignatureSP">`)
	j := strings.Index(doc, `</ds:Signature>`)
	if i < 0 || j < i {
		return errors.New("sin firma")
	}
	firma := doc[i : j+len(`</ds:Signature>`)]
	sin := doc[:i] + doc[j+len(`</ds:Signature>`):]
	sum := sha256.Sum256([]byte(sin))
	digest := base64.StdEncoding.EncodeToString(sum[:])
	if !strings.Contains(firma, "<ds:DigestValue>"+digest+"</ds:DigestValue>") {
		return errors.New("el resumen del documento no coincide")
	}
	a := strings.Index(firma, "<ds:SignedInfo>")
	b := strings.Index(firma, "</ds:SignedInfo>") + len("</ds:SignedInfo>")
	canonico := strings.Replace(firma[a:b], "<ds:SignedInfo>", "<ds:SignedInfo"+nsRaiz(sin)+">", 1)
	sv := firma[strings.Index(firma, "<ds:SignatureValue>")+len("<ds:SignatureValue>") : strings.Index(firma, "</ds:SignatureValue>")]
	sig, err := base64.StdEncoding.DecodeString(sv)
	if err != nil {
		return err
	}
	h := sha256.Sum256([]byte(canonico))
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("clave pública no RSA")
	}
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], sig)
}
