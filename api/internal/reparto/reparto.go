// Package reparto contiene las funciones puras de dinero: reparto por pesos con el método del
// mayor residuo y el reparto de medidores (recibo general − departamentos = áreas comunes).
// Todo en céntimos enteros; los consumos en milésimas de m³ (litros).
package reparto

import (
	"errors"
	"math/big"
	"sort"
)

// PorPesos reparte total entre los pesos dados de modo que la suma sea exactamente total.
// Cada parte es floor(total·peso/Σpesos); los céntimos que sobran van, uno a uno, a las partes
// con mayor residuo (empate: la de menor índice). Nunca se pierde un céntimo.
func PorPesos(total int64, pesos []int64) []int64 {
	partes := make([]int64, len(pesos))
	if len(pesos) == 0 || total == 0 {
		return partes
	}
	var suma int64
	for _, p := range pesos {
		suma += p
	}
	if suma == 0 {
		return partes
	}
	type residuo struct {
		i int
		r int64
	}
	residuos := make([]residuo, len(pesos))
	var asignado int64
	bt, bs := big.NewInt(total), big.NewInt(suma)
	for i, p := range pesos {
		num := new(big.Int).Mul(bt, big.NewInt(p))
		q, r := new(big.Int).QuoRem(num, bs, new(big.Int))
		partes[i] = q.Int64()
		residuos[i] = residuo{i, r.Int64()}
		asignado += partes[i]
	}
	sobra := total - asignado
	sort.SliceStable(residuos, func(a, b int) bool { return residuos[a].r > residuos[b].r })
	for k := int64(0); k < sobra; k++ {
		partes[residuos[k%int64(len(residuos))].i]++
	}
	return partes
}

// Unidad es una unidad que participa del reparto de un medidor.
type Unidad struct {
	UnidadID      int64  `json:"unidad_id"`
	Codigo        string `json:"unidad"`
	ConsumoLitros int64  `json:"-"`
	// Participación en diezmilésimas de punto porcentual: 4,20 % → 42000.
	Participacion int64 `json:"-"`
}

// Linea es el resultado del reparto para una unidad.
type Linea struct {
	UnidadID  int64  `json:"unidad_id"`
	Unidad    string `json:"unidad"`
	Consumo   string `json:"consumo"`
	PropioCts int64  `json:"propio_cts"`
	ComunCts  int64  `json:"comun_cts"`
	TotalCts  int64  `json:"total_cts"`
}

// Resultado del reparto de un recibo general.
type Resultado struct {
	TarifaCtsX1000   int64   `json:"tarifa_cts_x_1000"`
	TotalUnidadesCts int64   `json:"total_unidades_cts"`
	DiferenciaCts    int64   `json:"diferencia_cts"`
	MontoGeneralCts  int64   `json:"monto_general_cts"`
	Lineas           []Linea `json:"lineas"`
}

// ErrDiferenciaNegativa: los departamentos suman más que el recibo general.
var ErrDiferenciaNegativa = errors.New("DIFERENCIA_NEGATIVA")

// ErrSinConsumo: el recibo general no trae consumo.
var ErrSinConsumo = errors.New("SIN_CONSUMO_GENERAL")

// Medidores reparte un recibo general:
//
//	tarifa (milésimas de céntimo por m³) = monto_general / consumo_general
//	propio de cada unidad = consumo × tarifa, redondeado al céntimo
//	diferencia = monto_general − Σ propios  (áreas comunes: riego, limpieza…)
//	la diferencia se reparte por participación con el mayor residuo.
//
// La suma de todas las líneas es exactamente el monto general.
func Medidores(montoGeneralCts, consumoGeneralLitros int64, unidades []Unidad) (Resultado, error) {
	if consumoGeneralLitros <= 0 {
		return Resultado{}, ErrSinConsumo
	}
	// tarifa ×1000 = monto·10⁶ / litros  (monto en céntimos, litros = m³·1000)
	tarifa := redondeoDiv(new(big.Int).Mul(big.NewInt(montoGeneralCts), big.NewInt(1_000_000)), big.NewInt(consumoGeneralLitros))
	res := Resultado{TarifaCtsX1000: tarifa, MontoGeneralCts: montoGeneralCts}
	pesos := make([]int64, len(unidades))
	for i, u := range unidades {
		// propio (céntimos) = litros × tarifa×1000 / 10⁶
		propio := redondeoDiv(new(big.Int).Mul(big.NewInt(u.ConsumoLitros), big.NewInt(tarifa)), big.NewInt(1_000_000))
		if propio < 0 {
			propio = 0
		}
		res.TotalUnidadesCts += propio
		res.Lineas = append(res.Lineas, Linea{UnidadID: u.UnidadID, Unidad: u.Codigo, Consumo: litrosTexto(u.ConsumoLitros), PropioCts: propio})
		pesos[i] = u.Participacion
	}
	res.DiferenciaCts = montoGeneralCts - res.TotalUnidadesCts
	if res.DiferenciaCts < 0 {
		return res, ErrDiferenciaNegativa
	}
	comun := PorPesos(res.DiferenciaCts, pesos)
	for i := range res.Lineas {
		res.Lineas[i].ComunCts = comun[i]
		res.Lineas[i].TotalCts = res.Lineas[i].PropioCts + comun[i]
	}
	return res, nil
}

// redondeoDiv divide redondeando la mitad hacia arriba (valores no negativos).
func redondeoDiv(num, den *big.Int) int64 {
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	if new(big.Int).Mul(r, big.NewInt(2)).Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	return q.Int64()
}

func litrosTexto(l int64) string {
	s := big.NewInt(l).String()
	neg := false
	if l < 0 {
		neg = true
		s = s[1:]
	}
	for len(s) < 4 {
		s = "0" + s
	}
	out := s[:len(s)-3] + "." + s[len(s)-3:]
	if neg {
		out = "-" + out
	}
	return out
}
