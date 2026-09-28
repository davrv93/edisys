// Package plataforma reúne lo que usan todos los módulos: errores con el formato §2.6,
// respuestas JSON, dinero en céntimos y fechas en hora de Lima.
package plataforma

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Error es un error de negocio que se responde tal cual al cliente.
type Error struct {
	Status  int
	Codigo  string
	Mensaje string
	Campos  map[string]string
	Extra   map[string]any
}

func (e *Error) Error() string { return e.Codigo + ": " + e.Mensaje }

// Err crea un error de negocio.
func Err(status int, codigo, mensaje string) *Error {
	return &Error{Status: status, Codigo: codigo, Mensaje: mensaje}
}

// Con agrega un dato extra dentro del objeto error (p. ej. "permiso", "monto_cts").
func (e *Error) Con(clave string, valor any) *Error {
	if e.Extra == nil {
		e.Extra = map[string]any{}
	}
	e.Extra[clave] = valor
	return e
}

// Campo agrega un error de validación de un campo (422).
func (e *Error) Campo(campo, mensaje string) *Error {
	if e.Campos == nil {
		e.Campos = map[string]string{}
	}
	e.Campos[campo] = mensaje
	return e
}

// Atajos frecuentes.
func NoEncontrado(que string) *Error {
	return Err(http.StatusNotFound, "NO_ENCONTRADO", "No encontramos "+que+".")
}
func Validacion(mensaje string) *Error { return Err(http.StatusUnprocessableEntity, "VALIDACION", mensaje) }
func Conflicto(codigo, mensaje string) *Error { return Err(http.StatusConflict, codigo, mensaje) }
func Prohibido(codigo, mensaje string) *Error  { return Err(http.StatusForbidden, codigo, mensaje) }

// JSON responde con el estado y el cuerpo dados.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil || status == http.StatusNoContent {
		return
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// Leer decodifica el cuerpo JSON. Un JSON inválido es 422.
func Leer(r *http.Request, v any) error {
	if r.Body == nil {
		return Validacion("Falta el cuerpo de la petición.")
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 2<<20))
	if err := dec.Decode(v); err != nil {
		return Err(http.StatusUnprocessableEntity, "JSON_INVALIDO", "El cuerpo no es un JSON válido: "+err.Error())
	}
	return nil
}

type claveCtx string

// ClaveIDPeticion guarda el id de la petición en el contexto.
const ClaveIDPeticion claveCtx = "id_peticion"

// IDPeticion devuelve el id de la petición.
func IDPeticion(ctx context.Context) string {
	if v, ok := ctx.Value(ClaveIDPeticion).(string); ok {
		return v
	}
	return ""
}

// Traducir convierte errores de PostgreSQL con reglas duras en errores de negocio.
func Traducir(err error) error {
	var be *Error
	if errors.As(err, &be) {
		return be
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return NoEncontrado("el registro")
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23P01":
			return Conflicto("FRANJA_OCUPADA", "Alguien acaba de reservar esa franja. Elige otra.")
		case "ED001":
			return Err(http.StatusUnprocessableEntity, "PARTICIPACION_NO_SUMA_100", strings.TrimPrefix(pg.Message, "PARTICIPACION_NO_SUMA_100: ")).
				Campo("participacion_pct", "La suma de participaciones debe ser 100 %.")
		case "ED002":
			return Prohibido("MOROSO", "La unidad tiene deuda vencida y no puede reservar.")
		case "ED004":
			return Conflicto("TRANSICION_INVALIDA", strings.TrimPrefix(pg.Message, "TRANSICION_INVALIDA: "))
		case "23505":
			switch pg.ConstraintName {
			case "pago_operacion_uq":
				return Conflicto("PAGO_DUPLICADO", "Ese código de operación ya se registró para ese medio y fecha.")
			case "voto_incidencia_id_usuario_id_key":
				return Conflicto("YA_VOTASTE", "Ya votaste este trabajo.")
			case "lectura_medidor_id_periodo_id_key":
				return Conflicto("YA_LEIDO", "Ese medidor ya tiene lectura en el periodo. Para corregirla usa PUT.")
			case "periodo_edificio_id_periodo_key":
				return Conflicto("PERIODO_EXISTE", "Ese periodo ya está abierto.")
			case "unidad_edificio_id_codigo_key":
				return Conflicto("UNIDAD_EXISTE", "Ya hay una unidad con ese código.")
			case "usuario_correo_key":
				return Conflicto("CORREO_EXISTE", "Ya hay un usuario con ese correo.")
			case "recibo_unidad_periodo_uq":
				return Conflicto("YA_EMITIDO", "La unidad ya tiene recibo en ese periodo.")
			case "recibo_general_periodo_id_tipo_key":
				return Conflicto("RECIBO_GENERAL_EXISTE", "Ya se registró el recibo general de ese periodo.")
			}
			return Conflicto("DUPLICADO", "El registro ya existe.")
		case "23503":
			return Validacion("Una referencia no existe (" + pg.ConstraintName + ").")
		case "23514":
			return Validacion("Un valor no cumple las reglas (" + pg.ConstraintName + ").")
		case "22P02", "22007", "22008":
			return Validacion("Un valor tiene un formato inválido.")
		}
	}
	return err
}

// Fallo responde un error con el formato §2.6. Los 5xx se registran con el id de la petición.
func Fallo(w http.ResponseWriter, r *http.Request, err error) {
	err = Traducir(err)
	var be *Error
	if errors.As(err, &be) {
		cuerpo := map[string]any{"codigo": be.Codigo, "mensaje": be.Mensaje}
		if len(be.Campos) > 0 {
			cuerpo["campos"] = be.Campos
		}
		for k, v := range be.Extra {
			cuerpo[k] = v
		}
		JSON(w, be.Status, map[string]any{"error": cuerpo})
		return
	}
	id := IDPeticion(r.Context())
	slog.Error("error interno", "id_peticion", id, "ruta", r.Method+" "+r.URL.Path, "err", err)
	JSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{
		"codigo":      "ERROR_INTERNO",
		"mensaje":     fmt.Sprintf("Algo falló de nuestro lado. Código de seguimiento: %s", id),
		"id_peticion": id,
	}})
}
