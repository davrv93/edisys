// Package conciliacion lee extractos bancarios (CSV o XLSX), aplica el mapeo de columnas del banco y
// empareja cada movimiento con los pagos (ingresos) y egresos del sistema. Las funciones de este archivo
// son puras: se prueban sin base.
package conciliacion

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xuri/excelize/v2"
)

// Estados de un movimiento.
const (
	Conciliado = "conciliado"
	Sugerido   = "sugerido"
	SinPareja  = "sin_pareja"
)

// Reglas de emparejado, en orden.
const (
	ReglaCodigo     = "codigo"      // mismo código de operación (y monto) → conciliado
	ReglaMontoFecha = "monto_fecha" // monto exacto y fecha ±2 días → sugerido
	ReglaMonto      = "monto"       // monto exacto sin fecha → sugerido
	ReglaManual     = "manual"
)

// Mov es un movimiento del extracto. Monto > 0 abono (entra plata) · < 0 cargo (sale).
type Mov struct {
	Fecha       string `json:"fecha"` // AAAA-MM-DD
	Descripcion string `json:"descripcion"`
	MontoCts    int64  `json:"monto_cts"`
	Codigo      string `json:"codigo_operacion"`
}

// Item es un pago o un egreso del sistema, con el mismo signo que el banco.
type Item struct {
	Tipo        string `json:"tipo"` // pago | egreso
	ID          int64  `json:"id"`
	Fecha       string `json:"fecha"`
	MontoCts    int64  `json:"monto_cts"`
	Codigo      string `json:"codigo_operacion"`
	Descripcion string `json:"descripcion"`
}

// Pareja: resultado del emparejado de un movimiento. Item = -1 si no tiene pareja.
type Pareja struct {
	Mov    int
	Item   int
	Estado string
	Regla  string
}

// NormalizarCodigo deja solo letras y dígitos, sin ceros a la izquierda.
func NormalizarCodigo(c string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(c) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimLeft(b.String(), "0")
}

func dias(a, b string) int {
	ta, e1 := time.Parse("2006-01-02", a)
	tb, e2 := time.Parse("2006-01-02", b)
	if e1 != nil || e2 != nil {
		return 9999
	}
	d := ta.Sub(tb).Hours() / 24
	return int(math.Abs(math.Round(d)))
}

// Emparejar aplica las tres reglas en orden; cada ítem del sistema se usa una sola vez.
//  1. código de operación (con el mismo monto: conciliado; con otro monto: sugerido);
//  2. monto exacto y fecha ±2 días (el más cercano): sugerido;
//  3. monto exacto sin mirar la fecha: sugerido.
func Emparejar(movs []Mov, items []Item) []Pareja {
	out := make([]Pareja, len(movs))
	usado := make([]bool, len(items))
	hecho := make([]bool, len(movs))
	for i := range movs {
		out[i] = Pareja{Mov: i, Item: -1, Estado: SinPareja}
	}
	// 1 · código.
	for i, m := range movs {
		c := NormalizarCodigo(m.Codigo)
		if c == "" {
			continue
		}
		for j, it := range items {
			if usado[j] || NormalizarCodigo(it.Codigo) != c || (it.MontoCts > 0) != (m.MontoCts > 0) {
				continue
			}
			estado := Conciliado
			if it.MontoCts != m.MontoCts {
				estado = Sugerido
			}
			out[i] = Pareja{Mov: i, Item: j, Estado: estado, Regla: ReglaCodigo}
			usado[j], hecho[i] = true, true
			break
		}
	}
	// 2 · monto y fecha ±2 días, el más cercano.
	for i, m := range movs {
		if hecho[i] {
			continue
		}
		mejor, dist := -1, 3
		for j, it := range items {
			if usado[j] || it.MontoCts != m.MontoCts {
				continue
			}
			if d := dias(m.Fecha, it.Fecha); d < dist {
				mejor, dist = j, d
			}
		}
		if mejor >= 0 {
			out[i] = Pareja{Mov: i, Item: mejor, Estado: Sugerido, Regla: ReglaMontoFecha}
			usado[mejor], hecho[i] = true, true
		}
	}
	// 3 · solo monto.
	for i, m := range movs {
		if hecho[i] {
			continue
		}
		for j, it := range items {
			if !usado[j] && it.MontoCts == m.MontoCts {
				out[i] = Pareja{Mov: i, Item: j, Estado: Sugerido, Regla: ReglaMonto}
				usado[j], hecho[i] = true, true
				break
			}
		}
	}
	return out
}

// ---------- lectura del archivo ----------

