// Package app es el API HTTP de EDISYS: rutas, middleware de sesión y permisos, y los
// manejadores de cada pantalla (03–11) más WhatsApp, chatbot, analítica y kanban.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"edisys/api/internal/archivo"
	"edisys/api/internal/auth"
	"edisys/api/internal/config"
	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Server reúne las dependencias del API.
type Server struct {
	DB       *pgxpool.Pool
	Almacen  archivo.Almacen
	Firma    archivo.Firmador
	Cfg      config.Config
	HTTP     *http.Client // para Evolution
	limLogin *auth.Limitador
	limLead  *auth.Limitador
	permBase map[string]map[string]bool
}

// Nuevo crea el servidor y carga los permisos base de cada rol.
func Nuevo(ctx context.Context, pool *pgxpool.Pool, alm archivo.Almacen, cfg config.Config) (*Server, error) {
	s := &Server{
		DB: pool, Almacen: alm, Cfg: cfg,
		Firma:    archivo.Firmador{Clave: []byte("archivos:" + cfg.JWTSecret)},
		HTTP:     &http.Client{Timeout: 15 * time.Second},
		limLogin: auth.NuevoLimitador(5, 15*time.Minute),
		limLead:  auth.NuevoLimitador(3, time.Hour),
	}
	if err := s.cargarPermisos(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) cargarPermisos(ctx context.Context) error {
	filas, err := s.DB.Query(ctx, `SELECT rol, permiso FROM rol_permiso`)
	if err != nil {
		return err
	}
	defer filas.Close()
	s.permBase = map[string]map[string]bool{}
	for filas.Next() {
		var rol, perm string
		if err := filas.Scan(&rol, &perm); err != nil {
			return err
		}
		if s.permBase[rol] == nil {
			s.permBase[rol] = map[string]bool{}
		}
		s.permBase[rol][perm] = true
	}
	return filas.Err()
}

// ---------- contexto de la petición ----------

type claveCtx int

const (
	ctxSesion claveCtx = iota
	ctxEdificio
)

// Sesion del usuario autenticado.
type Sesion struct {
	UsuarioID  int64
	Nombre     string
	Correo     string
	Telefono   string
	AdmID      int64
	Superadmin bool
	Roles      map[int64]string // edificio → rol
	PorBearer  bool
}

// Edificios devuelve los ids ordenados.
func (se *Sesion) Edificios() []int64 {
	ids := make([]int64, 0, len(se.Roles))
	for id := range se.Roles {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// Edificio activo de la petición, con rol, permisos efectivos y (propietario/inquilino) sus unidades.
type Edificio struct {
	ID       int64
	Nombre   string
	Rol      string
	Permisos map[string]bool
	Unidades []int64
}

// Puede dice si el rol tiene el permiso en este edificio.
func (e *Edificio) Puede(p string) bool { return e.Permisos[p] }

// SoloLoSuyo: propietario e inquilino ven solo sus unidades (filtro, no permiso: §2.5).
func (e *Edificio) SoloLoSuyo() bool { return e.Rol == "propietario" || e.Rol == "inquilino" }

// EsSuya dice si la unidad es del usuario.
func (e *Edificio) EsSuya(uid int64) bool {
	for _, u := range e.Unidades {
		if u == uid {
			return true
		}
	}
	return false
}

func ses(r *http.Request) *Sesion {
	v, _ := r.Context().Value(ctxSesion).(*Sesion)
	return v
}

func edf(r *http.Request) *Edificio {
	v, _ := r.Context().Value(ctxEdificio).(*Edificio)
	return v
}

// ---------- rutas ----------

// Rutas arma el enrutador completo.
func (s *Server) Rutas() http.Handler {
	r := chi.NewRouter()
	r.Use(s.idPeticion, s.registro, s.recuperar)

	r.Get("/api/health", s.salud)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.salud)
		r.Post("/auth/login", s.login)
		r.Post("/auth/refresh", s.refresh)
		r.Post("/auth/logout", s.logout)
		r.Post("/auth/aceptar-invitacion", s.aceptarInvitacion)
		r.Post("/publico/contacto", s.contacto)
		r.Get("/archivos/{id}", s.servirArchivo)
		r.Post("/whatsapp/webhook", s.webhookWhatsApp)
		r.Get("/publico/marca/{slug}", s.marcaPorSlug) // marca: I2 · el login se pinta con la marca antes de la sesión

		r.Group(func(r chi.Router) {
			r.Use(s.autenticar, s.csrf, s.auditar)
			r.Get("/yo", s.yo)
			r.Get("/roles", s.listarRoles)
			r.Get("/edificios", s.misEdificios)

			// Módulos nuevos con el edificio por defecto del usuario (o ?edificio_id=).
			r.Group(func(r chi.Router) {
				r.Use(s.conEdificio)
				r.Use(s.exigirEdificioActivo) // marca: I5 · desactivado = solo lectura
				s.rutasModulosNuevos(r)
			})

			r.Route("/edificios/{eid}", func(r chi.Router) {
				r.Use(s.conEdificio)
				r.Use(s.exigirEdificioActivo) // marca: I5 · desactivado = solo lectura
				s.rutasEdificio(r)
				s.rutasModulosNuevos(r)
			})
		})
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		P.Fallo(w, r, P.Err(http.StatusNotFound, "RUTA_NO_EXISTE", "La ruta "+r.Method+" "+r.URL.Path+" no existe."))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		P.Fallo(w, r, P.Err(http.StatusMethodNotAllowed, "METODO_NO_PERMITIDO", "Método no permitido en esta ruta."))
	})
	return r
}

