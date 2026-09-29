// Package pyjson lee y escribe JSON como lo hace el módulo json de Python:
// conserva el orden de las claves, reescribe los números con el repr de Python
// y vuelca con json.dumps(..., ensure_ascii=False, indent=N) byte a byte.
//
// Existe para que los archivos del índice (faq.json, memoria.json) que escribía
// el motor en Python sigan valiendo tal cual, y para que el texto que llega al
// prompt (json.dumps(filas)) sea idéntico al del motor anterior.
package pyjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Kind es el tipo JSON de un Value.
type Kind int

const (
	Null Kind = iota
	Bool
	Number
	String
	Array
	Object
)

// Value es un valor JSON con el orden de claves preservado.
type Value struct {
	Kind  Kind
	B     bool
	Num   string // literal numérico tal cual llegó (se normaliza al volcar)
	S     string
	Arr   []*Value
	Keys  []string
	Props map[string]*Value
}

// Constructores.
func NewNull() *Value             { return &Value{Kind: Null} }
func NewBool(b bool) *Value       { return &Value{Kind: Bool, B: b} }
func NewString(s string) *Value   { return &Value{Kind: String, S: s} }
func NewInt(n int64) *Value       { return &Value{Kind: Number, Num: strconv.FormatInt(n, 10)} }
func NewFloat(f float64) *Value   { return &Value{Kind: Number, Num: FloatRepr(f)} }
func NewArray(v ...*Value) *Value { return &Value{Kind: Array, Arr: v} }
func NewObject() *Value           { return &Value{Kind: Object, Props: map[string]*Value{}} }

// Get devuelve la propiedad k (nil si no existe o no es objeto).
func (v *Value) Get(k string) *Value {
	if v == nil || v.Kind != Object {
		return nil
	}
	return v.Props[k]
}

// Set asigna k conservando su posición si ya existía (como dict de Python).
func (v *Value) Set(k string, x *Value) {
	if _, ok := v.Props[k]; !ok {
		v.Keys = append(v.Keys, k)
	}
	v.Props[k] = x
}

// Str devuelve el texto si es string, "" en otro caso.
func (v *Value) Str() string {
	if v == nil || v.Kind != String {
		return ""
	}
	return v.S
}

// Truthy replica bool() de Python sobre el valor decodificado.
func (v *Value) Truthy() bool {
	if v == nil {
		return false
	}
	switch v.Kind {
	case Null:
		return false
	case Bool:
		return v.B
	case Number:
		f, err := strconv.ParseFloat(v.Num, 64)
		return err != nil || f != 0
	case String:
		return v.S != ""
	case Array:
		return len(v.Arr) > 0
	case Object:
		return len(v.Keys) > 0
	}
	return false
}

// IsInt dice si el literal es un entero de JSON (Python lo decodifica como int).
func (v *Value) IsInt() bool {
	return v.Kind == Number && !strings.ContainsAny(v.Num, ".eE")
}

// Float devuelve el número como float64.
func (v *Value) Float() (float64, bool) {
	if v == nil || v.Kind != Number {
		return 0, false
	}
	f, err := strconv.ParseFloat(v.Num, 64)
	return f, err == nil
}

// PyStr replica str(x) de Python para el valor decodificado.
func (v *Value) PyStr() string {
	if v == nil {
		return "None"
	}
	switch v.Kind {
	case Null:
		return "None"
	case Bool:
		if v.B {
			return "True"
		}
		return "False"
	case String:
		return v.S
	case Number:
		return numRepr(v.Num)
	}
	// Listas y dicts: Python usa repr con comillas simples; no se da en la
	// práctica (ids y preguntas son escalares). Aproximación razonable.
	return Dumps(v)
}

// ---------- lectura ----------

// Parse decodifica un documento JSON conservando el orden de claves.
func Parse(data []byte) (*Value, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := leer(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("pyjson: datos sobrantes tras el valor JSON")
	}
	return v, nil
}

func leer(dec *json.Decoder) (*Value, error) {
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	return desdeToken(dec, t)
}

func desdeToken(dec *json.Decoder, t json.Token) (*Value, error) {
	switch x := t.(type) {
	case nil:
		return NewNull(), nil
	case bool:
		return NewBool(x), nil
	case json.Number:
		return &Value{Kind: Number, Num: string(x)}, nil
	case string:
		return NewString(x), nil
	case json.Delim:
		switch x {
		case '[':
			arr := NewArray()
			arr.Arr = []*Value{}
			for dec.More() {
				e, err := leer(dec)
				if err != nil {
					return nil, err
				}
				arr.Arr = append(arr.Arr, e)
			}
			_, err := dec.Token() // ]
			return arr, err
		case '{':
			obj := NewObject()
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, _ := kt.(string)
				e, err := leer(dec)
				if err != nil {
					return nil, err
				}
				obj.Set(k, e) // clave repetida: gana la última, como en Python
			}
			_, err := dec.Token() // }
			return obj, err
		}
	}
	return nil, fmt.Errorf("pyjson: token inesperado %v", t)
}