// Leer devuelve cabeceras y filas de un CSV (coma, punto y coma o tabulador) o de un XLSX (primera hoja).
func Leer(nombre string, datos []byte) ([]string, [][]string, error) {
	var filas [][]string
	if strings.HasSuffix(strings.ToLower(nombre), ".xlsx") || bytes.HasPrefix(datos, []byte("PK\x03\x04")) {
		f, err := excelize.OpenReader(bytes.NewReader(datos))
		if err != nil {
			return nil, nil, fmt.Errorf("no pude abrir el Excel: %w", err)
		}
		defer f.Close()
		hojas := f.GetSheetList()
		if len(hojas) == 0 {
			return nil, nil, errors.New("el Excel no tiene hojas")
		}
		if filas, err = f.GetRows(hojas[0]); err != nil {
			return nil, nil, err
		}
	} else {
		datos = bytes.TrimPrefix(datos, []byte("\xef\xbb\xbf"))
		primera := string(datos)
		if i := strings.IndexByte(primera, '\n'); i >= 0 {
			primera = primera[:i]
		}
		sep := ','
		if strings.Count(primera, ";") > strings.Count(primera, ",") {
			sep = ';'
		}
		if strings.Count(primera, "\t") > strings.Count(primera, string(sep)) {
			sep = '\t'
		}
		r := csv.NewReader(bytes.NewReader(datos))
		r.Comma = sep
		r.FieldsPerRecord = -1
		r.TrimLeadingSpace = true
		var err error
		if filas, err = r.ReadAll(); err != nil {
			return nil, nil, fmt.Errorf("CSV inválido: %w", err)
		}
	}
	// Salta filas vacías del inicio (algunos bancos ponen un título).
	for len(filas) > 0 && strings.TrimSpace(strings.Join(filas[0], "")) == "" {
		filas = filas[1:]
	}
	if len(filas) < 2 {
		return nil, nil, errors.New("el extracto no tiene movimientos")
	}
	cab := make([]string, len(filas[0]))
	for i, c := range filas[0] {
		cab[i] = strings.TrimSpace(c)
	}
	return cab, filas[1:], nil
}

// Mapeo: qué columna (por nombre de cabecera) es cada dato. Monto con signo, o Cargo y Abono por separado.
type Mapeo struct {
	Fecha       string `json:"fecha"`
	Descripcion string `json:"descripcion"`
	Monto       string `json:"monto"`
	Cargo       string `json:"cargo,omitempty"`
	Abono       string `json:"abono,omitempty"`
	Codigo      string `json:"codigo_operacion"`
	Saldo       string `json:"saldo,omitempty"`
}

func clave(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", ".", "", "_", " ").Replace(s)
}

// AutoMapeo adivina las columnas por el nombre de la cabecera.
func AutoMapeo(cab []string) Mapeo {
	var m Mapeo
	for _, c := range cab {
		k := clave(c)
		switch {
		case m.Fecha == "" && (strings.HasPrefix(k, "fecha") || k == "fec operacion" || k == "date"):
			m.Fecha = c
		case m.Descripcion == "" && (strings.Contains(k, "descrip") || strings.Contains(k, "concepto") || strings.Contains(k, "detalle") || strings.Contains(k, "glosa")):
			m.Descripcion = c
		case m.Codigo == "" && (strings.Contains(k, "operacion") || strings.Contains(k, "codigo") || k == "n op" || k == "nro op" || strings.Contains(k, "referencia")):
			m.Codigo = c
		case m.Monto == "" && (k == "monto" || k == "importe" || strings.HasPrefix(k, "monto ") || strings.HasPrefix(k, "importe ")):
			m.Monto = c
		case m.Saldo == "" && strings.HasPrefix(k, "saldo"):
			m.Saldo = c
		case m.Cargo == "" && (strings.HasPrefix(k, "cargo") || strings.HasPrefix(k, "debe") || strings.HasPrefix(k, "retiro")):
			m.Cargo = c
		case m.Abono == "" && (strings.HasPrefix(k, "abono") || strings.HasPrefix(k, "haber") || strings.HasPrefix(k, "deposito")):
			m.Abono = c
		}
	}
	return m
}

// Completo dice si el mapeo alcanza para leer movimientos.
func (m Mapeo) Completo() bool {
	return m.Fecha != "" && (m.Monto != "" || m.Cargo != "" || m.Abono != "")
}