func (s *Server) rutasEdificio(r chi.Router) {
	q := s.requiere
	// 06 · ficha, unidades, importación
	r.With(q("edificio.ver")).Get("/", s.verEdificio)
	r.With(q("edificio.editar")).Put("/", s.editarEdificio)
	r.With(q("unidades.ver")).Get("/unidades", s.listarUnidades)
	r.With(q("unidades.editar")).Post("/unidades", s.crearUnidad)
	r.With(q("unidades.ver")).Get("/unidades/{uid}", s.verUnidad)
	r.With(q("unidades.editar")).Put("/unidades/{uid}", s.editarUnidad)
	r.With(q("unidades.editar")).Delete("/unidades/{uid}", s.borrarUnidad)
	r.With(q("unidades.editar")).Post("/unidades/{uid}/personas", s.asignarPersona)
	r.With(q("portal.ver")).Get("/unidades/{uid}/permisos-inquilino", s.verPermisosInquilino)
	r.With(q("portal.ver")).Put("/unidades/{uid}/permisos-inquilino", s.editarPermisosInquilino)
	r.With(q("unidades.importar")).Get("/importaciones/plantilla.xlsx", s.plantillaExcel)
	r.With(q("unidades.importar")).Post("/importaciones", s.importarExcel)
	r.With(q("unidades.importar")).Post("/importaciones/{iid}/confirmar", s.confirmarImportacion)

	// 03 · dashboard y 04 · balance
	r.With(q("dashboard.ver")).Get("/dashboard", s.dashboard)
	r.With(q("balance.ver")).Get("/balance", s.balance)
	r.With(q("balance.ver")).Get("/balance/nodos/{nodo}", s.balanceNodo)
	r.With(q("balance.ver")).Get("/balance/documentos/{doc}", s.balanceDocumento)
	r.With(q("balance.ver")).Get("/balance/{periodo}.pdf", s.pdfBalance)
	r.With(q("balance.ver")).Get("/balance/{periodo}/informe-junta.pdf", s.pdfInformeJunta)
	r.With(q("recibos.emitir")).Post("/balance/{periodo}/enviar-correo", s.enviarBalanceCorreo)
	r.With(q("recibos.emitir")).Get("/correo/mensajes", s.listarCorreos)

	// 14 · conciliación bancaria
	r.With(q("balance.conciliar")).Get("/conciliacion", s.verConciliacion)
	r.With(q("balance.conciliar")).Get("/conciliacion/extracto-demo.csv", s.extractoDemo)
	r.With(q("balance.conciliar")).Post("/conciliacion/columnas", s.columnasExtracto)
	r.With(q("balance.conciliar")).Post("/conciliacion/extractos", s.subirExtracto)
	r.With(q("balance.conciliar")).Post("/conciliacion/confirmar-sugeridos", s.confirmarSugeridos)
	r.With(q("balance.conciliar")).Post("/conciliacion/movimientos/{mid}/confirmar", s.confirmarMovimiento)
	r.With(q("balance.conciliar")).Post("/conciliacion/movimientos/{mid}/deshacer", s.deshacerMovimiento)
	r.With(q("balance.conciliar")).Post("/conciliacion/movimientos/{mid}/crear-egreso", s.crearEgresoDesdeMovimiento)
	r.With(q("balance.conciliar")).Post("/conciliacion/movimientos/{mid}/crear-ingreso", s.crearIngresoDesdeMovimiento)
	r.With(q("balance.ver")).Get("/rubros", s.listarRubros)
	r.With(q("balance.ver")).Get("/egresos", s.listarEgresos)
	r.With(q("egresos.registrar")).Post("/egresos", s.crearEgreso)

	// Proveedores y cuentas por pagar (bloque B)
	r.With(q("proveedores.ver")).Get("/proveedores", s.listarProveedores)
	r.With(q("proveedores.administrar")).Post("/proveedores", s.crearProveedor)
	r.With(q("proveedores.ver")).Get("/proveedores/{pid}", s.verProveedor)
	r.With(q("proveedores.administrar")).Put("/proveedores/{pid}", s.editarProveedor)
	r.With(q("proveedores.administrar")).Delete("/proveedores/{pid}", s.borrarProveedor)
	r.With(q("cuentas_pagar.ver")).Get("/cuentas-por-pagar", s.listarCuentasPorPagar)
	r.With(q("cuentas_pagar.registrar")).Post("/cuentas-por-pagar", s.crearCuentaPorPagar)
	r.With(q("cuentas_pagar.ver")).Get("/cuentas-por-pagar/{cid}", s.verCuentaPorPagar)
	r.With(q("cuentas_pagar.registrar")).Post("/cuentas-por-pagar/{cid}/pagos", s.pagarCuentaPorPagar)

	// Fondos y trazabilidad (bloque C)
	r.With(q("fondos.ver")).Get("/fondos", s.listarFondos)
	r.With(q("fondos.ver")).Get("/fondos/trazabilidad", s.trazabilidadFondos)
	r.With(q("fondos.ver")).Get("/fondos/{fid}/movimientos", s.movimientosFondo)
	r.With(q("fondos.administrar")).Post("/fondos", s.crearFondo)
	r.With(q("fondos.administrar")).Put("/fondos/{fid}", s.editarFondo)
	r.With(q("fondos.administrar")).Delete("/fondos/{fid}", s.desactivarFondo)
	r.With(q("fondos.administrar")).Post("/fondos/{fid}/movimientos", s.movimientoManual)
	r.With(q("fondos.administrar")).Post("/fondos/transferencia", s.transferenciaFondos)

	// Informes económicos y consumos (bloque C)
	r.With(q("balance.ver")).Get("/informes/economico", s.informeEconomico)
	r.With(q("balance.ver")).Get("/informes/consumos", s.consumosPorDepartamento)

	// B4 · recibos e ingresos externos
	r.With(q("externos.ver")).Get("/recibos-externos", s.listarRecibosExternos)
	r.With(q("externos.registrar")).Post("/recibos-externos", s.crearReciboExterno)
	r.With(q("externos.registrar")).Post("/recibos-externos/{id}/pagar", s.pagarReciboExterno)
	r.With(q("externos.ver")).Get("/ingresos-externos", s.listarIngresosExternos)
	r.With(q("externos.registrar")).Post("/ingresos-externos", s.crearIngresoExterno)

	// A3 · cuentas bancarias y vouchers multicuenta
	r.With(q("cuentas_bancarias.ver")).Get("/cuentas-bancarias", s.listarCuentasBancarias)
	r.With(q("cuentas_bancarias.administrar")).Post("/cuentas-bancarias", s.crearCuentaBancaria)
	r.With(q("cuentas_bancarias.administrar")).Put("/cuentas-bancarias/{id}", s.editarCuentaBancaria)
	r.Post("/unidades/{uid}/vouchers", s.registrarVoucher)

	// A4 · cobranzas sin identificar y devoluciones
	r.With(q("cobranzas.ver")).Get("/cobranzas-sin-identificar", s.listarCobranzas)
	r.With(q("cobranzas.gestionar")).Post("/cobranzas-sin-identificar", s.crearCobranza)
	r.With(q("cobranzas.gestionar")).Post("/cobranzas-sin-identificar/{id}/imputar", s.imputarCobranza)
	r.With(q("cobranzas.gestionar")).Post("/cobranzas-sin-identificar/{id}/devolver", s.devolverCobranza)
	r.With(q("cobranzas.ver")).Get("/devoluciones", s.listarDevoluciones)

	// E1 · documentos por categorías
	r.With(q("documentos.ver")).Get("/documentos/categorias", s.listarDocumentoCategorias)
	r.With(q("documentos.administrar")).Post("/documentos/categorias", s.crearDocumentoCategoria)
	r.With(q("documentos.administrar")).Delete("/documentos/categorias/{id}", s.borrarDocumentoCategoria)
	r.With(q("documentos.ver")).Get("/documentos", s.listarDocumentos)
	r.With(q("documentos.administrar")).Post("/documentos", s.crearDocumento)
	r.With(q("documentos.administrar")).Post("/documentos/{id}/publicar", s.publicarDocumento)

	// marca: I1/I2/I5 · plantilla de recibo, marca blanca y configuración del edificio
	r.With(q("recibos.plantilla")).Get("/plantilla-recibo", s.verPlantillaRecibo)
	r.With(q("recibos.plantilla")).Put("/plantilla-recibo", s.guardarPlantillaRecibo)
	r.With(q("recibos.plantilla")).Post("/plantilla-recibo/vista-previa", s.vistaPreviaRecibo)
	r.With(q("marca.configurar")).Get("/marca", s.verMarca)
	r.With(q("marca.configurar")).Put("/marca", s.guardarMarca)
	r.With(q("marca.configurar")).Post("/marca/logo", s.subirLogo)
	r.With(q("marca.configurar")).Delete("/marca/logo", s.quitarLogo)
	r.With(q("configuracion.ver")).Get("/configuracion/estado", s.verEstadoEdificio)
	r.With(q("configuracion.editar")).Put("/configuracion/activo", s.cambiarActivoEdificio)
	r.With(q("configuracion.ver")).Get("/configuracion/asistentes", s.asistenteConfiguracion)
	r.With(q("configuracion.ver")).Get("/configuracion/cambios", s.registroCambios)

	// 05 · recibos y pagos
	r.With(q("recibos.ver")).Get("/periodos", s.listarPeriodos)
	r.With(q("periodos.administrar")).Post("/periodos", s.abrirPeriodo)
	r.With(q("periodos.administrar")).Get("/periodos/{p}/presupuesto", s.verPresupuesto)
	r.With(q("periodos.administrar")).Put("/periodos/{p}/presupuesto", s.guardarPresupuesto)
	r.With(q("periodos.administrar")).Get("/periodos/{p}/presupuesto/plantilla.xlsx", s.plantillaPresupuesto)
	r.With(q("periodos.administrar")).Post("/periodos/{p}/presupuesto/importar", s.importarPresupuesto)
	r.With(q("recibos.emitir")).Post("/periodos/{p}/recibos/generar", s.generarRecibos)
	r.With(q("recibos.emitir")).Post("/periodos/{p}/recibos/emitir", s.emitirRecibos)
	r.With(q("recibos.ver")).Get("/recibos", s.listarRecibos)
	r.With(q("recibos.ver")).Get("/recibos/{rid}", s.verRecibo)
	r.With(q("recibos.ver")).Get("/recibos/{rid}/pdf", s.pdfRecibo)
	r.With(q("recibos.emitir")).Post("/recibos/enviar", s.enviarRecibos)
	r.Post("/recibos/{rid}/pagos", s.registrarPago) // pagos.registrar o pagos.informar (se valida dentro)
	r.With(q("recibos.emitir")).Post("/recibos/{rid}/anular", s.anularRecibo)
	r.With(q("recibos.emitir")).Post("/recibos/{rid}/enviar-correo", s.enviarRecibosCorreo) // {rid} = periodo AAAA-MM
	r.With(q("recibos.ver")).Get("/recibos/{rid}/comprobante", s.comprobantesDeRecibo)
	r.With(q("comprobantes.emitir")).Post("/recibos/{rid}/comprobante", s.emitirComprobante)

	// SUNAT · facturación electrónica
	r.With(q("facturacion.configurar")).Get("/facturacion/config", s.verConfigFacturacion)
	r.With(q("facturacion.configurar")).Put("/facturacion/config", s.guardarConfigFacturacion)
	r.With(q("facturacion.configurar")).Post("/facturacion/certificado", s.subirCertificado)
	r.With(q("comprobantes.emitir")).Get("/comprobantes", s.listarComprobantes)
	r.With(q("recibos.ver")).Get("/comprobantes/{cid}", s.verComprobante)
	r.With(q("recibos.ver")).Get("/comprobantes/{cid}/xml", s.xmlComprobante)
	r.With(q("recibos.ver")).Get("/comprobantes/{cid}/cdr", s.xmlComprobante)
	r.With(q("recibos.ver")).Get("/comprobantes/{cid}/pdf", s.pdfComprobante)
	r.With(q("comprobantes.emitir")).Post("/comprobantes/{cid}/anular", s.anularComprobante)
	r.With(q("pagos.validar")).Get("/pagos", s.listarPagos)
	r.With(q("pagos.validar")).Patch("/pagos/{pid}", s.validarPago)
	r.With(q("morosidad.ver")).Get("/morosidad", s.morosidad)
	r.With(q("pagos.registrar")).Post("/unidades/{uid}/pagos", s.pagoACuenta)
	r.With(q("recibos.ver")).Get("/unidades/{uid}/cuenta", s.cuentaCorriente)

	// 07 · reservas
	r.With(q("reservas.ver")).Get("/areas", s.listarAreas)
	r.With(q("areas.administrar")).Post("/areas", s.crearArea)
	r.With(q("reservas.ver")).Get("/areas/{aid}", s.verArea)
	r.With(q("areas.administrar")).Put("/areas/{aid}", s.editarArea)
	r.With(q("reservas.ver")).Get("/disponibilidad", s.disponibilidad)
	r.With(q("reservas.ver")).Get("/reservas", s.listarReservas)
	r.With(q("reservas.crear")).Post("/reservas", s.crearReserva)
	r.With(q("reservas.crear")).Post("/reservas/{rid}/pago", s.pagarReserva)
	r.With(q("reservas.administrar")).Patch("/reservas/{rid}", s.cambiarReserva)
	r.With(q("reservas.crear")).Delete("/reservas/{rid}", s.cancelarReserva)

	// 08 · medidores
	r.With(q("lecturas.ver")).Get("/lecturas", s.listarLecturas)
	r.With(q("lecturas.registrar")).Post("/medidores/{mid}/lecturas", s.registrarLectura)
	r.With(q("lecturas.corregir")).Put("/lecturas/{lid}", s.corregirLectura)
	r.With(q("recibos.emitir")).Get("/ajustes", s.listarAjustes)
	r.With(q("lecturas.aprobar_reparto")).Post("/periodos/{p}/recibo-general", s.registrarReciboGeneral)
	r.With(q("lecturas.aprobar_reparto")).Get("/periodos/{p}/recibo-general", s.verReciboGeneral)
	r.With(q("lecturas.aprobar_reparto")).Post("/periodos/{p}/reparto-medidores/calcular", s.calcularReparto)
	r.With(q("lecturas.aprobar_reparto")).Post("/periodos/{p}/reparto-medidores/aprobar", s.aprobarReparto)

	// 09 · mantenimiento (rutas de la guía)
	r.With(q("incidencias.reportar")).Post("/incidencias", s.reportarIncidencia)
	r.With(q("incidencias.ver")).Get("/incidencias", s.listarIncidencias)
	r.With(q("incidencias.ver")).Get("/trabajos", s.tablero)
	r.With(q("incidencias.ver")).Get("/trabajos/{tid}", s.verTrabajo)
	r.With(q("incidencias.validar")).Post("/incidencias/{tid}/validar", s.validarIncidencia)
	r.With(q("trabajos.presupuestar")).Post("/trabajos/{tid}/informe", s.informeTrabajo)
	r.With(q("trabajos.votar")).Post("/trabajos/{tid}/votos", s.votarTrabajo)
	r.With(q("trabajos.ejecutar")).Post("/trabajos/{tid}/avance", s.avanceTrabajo)
	r.With(q("incidencias.ver")).Patch("/incidencias/{tid}/estado", s.cambiarEstadoIncidencia)
	r.With(q("incidencias.ver")).Get("/reglas-aprobacion", s.verReglasAprobacion)
	r.With(q("roles.administrar")).Put("/reglas-aprobacion", s.editarReglasAprobacion)

	// 10 · portal
	r.With(q("portal.ver")).Get("/portal", s.portal)

	// 11 · roles y usuarios
	r.With(q("usuarios.ver")).Get("/usuarios", s.listarUsuarios)
	r.With(q("roles.administrar")).Post("/usuarios/invitar", s.invitarUsuario)
	r.With(q("roles.administrar")).Patch("/usuarios/{uid}", s.editarUsuario)
	r.With(q("roles.administrar")).Put("/roles/{rol}/permisos", s.editarPermisosRol)
	r.With(q("usuarios.ver")).Get("/junta", s.verJunta)
	r.With(q("roles.administrar")).Put("/junta", s.editarJunta)
	r.With(q("auditoria.ver")).Get("/auditoria", s.listarAuditoria)
}

