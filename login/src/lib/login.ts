/**
 * Lógica pura del login, compartida por el navegador (fetch directo al API)
 * y por el servidor (respaldo sin JavaScript en onPost).
 */

export const RUTA_LOGIN_API = "/api/v1/auth/login";
export const DESTINO_POR_DEFECTO = "/app/";

/**
 * Solo se aceptan rutas internas que empiecen por /app/.
 * Rechaza URLs absolutas, protocolo relativo (//otro.pe), barras invertidas y «..».
 */
export function destinoSeguro(valor: string | null | undefined): string {
  if (!valor) return DESTINO_POR_DEFECTO;
  const v = valor.trim();
  if (v === "/app") return DESTINO_POR_DEFECTO;
  if (!v.startsWith("/app/")) return DESTINO_POR_DEFECTO;
  if (v.includes("\\") || v.includes("//") || /(^|\/)\.\.(\/|$)/.test(v)) return DESTINO_POR_DEFECTO;
  if (/[\u0000-\u001f\u007f]/.test(v)) return DESTINO_POR_DEFECTO;
  return v;
}

/** Lee `next` (o `volver`, el nombre que usa la guía) de la URL. */
export function destinoDesdeUrl(url: URL): string {
  return destinoSeguro(url.searchParams.get("next") ?? url.searchParams.get("volver"));
}

export interface ErrorLogin {
  /** Mensaje general que se muestra arriba del formulario. */
  mensaje: string;
  /** Errores por campo (422). */
  campos?: { correo?: string; clave?: string };
  /** Minutos de espera cuando el API responde 429. */
  esperaMin?: number;
  /** Id de petición para soporte (5xx). */
  idPeticion?: string;
}

interface CuerpoError {
  error?: {
    codigo?: string;
    mensaje?: string;
    campos?: Record<string, string>;
    minutos?: number;
    espera_min?: number;
    id_peticion?: string;
  };
}

/** Traduce una respuesta de error del API (§2.6) a lo que ve la persona. */
export function errorDesdeRespuesta(
  status: number,
  cuerpo: unknown,
  cabeceras: { get(nombre: string): string | null },
): ErrorLogin {
  const e = (cuerpo as CuerpoError | null)?.error ?? {};
  switch (status) {
    case 401:
      return { mensaje: "Correo o clave incorrectos." };
    case 423:
      return {
        mensaje: e.mensaje ?? "Tu cuenta está bloqueada. Pide a la administración que la reactive.",
      };
    case 429: {
      const retry = Number(cabeceras.get("retry-after"));
      const min =
        e.minutos ?? e.espera_min ?? (Number.isFinite(retry) && retry > 0 ? Math.ceil(retry / 60) : 15);
      return {
        mensaje: `Demasiados intentos. Espera ${min} ${min === 1 ? "minuto" : "minutos"} y vuelve a intentar.`,
        esperaMin: min,
      };
    }
    case 400:
    case 422: {
      const campos: ErrorLogin["campos"] = {};
      const c = e.campos ?? {};
      if (c.correo ?? c.usuario) campos.correo = c.correo ?? c.usuario;
      if (c.clave) campos.clave = c.clave;
      return { mensaje: e.mensaje ?? "Revisa los datos marcados.", campos };
    }
    default: {
      const id = e.id_peticion ?? cabeceras.get("x-request-id") ?? undefined;
      return {
        mensaje: "No pudimos ingresar por un problema del servidor. Intenta otra vez en un momento.",
        idPeticion: id,
      };
    }
  }
}

export const ERROR_SIN_CONEXION: ErrorLogin = {
  mensaje: "Revisa tu conexión e intenta otra vez.",
};

/** Validación mínima en el navegador: la clave no se valida por largo al entrar (§01). */
export function validar(correo: string, clave: string): ErrorLogin | null {
  const campos: ErrorLogin["campos"] = {};
  if (!correo.trim()) campos.correo = "Escribe tu correo o DNI.";
  if (!clave) campos.clave = "Escribe tu contraseña.";
  if (campos.correo || campos.clave) return { mensaje: "Completa los campos marcados.", campos };
  return null;
}