// MontoCts entiende «1.234,56», «1,234.56», «-80.00», «S/ 80,00» y «(80.00)».
func MontoCts(s string) (int64, error) {
	s = strings.TrimSpace(strings.NewReplacer("S/", "", "s/", "", "PEN", "", " ", "", " ", "").Replace(s))
	if s == "" {
		return 0, nil
	}
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg, s = true, strings.Trim(s, "()")
	}
	if strings.HasPrefix(s, "-") {
		neg, s = true, s[1:]
	} else if strings.HasSuffix(s, "-") {
		neg, s = true, strings.TrimSuffix(s, "-")
	}
	s = strings.TrimPrefix(s, "+")
	coma, punto := strings.LastIndex(s, ","), strings.LastIndex(s, ".")
	switch {
	case coma > punto: // coma decimal
		s = strings.ReplaceAll(s, ".", "")
		s = strings.Replace(s, ",", ".", 1)
	case punto > coma: // punto decimal
		s = strings.ReplaceAll(s, ",", "")
	}
	// «1.234» o «1.234.567»: punto de miles (el dinero no lleva 3 decimales).
	if i := strings.LastIndex(s, "."); strings.Count(s, ".") > 1 || (coma < 0 && i >= 0 && len(s)-i-1 == 3) {
		s = strings.ReplaceAll(s, ".", "")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("monto inválido %q", s)
	}
	v := int64(math.Round(f * 100))
	if neg {
		v = -v
	}
	return v, nil
}

// Fecha entiende DD/MM/AAAA, DD-MM-AAAA, DD/MM/AA, AAAA-MM-DD y MM-DD-AA (formato por defecto de Excel).
func Fecha(s string) (string, error) {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " T"); i > 0 {
		s = s[:i]
	}
	for _, f := range []string{"2006-01-02", "02/01/2006", "02-01-2006", "02/01/06", "02.01.2006", "01-02-06", "2/1/2006", "2/1/06"} {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	// Número de serie de Excel.
	if n, err := strconv.ParseFloat(s, 64); err == nil && n > 30000 && n < 80000 {
		return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(n)).Format("2006-01-02"), nil
	}
	return "", fmt.Errorf("fecha inválida %q", s)
}

// Aplicar convierte filas en movimientos con el mapeo. Las filas sin fecha ni monto se saltan (subtotales).
func Aplicar(cab []string, filas [][]string, m Mapeo) ([]Mov, []string) {
	idx := map[string]int{}
	for i, c := range cab {
		idx[c] = i
	}
	col := func(f []string, nombre string) string {
		i, ok := idx[nombre]
		if !ok || nombre == "" || i >= len(f) {
			return ""
		}
		return strings.TrimSpace(f[i])
	}
	var movs []Mov
	var errs []string
	for n, f := range filas {
		fechaTxt := col(f, m.Fecha)
		if fechaTxt == "" {
			continue
		}
		fecha, err := Fecha(fechaTxt)
		if err != nil {
			errs = append(errs, fmt.Sprintf("fila %d: %v", n+2, err))
			continue
		}
		var monto int64
		if m.Monto != "" {
			monto, err = MontoCts(col(f, m.Monto))
		} else {
			var c, a int64
			c, err = MontoCts(col(f, m.Cargo))
			if err == nil {
				a, err = MontoCts(col(f, m.Abono))
			}
			monto = a - abs(c)
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("fila %d: %v", n+2, err))
			continue
		}
		if monto == 0 {
			continue
		}
		movs = append(movs, Mov{Fecha: fecha, Descripcion: col(f, m.Descripcion), MontoCts: monto, Codigo: col(f, m.Codigo)})
	}
	sort.SliceStable(movs, func(i, j int) bool { return movs[i].Fecha < movs[j].Fecha })
	return movs, errs
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// SaldoFinal: el saldo de la fila más reciente (la columna Saldo del extracto), si el mapeo la trae.
func SaldoFinal(cab []string, filas [][]string, m Mapeo) (int64, bool) {
	if m.Saldo == "" || m.Fecha == "" {
		return 0, false
	}
	iF, iS := -1, -1
	for i, c := range cab {
		if c == m.Fecha {
			iF = i
		}
		if c == m.Saldo {
			iS = i
		}
	}
	if iF < 0 || iS < 0 {
		return 0, false
	}
	type fs struct {
		fecha string
		saldo int64
	}
	var validas []fs
	for _, f := range filas {
		if iF >= len(f) || iS >= len(f) {
			continue
		}
		fe, err := Fecha(f[iF])
		if err != nil || strings.TrimSpace(f[iS]) == "" {
			continue
		}
		sa, err := MontoCts(f[iS])
		if err != nil {
			continue
		}
		validas = append(validas, fs{fe, sa})
	}
	if len(validas) == 0 {
		return 0, false
	}
	asc := validas[0].fecha <= validas[len(validas)-1].fecha
	mejor := validas[0]
	for _, v := range validas {
		if v.fecha > mejor.fecha || (v.fecha == mejor.fecha && asc) {
			mejor = v
		}
	}
	return mejor.saldo, true
}