// Módulos nuevos: WhatsApp, chatbot, analítica, kanban de mantenimiento y comercio.
func (s *Server) rutasModulosNuevos(r chi.Router) {
	q := s.requiere
	r.With(q("whatsapp.ver")).Get("/whatsapp/mensajes", s.listarMensajes)
	r.With(q("whatsapp.enviar")).Post("/whatsapp/enviar", s.enviarWhatsApp)
	r.With(q("whatsapp.enviar")).Post("/whatsapp/recibos/{periodo}/enviar", s.enviarRecibosWhatsApp)
	r.With(q("whatsapp.ver")).Get("/whatsapp/config", s.verConfigWhatsApp)
	r.With(q("whatsapp.configurar")).Put("/whatsapp/config", s.guardarConfigWhatsApp)
	r.With(q("whatsapp.ver")).Get("/whatsapp/plantillas", s.listarPlantillas)
	r.With(q("chatbot.probar")).Post("/chatbot/mensaje", s.chatbotMensaje)
	r.With(q("chatbot.probar")).Post("/chatbot/mensajes/{mid}/feedback", s.feedbackMensaje)
	r.With(q("chatbot.probar")).Get("/chatbot/golden", s.listarGolden)
	r.With(q("chatbot.probar")).Patch("/chatbot/golden/{gid}", s.cambiarGolden)
	r.With(q("chatbot.probar")).Delete("/chatbot/golden/{gid}", s.borrarGolden)
	r.With(q("chatbot.probar")).Get("/chatbot/aprendizaje", s.resumenAprendizaje)
	r.With(q("motor.administrar")).Post("/motor/consulta", s.motorConsulta)
	r.With(q("motor.administrar")).Get("/motor/admin", s.motorAdmin)
	r.With(q("motor.administrar")).Post("/motor/golden", s.motorGolden)
	r.With(q("motor.administrar")).Post("/motor/golden/{gid}/ejecutar", s.motorGoldenEjecutar)
	r.With(q("motor.administrar")).Put("/motor/ajuste", s.motorAjuste)
	r.With(q("motor.administrar")).Post("/motor/confirma", s.motorConfirma)

	r.With(q("analitica.ver")).Get("/analitica/resumen", s.analitica)

	// comercio (punto de venta): catálogo, clientes, caja y ventas
	r.With(q("productos.ver")).Get("/productos", s.listarProductos)
	r.With(q("productos.ver")).Get("/productos/{pid}", s.verProducto)
	r.With(q("productos.registrar")).Post("/productos", s.crearProducto)
	r.With(q("productos.registrar")).Put("/productos/{pid}", s.editarProducto)
	r.With(q("productos.registrar")).Delete("/productos/{pid}", s.desactivarProducto)
	r.With(q("productos.ver")).Get("/categorias", s.listarCategorias)
	r.With(q("productos.registrar")).Post("/categorias", s.crearCategoria)
	r.With(q("productos.registrar")).Put("/categorias/{cid}", s.editarCategoria)
	r.With(q("productos.registrar")).Delete("/categorias/{cid}", s.desactivarCategoria)
	r.With(q("clientes.ver")).Get("/clientes", s.listarClientes)
	r.With(q("clientes.registrar")).Post("/clientes", s.crearCliente)
	r.With(q("clientes.registrar")).Put("/clientes/{cid}", s.editarCliente)

	// Proveedores y cuentas por pagar (bloque B)
	r.With(q("proveedores.ver")).Get("/proveedores", s.listarProveedores)
	r.With(q("proveedores.administrar")).Post("/proveedores", s.crearProveedor)
	r.With(q("proveedores.ver")).Get("/proveedores/{pid}", s.verProveedor)
	r.With(q("proveedores.administrar")).Put("/proveedores/{pid}", s.editarProveedor)
	r.With(q("proveedores.administrar")).Delete("/proveedores/{pid}", s.borrarProveedor)
	r.With(q("cuentas_pagar.ver")).Get("/cuentas-por-pagar", s.listarCuentasPorPagar)
	r.With(q("cuentas_pagar.registrar")).Post("/cuentas-por-pagar", s.crearCuentaPorPagar)
	r.With(q("cuentas_pagar.ver")).Get("/cuentas-por-pagar/{cid}", s.verCuentaPorPagar)
	r.With(q("cuentas_pagar.registrar")).Post("/cuentas-por-pagar/{cid}/pagos", s.pagarCuentaPorPagar)

	// Fondos y trazabilidad (bloque C)
	r.With(q("fondos.ver")).Get("/fondos", s.listarFondos)
	r.With(q("fondos.ver")).Get("/fondos/trazabilidad", s.trazabilidadFondos)
	r.With(q("fondos.ver")).Get("/fondos/{fid}/movimientos", s.movimientosFondo)
	r.With(q("fondos.administrar")).Post("/fondos", s.crearFondo)
	r.With(q("fondos.administrar")).Put("/fondos/{fid}", s.editarFondo)
	r.With(q("fondos.administrar")).Delete("/fondos/{fid}", s.desactivarFondo)
	r.With(q("fondos.administrar")).Post("/fondos/{fid}/movimientos", s.movimientoManual)
	r.With(q("fondos.administrar")).Post("/fondos/transferencia", s.transferenciaFondos)

	// Informes económicos y consumos (bloque C)
	r.With(q("balance.ver")).Get("/informes/economico", s.informeEconomico)
	r.With(q("balance.ver")).Get("/informes/consumos", s.consumosPorDepartamento)

	// B4 · recibos e ingresos externos
	r.With(q("externos.ver")).Get("/recibos-externos", s.listarRecibosExternos)
	r.With(q("externos.registrar")).Post("/recibos-externos", s.crearReciboExterno)
	r.With(q("externos.registrar")).Post("/recibos-externos/{id}/pagar", s.pagarReciboExterno)
	r.With(q("externos.ver")).Get("/ingresos-externos", s.listarIngresosExternos)
	r.With(q("externos.registrar")).Post("/ingresos-externos", s.crearIngresoExterno)

	// A3 · cuentas bancarias y vouchers multicuenta
	r.With(q("cuentas_bancarias.ver")).Get("/cuentas-bancarias", s.listarCuentasBancarias)
	r.With(q("cuentas_bancarias.administrar")).Post("/cuentas-bancarias", s.crearCuentaBancaria)
	r.With(q("cuentas_bancarias.administrar")).Put("/cuentas-bancarias/{id}", s.editarCuentaBancaria)
	r.Post("/unidades/{uid}/vouchers", s.registrarVoucher)

	// A4 · cobranzas sin identificar y devoluciones
	r.With(q("cobranzas.ver")).Get("/cobranzas-sin-identificar", s.listarCobranzas)
	r.With(q("cobranzas.gestionar")).Post("/cobranzas-sin-identificar", s.crearCobranza)
	r.With(q("cobranzas.gestionar")).Post("/cobranzas-sin-identificar/{id}/imputar", s.imputarCobranza)
	r.With(q("cobranzas.gestionar")).Post("/cobranzas-sin-identificar/{id}/devolver", s.devolverCobranza)
	r.With(q("cobranzas.ver")).Get("/devoluciones", s.listarDevoluciones)

	// E1 · documentos por categorías
	r.With(q("documentos.ver")).Get("/documentos/categorias", s.listarDocumentoCategorias)
	r.With(q("documentos.administrar")).Post("/documentos/categorias", s.crearDocumentoCategoria)
	r.With(q("documentos.administrar")).Delete("/documentos/categorias/{id}", s.borrarDocumentoCategoria)
	r.With(q("documentos.ver")).Get("/documentos", s.listarDocumentos)
	r.With(q("documentos.administrar")).Post("/documentos", s.crearDocumento)
	r.With(q("documentos.administrar")).Post("/documentos/{id}/publicar", s.publicarDocumento)

	// marca: I1/I2/I5 · plantilla de recibo, marca blanca y configuración del edificio
	r.With(q("recibos.plantilla")).Get("/plantilla-recibo", s.verPlantillaRecibo)
	r.With(q("recibos.plantilla")).Put("/plantilla-recibo", s.guardarPlantillaRecibo)
	r.With(q("recibos.plantilla")).Post("/plantilla-recibo/vista-previa", s.vistaPreviaRecibo)
	r.With(q("marca.configurar")).Get("/marca", s.verMarca)
	r.With(q("marca.configurar")).Put("/marca", s.guardarMarca)
	r.With(q("marca.configurar")).Post("/marca/logo", s.subirLogo)
	r.With(q("marca.configurar")).Delete("/marca/logo", s.quitarLogo)
	r.With(q("configuracion.ver")).Get("/configuracion/estado", s.verEstadoEdificio)
	r.With(q("configuracion.editar")).Put("/configuracion/activo", s.cambiarActivoEdificio)
	r.With(q("configuracion.ver")).Get("/configuracion/asistentes", s.asistenteConfiguracion)
	r.With(q("configuracion.ver")).Get("/configuracion/cambios", s.registroCambios)

	r.With(q("incidencias.ver")).Get("/mantenimiento/incidencias", s.listarIncidencias)
	r.With(q("incidencias.ver")).Patch("/mantenimiento/incidencias/{tid}/estado", s.cambiarEstadoIncidencia)
	r.With(q("incidencias.ver")).Get("/mantenimiento/incidencias/exportar", s.exportarIncidencias)
	r.With(q("incidencias.ver")).Get("/mantenimiento/plan", s.planTrabajos)
	r.With(q("incidencias.ver")).Get("/mantenimiento/tablero/config", s.verConfigTablero)
	r.With(q("roles.administrar")).Put("/mantenimiento/tablero/config", s.guardarConfigTablero)
}

