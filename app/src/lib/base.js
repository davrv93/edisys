// Prefijo de URL pública de la app. Vacío en local; «/edisys» cuando la app se
// sirve bajo el alias de boticalima. Se deriva del base de Astro, que es
// «${PREFIJO}/app/» (p. ej. «/edisys/app/» → prefijo «/edisys»).
const BASE_URL = import.meta.env.BASE_URL || '/app/';

export const PREFIJO = BASE_URL.replace(/\/app\/?$/, '').replace(/\/$/, '');
export const BASE_APP = `${PREFIJO}/app`;
export const BASE_API = `${PREFIJO}/api/v1`;
export const BASE_LOGIN = `${PREFIJO}/login/`;
