package app

import "github.com/go-chi/chi/v5"

// Rutas de la gestión de deuda (bloques B3, D1, D2 y D3). Se registran con una sola línea en
// rutasEdificio y en rutasModulosNuevos, como el resto de módulos nuevos.
func (s *Server) rutasDeuda(r chi.Router) {
	q := s.requiere

	// B3 · cuentas por cobrar y estado de cuenta del propietario
	r.With(q("morosidad.ver")).Get("/cuentas-por-cobrar", s.cuentasPorCobrar)
	r.With(q("recibos.ver")).Get("/unidades/{uid}/estado-cuenta", s.estadoCuenta)

	// D1 · acuerdos de pago
	r.With(q("acuerdos.ver")).Get("/acuerdos", s.listarAcuerdos)
	r.With(q("acuerdos.gestionar")).Get("/acuerdos/propuesta", s.propuestaAcuerdo)
	r.With(q("acuerdos.gestionar")).Post("/acuerdos", s.crearAcuerdo)
	r.With(q("acuerdos.ver")).Get("/acuerdos/{id}", s.verAcuerdo)
	r.With(q("acuerdos.gestionar")).Post("/acuerdos/{id}/anular", s.anularAcuerdo)
	r.With(q("acuerdos.gestionar")).Post("/acuerdos/{id}/documento", s.documentoAcuerdo)

	// D2 · avisos de cobranza
	r.With(q("avisos.gestionar")).Get("/avisos-cobranza", s.listarAvisos)
	r.With(q("avisos.gestionar")).Post("/avisos-cobranza", s.crearAviso)
	r.With(q("avisos.gestionar")).Get("/avisos-cobranza/envios", s.listarEnviosAviso)
	r.With(q("avisos.gestionar")).Post("/avisos-cobranza/manual", s.avisoManual)
	r.With(q("avisos.gestionar")).Put("/avisos-cobranza/{id}", s.editarAviso)
	r.With(q("avisos.gestionar")).Delete("/avisos-cobranza/{id}", s.borrarAviso)
	r.With(q("avisos.gestionar")).Post("/avisos-cobranza/{id}/ejecutar", s.ejecutarAvisoAhora)

	// D3 · morosos, morosos detallado y puntualidad; configuración de etiquetas y tasa de mora
	r.With(q("morosidad.ver")).Get("/morosos", s.grillaMorosos)
	r.With(q("morosidad.ver")).Get("/deuda/config", s.verDeudaConfig)
	r.With(q("acuerdos.gestionar")).Put("/deuda/config", s.guardarDeudaConfig)
}