// ---------- middleware ----------

func (s *Server) idPeticion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			b := make([]byte, 6)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), P.ClaveIDPeticion, id)))
	})
}

type grabador struct {
	http.ResponseWriter
	status int
}

func (g *grabador) WriteHeader(c int) { g.status = c; g.ResponseWriter.WriteHeader(c) }

func (s *Server) registro(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ini := time.Now()
		g := &grabador{ResponseWriter: w, status: 200}
		next.ServeHTTP(g, r)
		if r.URL.Path == "/api/health" {
			return
		}
		slog.Info("http", "metodo", r.Method, "ruta", r.URL.Path, "estado", g.status, "ms", time.Since(ini).Milliseconds(), "id", P.IDPeticion(r.Context()))
	})
}

func (s *Server) recuperar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.Error("pánico", "valor", v, "ruta", r.URL.Path, "id", P.IDPeticion(r.Context()))
				P.Fallo(w, r, errPanico{v})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type errPanico struct{ v any }

func (e errPanico) Error() string { return "pánico" }

// autenticar acepta la cookie edisys_at o Authorization: Bearer.
func (s *Server) autenticar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, bearer := "", false
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			token, bearer = strings.TrimPrefix(h, "Bearer "), true
		} else if c, err := r.Cookie(auth.CookieAcceso); err == nil {
			token = c.Value
		}
		if token == "" {
			P.Fallo(w, r, P.Err(http.StatusUnauthorized, "SIN_SESION", "Inicia sesión para continuar."))
			return
		}
		uid, _, err := auth.LeerJWT(s.Cfg.JWTSecret, token)
		if err != nil {
			P.Fallo(w, r, P.Err(http.StatusUnauthorized, "TOKEN_VENCIDO", "Tu sesión venció. Vuelve a entrar."))
			return
		}
		se, err := s.cargarSesion(r.Context(), uid)
		if err != nil {
			P.Fallo(w, r, P.Err(http.StatusUnauthorized, "SIN_SESION", "Tu usuario no está activo."))
			return
		}
		se.PorBearer = bearer
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxSesion, se)))
	})
}

