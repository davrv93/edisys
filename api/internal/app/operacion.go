package app

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
	"edisys/api/internal/whatsapp"
)

// Bloque G · Operación del edificio: cuaderno de ocurrencias (G1), tickets con SLA (G2),
// visitas con QR (G3), parking (G4) y paquetes (G5). Todo interno, sin terceros.

// rutasOperacion registra las rutas del bloque G (se llama desde los dos grupos de servidor.go).
func (s *Server) rutasOperacion(r chi.Router) {
	q := s.requiere
	// Unidades para los selectores de portería (el conserje no tiene unidades.ver).
	r.Get("/operacion/unidades", s.unidadesOperacion)

	// G1 · cuaderno de ocurrencias
	r.With(q("ocurrencias.ver")).Get("/ocurrencias", s.listarOcurrencias)
	r.With(q("ocurrencias.registrar")).Post("/ocurrencias", s.crearOcurrencia)
	r.With(q("ocurrencias.registrar")).Post("/ocurrencias/{id}/cerrar", s.cerrarOcurrencia)
	r.With(q("ocurrencias.registrar")).Post("/ocurrencias/{id}/escalar", s.escalarOcurrencia)

	// G2 · tickets con SLA (las incidencias del tablero)
	r.With(q("incidencias.ver")).Get("/tickets", s.listarTickets)
	r.With(q("incidencias.ver")).Get("/tickets/sla", s.verConfigSLA)
	r.With(q("tickets.configurar")).Put("/tickets/sla", s.guardarConfigSLA)

	// G3 · visitas e identificación QR
	r.With(q("visitas.ver")).Get("/visitas", s.listarVisitas)
	r.With(q("visitas.autorizar")).Post("/visitas", s.crearVisita)
	r.With(q("visitas.ver")).Get("/visitas/{id}/qr", s.qrVisita)
	r.With(q("visitas.autorizar")).Post("/visitas/{id}/anular", s.anularVisita)
	r.With(q("visitas.validar")).Post("/visitas/validar", s.validarVisita)
	r.With(q("visitas.validar")).Post("/visitas/{id}/salida", s.salidaVisita)
	r.With(q("visitas.validar")).Get("/accesos", s.listarAccesos)

	// G4 · parking
	r.With(q("parking.ver")).Get("/parking/estacionamientos", s.listarEstacionamientos)
	r.With(q("parking.administrar")).Post("/parking/estacionamientos", s.crearEstacionamiento)
	r.With(q("parking.administrar")).Put("/parking/estacionamientos/{id}", s.editarEstacionamiento)
	r.With(q("parking.ver")).Get("/parking/sesiones", s.listarSesionesParking)
	r.With(q("parking.operar")).Post("/parking/sesiones", s.entradaParking)
	r.With(q("parking.ver")).Get("/parking/sesiones/{id}/cotizar", s.cotizarParking)
	r.With(q("parking.operar")).Post("/parking/sesiones/{id}/salida", s.salidaParking)

	// G5 · paquetes
	r.With(q("paquetes.ver")).Get("/paquetes", s.listarPaquetes)
	r.With(q("paquetes.registrar")).Post("/paquetes", s.recibirPaquete)
	r.With(q("paquetes.registrar")).Post("/paquetes/{id}/entregar", s.entregarPaquete)
	r.With(q("paquetes.registrar")).Post("/paquetes/{id}/devolver", s.devolverPaquete)
	r.With(q("paquetes.registrar")).Post("/paquetes/{id}/reavisar", s.reavisarPaquete)
}

// errOperacion traduce las reglas duras del bloque G (sin tocar el traductor común).
func errOperacion(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch {
		case pg.Code == "ED010" && strings.HasPrefix(pg.Message, "OCURRENCIA_CERRADA"):
			return P.Conflicto("OCURRENCIA_CERRADA", "Esa ocurrencia ya está cerrada; el cuaderno no se reabre.")
		case pg.Code == "ED010":
			return P.Conflicto("OCURRENCIA_INMUTABLE", "Lo anotado en el cuaderno no se reescribe.")
		case pg.Code == "23505" && pg.ConstraintName == "sesion_parking_espacio_uq":
			return P.Conflicto("ESPACIO_OCUPADO", "Ese estacionamiento ya tiene un vehículo dentro.")
		case pg.Code == "23505" && pg.ConstraintName == "sesion_parking_placa_uq":
			return P.Conflicto("PLACA_DENTRO", "Esa placa ya figura dentro del edificio.")
		case pg.Code == "23505" && pg.ConstraintName == "estacionamiento_uq":
			return P.Conflicto("ESTACIONAMIENTO_EXISTE", "Ya hay un estacionamiento con ese código.")
		}
	}
	return err
}

// unidadesOperacion: GET /operacion/unidades — id, código y residente de cada unidad, para quien
// atiende la portería (visitas, parking o paquetes). No expone teléfonos ni deudas.
func (s *Server) unidadesOperacion(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	if !(e.Puede("visitas.validar") || e.Puede("parking.operar") || e.Puede("paquetes.registrar") || e.Puede("unidades.ver")) {
		P.Fallo(w, r, P.Prohibido("SIN_PERMISO", "Tu rol no tiene permiso para esto.").Con("permiso", "visitas.validar"))
		return
	}
	filas, err := db.Filas(r.Context(), s.DB, `SELECT u.id, u.codigo, COALESCE(pe.nombre,'') AS residente
		FROM unidad u LEFT JOIN LATERAL (SELECT pe.nombre FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id
			WHERE up.unidad_id=u.id AND up.hasta IS NULL ORDER BY CASE up.rol WHEN 'inquilino' THEN 0 ELSE 1 END LIMIT 1) pe ON true
		WHERE u.edificio_id=$1 ORDER BY u.codigo`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// contactoUnidad devuelve el nombre y celular del residente vigente (propietario primero, si no el inquilino).
func contactoUnidad(ctx context.Context, q db.Q, unidad int64) (nombre, celular, codigo string) {
	_ = q.QueryRow(ctx, `SELECT u.codigo, COALESCE(pe.nombre,''), COALESCE(pe.celular,'')
		FROM unidad u LEFT JOIN LATERAL (SELECT pe.nombre, pe.celular FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id
			WHERE up.unidad_id=u.id AND up.hasta IS NULL AND pe.celular <> ''
			ORDER BY CASE up.rol WHEN 'propietario' THEN 0 ELSE 1 END LIMIT 1) pe ON true
		WHERE u.id=$1`, unidad).Scan(&codigo, &nombre, &celular)
	return
}

// avisoSistema encola un WhatsApp «libre» de origen sistema. Valida antes de insertar para no
// abortar la transacción de quien llama: sin teléfono útil devuelve 0 y el registro sigue.
func (s *Server) avisoSistema(ctx context.Context, q db.Q, eid int64, unidad *int64, telefono, texto string) int64 {
	if len(whatsapp.NormalizarTelefono(telefono)) < 9 || strings.TrimSpace(texto) == "" {
		return 0
	}
	id, err := s.encolar(ctx, q, eid, unidad, telefono, "libre", map[string]string{"texto": texto}, "sistema", nil)
	if err != nil {
		return 0
	}
	return id
}
