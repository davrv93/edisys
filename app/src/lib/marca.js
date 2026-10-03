// Marca blanca en el navegador (bloque I2). El API calcula los tokens de color de la administradora
// (/yo → marca.tokens) y aquí solo se aplican: una hoja <style> que pisa las variables de tokens.css
// en el tema claro y en el oscuro. Sin marca, nada cambia y queda EDISYS.
import { useSyncExternalStore } from 'react';

const ID_ESTILO = 'edisys-marca';
// El login (otra app, mismo origen) lee este slug para pintarse con la marca antes de la sesión.
export const CLAVE_SLUG = 'edisys.marca';

const RE_VAR = /^--color-[a-z-]+$/;
const RE_HEX = /^#[0-9A-Fa-f]{6}$/;

/** Hoja CSS con los tokens. Solo pasan variables --color-* con valores #RRGGBB: nada de CSS arbitrario. */
export function cssDeTokens(tokens) {
  const bloque = (sel, vars) => {
    const decl = Object.entries(vars || {})
      .filter(([k, v]) => RE_VAR.test(k) && RE_HEX.test(String(v)))
      .map(([k, v]) => `${k}:${v};`)
      .join('');
    return decl ? `${sel}{${decl}}` : '';
  };
  return bloque(':root', tokens?.claro) + bloque(':root[data-tema="oscuro"]', tokens?.oscuro);
}

/** Contraste WCAG entre dos #RRGGBB (1–21); 0 si alguno no es válido. Espejo de app.Contraste en Go. */
export function contraste(a, b) {
  if (!RE_HEX.test(a || '') || !RE_HEX.test(b || '')) return 0;
  const lum = (h) => {
    const c = [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16) / 255).map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
    return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
  };
  const [l1, l2] = [lum(a), lum(b)].sort((x, y) => y - x);
  return (l1 + 0.05) / (l2 + 0.05);
}

export const CONTRASTE_MINIMO = 4.5;

// ---------- estado compartido (logo y nombre para el armazón) ----------

let actual = null;
const oyentes = new Set();

function avisar() {
  oyentes.forEach((f) => f());
}

/** Aplica la marca de /yo (o del login). Con null o sin personalizar, vuelve a EDISYS. */
export function aplicarMarca(marca) {
  actual = marca && marca.personalizada ? marca : null;
  if (typeof document !== 'undefined') {
    let el = document.getElementById(ID_ESTILO);
    const css = actual ? cssDeTokens(actual.tokens) : '';
    if (css) {
      if (!el) {
        el = document.createElement('style');
        el.id = ID_ESTILO;
        document.head.appendChild(el);
      }
      el.textContent = css;
    } else if (el) {
      el.remove();
    }
  }
  try {
    if (marca?.slug) window.localStorage.setItem(CLAVE_SLUG, marca.slug);
    else window.localStorage.removeItem(CLAVE_SLUG);
  } catch {
    /* modo privado: el login sale con EDISYS */
  }
  avisar();
}

function suscribir(f) {
  oyentes.add(f);
  return () => oyentes.delete(f);
}

/** Marca activa ({ nombre, logo_url, … }) o null si es EDISYS. */
export function useMarca() {
  return useSyncExternalStore(suscribir, () => actual, () => null);
}