func (s *Server) cargarSesion(ctx context.Context, uid int64) (*Sesion, error) {
	se := &Sesion{UsuarioID: uid, Roles: map[int64]string{}}
	var adm *int64
	var correo *string
	err := s.DB.QueryRow(ctx, `SELECT nombre, correo, telefono, administradora_id, es_superadmin FROM usuario WHERE id=$1 AND activo`, uid).
		Scan(&se.Nombre, &correo, &se.Telefono, &adm, &se.Superadmin)
	if err != nil {
		return nil, err
	}
	if adm != nil {
		se.AdmID = *adm
	}
	if correo != nil {
		se.Correo = *correo
	}
	var q string
	if se.Superadmin {
		q = `SELECT id, 'superadmin' FROM edificio`
	} else {
		q = `SELECT uer.edificio_id, uer.rol FROM usuario_edificio_rol uer JOIN edificio e ON e.id = uer.edificio_id
		     WHERE uer.usuario_id = $1 AND e.administradora_id = (SELECT administradora_id FROM usuario WHERE id = $1)`
	}
	args := []any{}
	if !se.Superadmin {
		args = append(args, uid)
	}
	filas, err := s.DB.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	for filas.Next() {
		var id int64
		var rol string
		if err := filas.Scan(&id, &rol); err != nil {
			return nil, err
		}
		se.Roles[id] = rol
	}
	return se, filas.Err()
}

