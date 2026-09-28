package reparto

// Datos del Edificio Demo (design/DISENO.md y §3 · 08 de la guía). Los usan la semilla y las pruebas,
// así la prueba del caso del dueño y la base sembrada no pueden divergir.

// ParticipacionDemo en diezmilésimas de punto porcentual (4,20 % = 42000).
var ParticipacionDemo = map[string]int64{
	"101": 42000, "102": 40000, "103": 42000, "104": 42000,
	"201": 42000, "202": 40000, "203": 42000, "204": 42000,
	"301": 42000, "302": 40000, "303": 42000, "304": 42000,
	"401": 42000, "402": 40000, "403": 42000, "404": 42000,
	"501": 42000, "502": 40000, "503": 42000, "504": 42000,
	"601": 44000, "602": 40000, "603": 42000, "604": 44000,
}

// CodigosDemo en orden de ronda (piso por piso).
var CodigosDemo = []string{
	"101", "102", "103", "104", "201", "202", "203", "204", "301", "302", "303", "304",
	"401", "402", "403", "404", "501", "502", "503", "504", "601", "602", "603", "604",
}

// ConsumoSetiembreDemo en litros (m³ × 1000). Suman 342.857 m³ y, a la tarifa efectiva de
// S/ 5.000 ÷ 357,143 m³, los 24 cargos suman exactamente S/ 4.800,00.
// 402 tiene la fuga (pico); 104 y 503 casi no consumen; 201 = 14,000 m³ → S/ 196,00.
var ConsumoSetiembreDemo = map[string]int64{
	"101": 12000, "102": 11500, "103": 13000, "104": 3286,
	"201": 14000, "202": 12500, "203": 14500, "204": 13000,
	"301": 15000, "302": 13214, "303": 12000, "304": 13500,
	"401": 14000, "402": 52857, "403": 12500, "404": 13000,
	"501": 18500, "502": 11000, "503": 3286, "504": 13214,
	"601": 18000, "602": 12000, "603": 14000, "604": 13000,
}

// Recibo general de Sedapal de setiembre 2026.
const (
	SedapalSetiembreCts    int64 = 500000 // S/ 5.000,00
	SedapalSetiembreLitros int64 = 357143 // 357,143 m³
)

// UnidadesDemo arma la entrada del reparto con un consumo dado.
func UnidadesDemo(consumo map[string]int64) []Unidad {
	us := make([]Unidad, 0, len(CodigosDemo))
	for i, c := range CodigosDemo {
		us = append(us, Unidad{UnidadID: int64(i + 1), Codigo: c, ConsumoLitros: consumo[c], Participacion: ParticipacionDemo[c]})
	}
	return us
}
