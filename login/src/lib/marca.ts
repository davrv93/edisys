// Marca blanca en el login (bloque I2). El slug llega por ?marca= o lo dejó la app en localStorage
// (mismo origen) la última vez; el API devuelve nombre, lema, logo y los tokens de color ya calculados.
import { PREFIJO } from "./login";

export const CLAVE_SLUG = "edisys.marca";
const RE_SLUG = /^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$/;
const RE_VAR = /^--color-[a-z-]+$/;
const RE_HEX = /^#[0-9A-Fa-f]{6}$/;

export interface MarcaPublica {
  nombre: string;
  lema: string;
  slug: string;
  logo_url: string | null;
  color_fondo_login: string;
  personalizada: boolean;
  tokens: { claro?: Record<string, string>; oscuro?: Record<string, string> };
}

/** Slug válido de la URL o, si no hay, el guardado. */
export function slugDe(url: URL, guardado: string | null): string | null {
  const s = (url.searchParams.get("marca") || guardado || "").toLowerCase();
  return RE_SLUG.test(s) ? s : null;
}

export const rutaMarca = (slug: string) => `${PREFIJO}/api/v1/publico/marca/${slug}`;

/** Hoja CSS con los tokens: solo variables --color-* con valores #RRGGBB. */
export function cssDeTokens(t: MarcaPublica["tokens"] | undefined): string {
  const bloque = (sel: string, vars?: Record<string, string>) => {
    const decl = Object.entries(vars || {})
      .filter(([k, v]) => RE_VAR.test(k) && RE_HEX.test(String(v)))
      .map(([k, v]) => `${k}:${v};`)
      .join("");
    return decl ? `${sel}{${decl}}` : "";
  };
  return bloque(":root", t?.claro) + bloque(':root[data-tema="oscuro"]', t?.oscuro);
}

export const colorValido = (c: string | undefined) => (c && RE_HEX.test(c) ? c : null);