// csrf: con cookie, toda escritura exige X-EDISYS: 1 (§2.4, paso 6). Con Bearer no hace falta.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if se := ses(r); se != nil && !se.PorBearer && r.Header.Get("X-EDISYS") != "1" {
				P.Fallo(w, r, P.Prohibido("CSRF", "Falta la cabecera X-EDISYS: 1."))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// auditar deja rastro de toda escritura que salió bien (quién, qué ruta, desde qué IP).
func (s *Server) auditar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g := &grabador{ResponseWriter: w, status: 200}
		next.ServeHTTP(g, r)
		if r.Method == http.MethodGet || g.status >= 400 {
			return
		}
		se := ses(r)
		if se == nil {
			return
		}
		var eid *int64
		if e := edf(r); e != nil {
			eid = &e.ID
		} else if v, err := strconv.ParseInt(chi.URLParam(r, "eid"), 10, 64); err == nil {
			eid = &v
		}
		patron := chi.RouteContext(r.Context()).RoutePattern()
		modulo := moduloDeRuta(patron)
		_, err := s.DB.Exec(context.Background(), `INSERT INTO auditoria (edificio_id, usuario_id, modulo, accion, entidad, ip)
			VALUES ($1,$2,$3,$4,$5,$6)`, eid, se.UsuarioID, modulo, r.Method+" "+patron, r.URL.Path, ipDe(r))
		if err != nil {
			slog.Warn("auditoría", "err", err)
		}
	})
}

