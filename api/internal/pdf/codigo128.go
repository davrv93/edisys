package pdf

// Código de barras Code 128 (juego B) para el número del recibo (bloque I1). Lo leen las lectoras
// de ventanilla del banco o del conserje; el juego B cubre letras, cifras y guiones.

import "fmt"

// patrones128: anchos barra/espacio de cada símbolo 0–106 (106 = parada, con su barra final de 2).
var patrones128 = [...]string{
	"212222", "222122", "222221", "121223", "121322", "131222", "122213", "122312", "132212", "221213",
	"221312", "231212", "112232", "122132", "122231", "113222", "123122", "123221", "223211", "221132",
	"221231", "213212", "223112", "312131", "311222", "321122", "321221", "312212", "322112", "322211",
	"212123", "212321", "232121", "111323", "131123", "131321", "112313", "132113", "132311", "211313",
	"231113", "231311", "112133", "112331", "132131", "113123", "113321", "133121", "313121", "211331",
	"231131", "213113", "213311", "213131", "311123", "311321", "331121", "312113", "312311", "332111",
	"314111", "221411", "431111", "111224", "111422", "121124", "121421", "141122", "141221", "112214",
	"112412", "122114", "122411", "142112", "142211", "241211", "221114", "413111", "241112", "134111",
	"111242", "121142", "121241", "114212", "124112", "124211", "411212", "421112", "421211", "212141",
	"214121", "412121", "111143", "111341", "131141", "114113", "114311", "411113", "411311", "113141",
	"114131", "311141", "411131", "211412", "211214", "211232", "2331112",
}

const inicioB128 = 104

// Modulos128 devuelve los anchos (en módulos) alternando barra y espacio, empezando por barra.
// Los caracteres fuera del ASCII imprimible (32–126) se reemplazan por «?».
func Modulos128(texto string) []int {
	simbolos := []int{inicioB128}
	suma := inicioB128
	i := 1
	for _, r := range texto {
		if r < 32 || r > 126 {
			r = '?'
		}
		v := int(r) - 32
		simbolos = append(simbolos, v)
		suma += v * i
		i++
	}
	simbolos = append(simbolos, suma%103, 106)
	var out []int
	for _, s := range simbolos {
		for _, c := range patrones128[s] {
			out = append(out, int(c-'0'))
		}
	}
	return out
}

// Barras128 dibuja el código en (x, y) con el alto dado; modulo es el ancho de la barra más fina.
// Deja el texto legible debajo y devuelve el ancho total.
func (d *Doc) Barras128(x, y, alto, modulo float64, texto string) float64 {
	cx := x
	for i, m := range Modulos128(texto) {
		w := float64(m) * modulo
		if i%2 == 0 {
			fmt.Fprintf(d.actual, "0 0 0 rg %.2f %.2f %.2f %.2f re f\n", cx, y, w, alto)
		}
		cx += w
	}
	d.Texto(x, y-10, 8, false, texto)
	return cx - x
}
