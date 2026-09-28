package reparto

import "testing"

// Caso del dueño: recibo general S/ 5.000 − departamentos S/ 4.800 = S/ 200 de áreas comunes,
// repartidos por participación: 8,40 (4,20 %) · 8,00 (4,00 %) · 8,80 (4,40 %).
func TestCasoDelDueno5000_4800_200(t *testing.T) {
	res, err := Medidores(SedapalSetiembreCts, SedapalSetiembreLitros, UnidadesDemo(ConsumoSetiembreDemo))
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalUnidadesCts != 480000 {
		t.Fatalf("departamentos = %d, quiero 480000 (S/ 4.800,00)", res.TotalUnidadesCts)
	}
	if res.DiferenciaCts != 20000 {
		t.Fatalf("diferencia = %d, quiero 20000 (S/ 200,00)", res.DiferenciaCts)
	}
	esperadoComun := map[int64]int64{42000: 840, 40000: 800, 44000: 880}
	var sumaComun, sumaTotal int64
	for _, l := range res.Lineas {
		p := ParticipacionDemo[l.Unidad]
		if l.ComunCts != esperadoComun[p] {
			t.Errorf("unidad %s: común = %d, quiero %d", l.Unidad, l.ComunCts, esperadoComun[p])
		}
		sumaComun += l.ComunCts
		sumaTotal += l.TotalCts
	}
	if sumaComun != 20000 {
		t.Errorf("común suma %d, quiero 20000", sumaComun)
	}
	if sumaTotal != 500000 {
		t.Errorf("todo suma %d, quiero 500000 (S/ 5.000,00)", sumaTotal)
	}
	// Dpto 201: 14,000 m³ → S/ 196,00 + S/ 8,40.
	for _, l := range res.Lineas {
		if l.Unidad == "201" && (l.PropioCts != 19600 || l.ComunCts != 840 || l.Consumo != "14.000") {
			t.Errorf("201: %+v", l)
		}
		if l.Unidad == "402" && l.PropioCts != 74000 {
			t.Errorf("402 (fuga): propio %d, quiero 74000", l.PropioCts)
		}
		if (l.Unidad == "104" || l.Unidad == "503") && l.PropioCts != 4600 {
			t.Errorf("%s: propio %d, quiero 4600", l.Unidad, l.PropioCts)
		}
	}
}

// Participaciones 33,3333 % × 3 y diferencia de S/ 100: las tres partes suman exactamente S/ 100,00.
func TestMayorResiduoTercios(t *testing.T) {
	partes := PorPesos(10000, []int64{333333, 333333, 333334})
	var s int64
	for _, p := range partes {
		s += p
	}
	if s != 10000 {
		t.Fatalf("suma %d, quiero 10000: %v", s, partes)
	}
	if partes[0] != 3333 || partes[1] != 3333 || partes[2] != 3334 {
		t.Errorf("partes = %v", partes)
	}
	// Tres pesos iguales: el céntimo que sobra va a una sola unidad, nunca se pierde.
	partes = PorPesos(10000, []int64{1, 1, 1})
	if partes[0]+partes[1]+partes[2] != 10000 {
		t.Errorf("iguales: %v", partes)
	}
}

func TestCuotasDemo16800(t *testing.T) {
	pesos := make([]int64, 0, 24)
	for _, c := range CodigosDemo {
		pesos = append(pesos, ParticipacionDemo[c])
	}
	partes := PorPesos(1680000, pesos)
	var s int64
	for i, c := range CodigosDemo {
		s += partes[i]
		if c == "201" && partes[i] != 70560 {
			t.Errorf("cuota 201 = %d, quiero 70560", partes[i])
		}
	}
	if s != 1680000 {
		t.Errorf("cuotas suman %d", s)
	}
}

func TestDiferenciaNegativa(t *testing.T) {
	us := []Unidad{{UnidadID: 1, Codigo: "101", ConsumoLitros: 20000, Participacion: 50}, {UnidadID: 2, Codigo: "102", ConsumoLitros: 20000, Participacion: 50}}
	if _, err := Medidores(28000, 30000, us); err != ErrDiferenciaNegativa {
		t.Fatalf("quiero ErrDiferenciaNegativa, obtuve %v", err)
	}
}