func moduloDeRuta(patron string) string {
	partes := strings.Split(strings.Trim(patron, "/"), "/")
	for i, p := range partes {
		if p == "v1" || p == "api" || p == "edificios" || strings.HasPrefix(p, "{") {
			continue
		}
		_ = i
		return p
	}
	return "api"
}

func ipDe(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

// conEdificio resuelve el edificio (ruta, ?edificio_id, cabecera X-Edificio o el primero del usuario),
// comprueba que el usuario pertenece a él (si no: 404, no 403) y carga permisos y unidades propias.
func (s *Server) conEdificio(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		se := ses(r)
		txt := chi.URLParam(r, "eid")
		if txt == "" {
			txt = r.URL.Query().Get("edificio_id")
		}
		if txt == "" {
			txt = r.Header.Get("X-Edificio")
		}
		var eid int64
		if txt != "" {
			v, err := strconv.ParseInt(txt, 10, 64)
			if err != nil {
				P.Fallo(w, r, P.NoEncontrado("el edificio"))
				return
			}
			eid = v
		} else if ids := se.Edificios(); len(ids) > 0 {
			eid = ids[0]
		}
		rol, ok := se.Roles[eid]
		if !ok {
			P.Fallo(w, r, P.NoEncontrado("el edificio"))
			return
		}
		e, err := s.cargarEdificio(r.Context(), eid, rol, se.UsuarioID)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxEdificio, e)))
	})
}