// UnmarshalJSON permite usar *Value dentro de structs.
func (v *Value) UnmarshalJSON(b []byte) error {
	x, err := Parse(b)
	if err != nil {
		return err
	}
	*v = *x
	return nil
}

// MarshalJSON emite el valor como lo haría Starlette (separadores compactos).
func (v *Value) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	volcar(&b, v, -1, 0, ",", ":")
	return []byte(b.String()), nil
}

// ---------- escritura ----------

// Dumps = json.dumps(v, ensure_ascii=False) (separadores ", " y ": ").
func Dumps(v *Value) string {
	var b strings.Builder
	volcar(&b, v, -1, 0, ", ", ": ")
	return b.String()
}

// DumpsIndent = json.dumps(v, ensure_ascii=False, indent=n).
func DumpsIndent(v *Value, n int) string {
	var b strings.Builder
	volcar(&b, v, n, 0, ",", ": ")
	return b.String()
}

func volcar(b *strings.Builder, v *Value, indent, nivel int, sepItem, sepClave string) {
	if v == nil {
		b.WriteString("null")
		return
	}
	salto := func(n int) {
		if indent >= 0 {
			b.WriteByte('\n')
			b.WriteString(strings.Repeat(" ", indent*n))
		}
	}
	switch v.Kind {
	case Null:
		b.WriteString("null")
	case Bool:
		if v.B {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case Number:
		b.WriteString(numRepr(v.Num))
	case String:
		b.WriteString(Quote(v.S))
	case Array:
		if len(v.Arr) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, e := range v.Arr {
			if i > 0 {
				b.WriteString(sepItem)
			}
			salto(nivel + 1)
			volcar(b, e, indent, nivel+1, sepItem, sepClave)
		}
		salto(nivel)
		b.WriteByte(']')
	case Object:
		if len(v.Keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, k := range v.Keys {
			if i > 0 {
				b.WriteString(sepItem)
			}
			salto(nivel + 1)
			b.WriteString(Quote(k))
			b.WriteString(sepClave)
			volcar(b, v.Props[k], indent, nivel+1, sepItem, sepClave)
		}
		salto(nivel)
		b.WriteByte('}')
	}
}

// Quote escapa como json.dumps(ensure_ascii=False).
func Quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		i += n
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// numRepr normaliza un literal JSON a como lo reescribe Python:
// enteros tal cual (int arbitrario), flotantes con repr(float).
func numRepr(lit string) string {
	if !strings.ContainsAny(lit, ".eE") {
		// int de Python: "-0" → "0"; sin ceros a la izquierda en JSON válido.
		if lit == "-0" {
			return "0"
		}
		return lit
	}
	f, err := strconv.ParseFloat(lit, 64)
	if err != nil { // desborde: Python da inf, que json.dumps escribe "Infinity"
		if math.IsInf(f, 1) {
			return "Infinity"
		}
		if math.IsInf(f, -1) {
			return "-Infinity"
		}
		return lit
	}
	return FloatRepr(f)
}

// FloatRepr = repr(float) de Python: dígitos mínimos, notación fija si el
// exponente decimal está en [-4, 16), si no científica con dos dígitos.
func FloatRepr(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	e := strconv.FormatFloat(f, 'e', -1, 64) // p. ej. -1.2345e-05
	signo := ""
	if strings.HasPrefix(e, "-") {
		signo, e = "-", e[1:]
	}
	mant, expS, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(expS)
	digitos := strings.Replace(mant, ".", "", 1)
	if -4 <= exp && exp < 16 {
		var s string
		if exp >= 0 {
			if len(digitos) <= exp+1 {
				s = digitos + strings.Repeat("0", exp+1-len(digitos)) + ".0"
			} else {
				s = digitos[:exp+1] + "." + digitos[exp+1:]
			}
		} else {
			s = "0." + strings.Repeat("0", -exp-1) + digitos
		}
		return signo + s
	}
	m := digitos[:1]
	if len(digitos) > 1 {
		m += "." + digitos[1:]
	}
	es := "+"
	if exp < 0 {
		es, exp = "-", -exp
	}
	return fmt.Sprintf("%s%se%s%02d", signo, m, es, exp)
}
