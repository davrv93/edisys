package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Layouts de banco para la cuenta recaudadora (bloque A2). El CREP (lo que el edificio manda al banco:
// una fila por deuda) y el CDPG (lo que el banco devuelve: una fila por pago) son TXT de ancho fijo
// que cambian por banco y por año. Cada banco es un adaptador LayoutBanco con su cabecera, detalle y
// cola; las pruebas de contrato fijan las posiciones exactas para que un cambio no pase en silencio.

// CrepCabecera son los datos de la cuenta y los totales del archivo.
type CrepCabecera struct {
	Cuenta    string // número de cuenta recaudadora tal como lo da el banco («193-1234567-0-12»)
	Moneda    string // PEN o USD
	Empresa   string
	Fecha     time.Time
	Registros int
	TotalCts  int64
}

// CrepDetalle es una deuda: quién paga (código de depositante), con qué referencia vuelve y cuánto.
type CrepDetalle struct {
	Depositante string // código de la unidad
	Nombre      string
	Referencia  string // número del recibo: vuelve tal cual en el CDPG
	Emision     time.Time
	Vence       time.Time
	MontoCts    int64
	MoraCts     int64
}

// CdpgFila es un pago leído del CDPG.
type CdpgFila struct {
	Linea           int    `json:"linea"`
	Depositante     string `json:"codigo_depositante"`
	Referencia      string `json:"referencia"`
	Fecha           string `json:"fecha"` // AAAA-MM-DD
	MontoCts        int64  `json:"monto_cts"`
	Agencia         string `json:"agencia"`
	NumeroOperacion string `json:"numero_operacion"`
}

// LayoutBanco es el adaptador de un banco.
type LayoutBanco interface {
	Codigo() string
	Nombre() string
	// Provisional: true mientras el layout no venga del convenio firmado con el banco.
	Provisional() bool
	Cabecera(c CrepCabecera) (string, error)
	Detalle(c CrepCabecera, d CrepDetalle) (string, error)
	Cola(c CrepCabecera) string
	// LeerLineaCDPG devuelve (fila, true, nil) si la línea es un pago; (_, false, nil) si es cabecera,
	// cola o vacía; y un error si es un detalle ilegible (la carga sigue con las demás líneas).
	LeerLineaCDPG(linea string) (CdpgFila, bool, error)
}

// layouts disponibles por código.
var layouts = map[string]LayoutBanco{"bcp_ref": layoutBCPRef{}}

// LayoutDeBanco devuelve el adaptador por código (vacío = el de referencia BCP).
func LayoutDeBanco(c string) (LayoutBanco, bool) {
	if c == "" {
		c = "bcp_ref"
	}
	l, ok := layouts[c]
	return l, ok
}

// GenerarCREP arma el archivo completo: cabecera, un detalle por deuda y cola, con fin de línea CRLF.
// Los totales de la cabecera se calculan aquí, nunca se reciben.
func GenerarCREP(l LayoutBanco, cab CrepCabecera, dets []CrepDetalle) (string, error) {
	cab.Registros = len(dets)
	cab.TotalCts = 0
	for _, d := range dets {
		cab.TotalCts += d.MontoCts
	}
	var b strings.Builder
	h, err := l.Cabecera(cab)
	if err != nil {
		return "", err
	}
	b.WriteString(h + "\r\n")
	for _, d := range dets {
		ln, err := l.Detalle(cab, d)
		if err != nil {
			return "", fmt.Errorf("depositante %s: %w", d.Depositante, err)
		}
		b.WriteString(ln + "\r\n")
	}
	if c := l.Cola(cab); c != "" {
		b.WriteString(c + "\r\n")
	}
	return b.String(), nil
}

// LeerCDPG lee todas las líneas; una línea mala no corta la lectura.
func LeerCDPG(l LayoutBanco, datos []byte) ([]CdpgFila, []map[string]any) {
	texto := strings.ReplaceAll(string(datos), "\r\n", "\n")
	var filas []CdpgFila
	errores := []map[string]any{}
	for i, ln := range strings.Split(texto, "\n") {
		f, es, err := l.LeerLineaCDPG(ln)
		if err != nil {
			errores = append(errores, map[string]any{"linea": i + 1, "motivo": err.Error()})
			continue
		}
		if es {
			f.Linea = i + 1
			filas = append(filas, f)
		}
	}
	return filas, errores
}

// ---------- utilidades de ancho fijo ----------

// alfa: texto en mayúsculas, sin tildes ni caracteres fuera de ASCII, relleno con espacios a la derecha.
func alfa(s string, n int) string {
	s = strings.ToUpper(strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", "Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ñ", "N", "ü", "u", "Ü", "U").Replace(s))
	var b strings.Builder
	for _, r := range s {
		if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '.') {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > n {
		return out[:n]
	}
	return out + strings.Repeat(" ", n-len(out))
}

// num: entero sin signo con ceros a la izquierda; error si no cabe.
func num(v int64, n int) (string, error) {
	if v < 0 {
		return "", fmt.Errorf("número negativo")
	}
	s := strconv.FormatInt(v, 10)
	if len(s) > n {
		return "", fmt.Errorf("%d no cabe en %d posiciones", v, n)
	}
	return strings.Repeat("0", n-len(s)) + s, nil
}

func relleno(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func soloDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---------- BCP · layout de referencia (PROVISIONAL) ----------

// layoutBCPRef sigue la forma pública del CREP/CDPG de BCP (registros «CC» y «DD» de 250 posiciones),
// pero es PROVISIONAL: las posiciones exactas se cambian cuando llegue el layout del convenio. La cola
// «TT» es nuestra (control de registros y total); si el banco no la acepta, se apaga devolviendo "".
//
// CREP · cabecera «CC» (posiciones 1-based):
//
//	1-2 «CC» · 3-5 sucursal · 6 moneda (0 PEN, 1 USD) · 7-13 cuenta · 14 «C» validación completa ·
//	15-54 empresa · 55-62 fecha AAAAMMDD · 63-71 registros · 72-86 total en céntimos · 87 «R» reemplazo · 88-250 espacios
//
// CREP · detalle «DD»:
//
//	1-2 «DD» · 3-5 sucursal · 6 moneda · 7-13 cuenta · 14-27 depositante · 28-67 nombre · 68-97 referencia ·
//	98-105 emisión · 106-113 vencimiento · 114-128 monto · 129-143 mora · 144-152 monto mínimo · 153 «A» alta · 154-250 espacios
//
// CREP · cola «TT»: 1-2 «TT» · 3-11 registros · 12-26 total · 27-250 espacios
//
// CDPG · detalle «DD»:
//
//	1-2 «DD» · 3-13 sucursal+moneda+cuenta · 14-27 depositante · 28-57 referencia · 58-65 fecha de pago ·
//	66-73 vencimiento · 74-88 monto pagado · 89-103 mora · 104-118 monto total · 119-124 agencia · 125-130 n.º de operación
type layoutBCPRef struct{}

const anchoBCP = 250

func (layoutBCPRef) Codigo() string    { return "bcp_ref" }
func (layoutBCPRef) Nombre() string    { return "BCP · layout de referencia" }
func (layoutBCPRef) Provisional() bool { return true }

// cuentaBCP parte «193-1234567-0-12» en sucursal (3) y cuenta (7).
func cuentaBCP(c string) (string, string, error) {
	d := soloDigitos(c)
	if len(d) < 10 {
		return "", "", fmt.Errorf("la cuenta %q no tiene el formato BCP (sucursal + 7 dígitos)", c)
	}
	return d[:3], d[3:10], nil
}

func monedaBCP(m string) string {
	if m == "USD" {
		return "1"
	}
	return "0"
}

func (layoutBCPRef) Cabecera(c CrepCabecera) (string, error) {
	suc, cta, err := cuentaBCP(c.Cuenta)
	if err != nil {
		return "", err
	}
	reg, err := num(int64(c.Registros), 9)
	if err != nil {
		return "", err
	}
	tot, err := num(c.TotalCts, 15)
	if err != nil {
		return "", err
	}
	return relleno("CC"+suc+monedaBCP(c.Moneda)+cta+"C"+alfa(c.Empresa, 40)+c.Fecha.Format("20060102")+reg+tot+"R", anchoBCP), nil
}

func (layoutBCPRef) Detalle(c CrepCabecera, d CrepDetalle) (string, error) {
	suc, cta, err := cuentaBCP(c.Cuenta)
	if err != nil {
		return "", err
	}
	monto, err := num(d.MontoCts, 15)
	if err != nil {
		return "", err
	}
	mora, err := num(d.MoraCts, 15)
	if err != nil {
		return "", err
	}
	return relleno("DD"+suc+monedaBCP(c.Moneda)+cta+alfa(d.Depositante, 14)+alfa(d.Nombre, 40)+alfa(d.Referencia, 30)+
		d.Emision.Format("20060102")+d.Vence.Format("20060102")+monto+mora+"000000000"+"A", anchoBCP), nil
}

func (layoutBCPRef) Cola(c CrepCabecera) string {
	reg, _ := num(int64(c.Registros), 9)
	tot, _ := num(c.TotalCts, 15)
	return relleno("TT"+reg+tot, anchoBCP)
}

func (layoutBCPRef) LeerLineaCDPG(ln string) (CdpgFila, bool, error) {
	ln = strings.TrimRight(ln, "\r")
	if strings.TrimSpace(ln) == "" || strings.HasPrefix(ln, "CC") || strings.HasPrefix(ln, "TT") {
		return CdpgFila{}, false, nil
	}
	if !strings.HasPrefix(ln, "DD") {
		return CdpgFila{}, false, fmt.Errorf("tipo de registro desconocido %q", firstN(ln, 2))
	}
	if len(ln) < 130 {
		return CdpgFila{}, false, fmt.Errorf("línea corta: %d posiciones, se esperan al menos 130", len(ln))
	}
	sub := func(a, b int) string { return strings.TrimSpace(ln[a-1 : b]) } // posiciones 1-based inclusivas
	fecha, err := time.Parse("20060102", sub(58, 65))
	if err != nil {
		return CdpgFila{}, false, fmt.Errorf("fecha de pago inválida %q", sub(58, 65))
	}
	total, err := strconv.ParseInt(sub(104, 118), 10, 64)
	if err != nil || total <= 0 {
		return CdpgFila{}, false, fmt.Errorf("monto total inválido %q", sub(104, 118))
	}
	op := sub(125, 130)
	if op == "" {
		return CdpgFila{}, false, fmt.Errorf("falta el número de operación")
	}
	return CdpgFila{Depositante: sub(14, 27), Referencia: sub(28, 57), Fecha: fecha.Format("2006-01-02"), MontoCts: total,
		Agencia: sub(119, 124), NumeroOperacion: op}, true, nil
}

func firstN(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