func (s *Server) cargarEdificio(ctx context.Context, eid int64, rol string, uid int64) (*Edificio, error) {
	e := &Edificio{ID: eid, Rol: rol, Permisos: map[string]bool{}}
	if err := s.DB.QueryRow(ctx, `SELECT nombre FROM edificio WHERE id=$1`, eid).Scan(&e.Nombre); err != nil {
		return nil, P.NoEncontrado("el edificio")
	}
	for p := range s.permBase[rol] {
		e.Permisos[p] = true
	}
	filas, err := s.DB.Query(ctx, `SELECT rpe.permiso, rpe.habilitado FROM rol_permiso_edificio rpe JOIN permiso p ON p.codigo = rpe.permiso
		WHERE rpe.edificio_id=$1 AND rpe.rol=$2 AND p.ajustable`, eid, rol)
	if err != nil {
		return nil, err
	}
	for filas.Next() {
		var p string
		var h bool
		if err := filas.Scan(&p, &h); err != nil {
			filas.Close()
			return nil, err
		}
		if h {
			e.Permisos[p] = true
		} else {
			delete(e.Permisos, p)
		}
	}
	filas.Close()
	if e.SoloLoSuyo() {
		ids, err := s.unidadesDeUsuario(ctx, eid, uid, rol)
		if err != nil {
			return nil, err
		}
		e.Unidades = ids
		// El inquilino ve recibos solo si el propietario lo habilitó en su unidad.
		if rol == "inquilino" && len(ids) > 0 {
			var ver bool
			_ = s.DB.QueryRow(ctx, `SELECT bool_or(COALESCE((permisos_inquilino->>'ver_recibos')::boolean,false)) FROM unidad WHERE id = ANY($1)`, ids).Scan(&ver)
			if ver {
				e.Permisos["recibos.ver"] = true
			}
		}
	}
	return e, nil
}

func (s *Server) unidadesDeUsuario(ctx context.Context, eid, uid int64, rol string) ([]int64, error) {
	filas, err := s.DB.Query(ctx, `SELECT DISTINCT up.unidad_id FROM unidad_persona up
		JOIN persona p ON p.id = up.persona_id JOIN unidad u ON u.id = up.unidad_id
		WHERE p.usuario_id=$1 AND u.edificio_id=$2 AND up.hasta IS NULL AND up.rol=$3 ORDER BY 1`, uid, eid, rol)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	ids := []int64{}
	for filas.Next() {
		var id int64
		if err := filas.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, filas.Err()
}

// requiere exige un permiso en el edificio activo: 403 SIN_PERMISO con el permiso que falta.
func (s *Server) requiere(permiso string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			e := edf(r)
			if e == nil || !e.Puede(permiso) {
				P.Fallo(w, r, P.Prohibido("SIN_PERMISO", "Tu rol no tiene permiso para esto.").Con("permiso", permiso))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---------- utilidades comunes ----------

func idRuta(r *http.Request, nombre string) (int64, error) {
	v, err := strconv.ParseInt(chi.URLParam(r, nombre), 10, 64)
	if err != nil || v <= 0 {
		return 0, P.NoEncontrado("el registro")
	}
	return v, nil
}

func paginacion(r *http.Request) (pagina, porPagina int) {
	pagina, _ = strconv.Atoi(r.URL.Query().Get("pagina"))
	porPagina, _ = strconv.Atoi(r.URL.Query().Get("por_pagina"))
	if pagina < 1 {
		pagina = 1
	}
	if porPagina < 1 || porPagina > 200 {
		porPagina = 25
	}
	return
}

func paginado(datos []map[string]any, total int64, pagina int) map[string]any {
	return map[string]any{"datos": datos, "total": total, "pagina": pagina}
}

// auditarCambio registra antes y después de un cambio sensible.
func (s *Server) auditarCambio(ctx context.Context, q db.Q, r *http.Request, modulo, accion, entidad string, id any, antes, despues any) {
	var eid *int64
	if e := edf(r); e != nil {
		eid = &e.ID
	}
	var uid *int64
	if se := ses(r); se != nil {
		uid = &se.UsuarioID
	}
	_, err := q.Exec(ctx, `INSERT INTO auditoria (edificio_id, usuario_id, modulo, accion, entidad, entidad_id, antes, despues, ip)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, eid, uid, modulo, accion, entidad, toStr(id), antes, despues, ipDe(r))
	if err != nil {
		slog.Warn("auditoría", "err", err)
	}
}

func toStr(v any) string {
	switch x := v.(type) {
	case int64:
		return strconv.FormatInt(x, 10)
	case string:
		return x
	case nil:
		return ""
	}
	return ""
}

// salud: GET /api/health → { ok, version, db, s3 }.
func (s *Server) salud(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	dbOK := s.DB.Ping(ctx) == nil
	s3OK := s.Almacen != nil && s.Almacen.Ping(ctx) == nil
	estado := http.StatusOK
	if !dbOK {
		estado = http.StatusServiceUnavailable
	}
	P.JSON(w, estado, map[string]any{"ok": dbOK && s3OK, "version": s.Cfg.Version, "db": dbOK, "s3": s3OK, "whatsapp_modo": s.Cfg.WhatsAppModo, "correo_modo": s.Cfg.CorreoModo})
}
