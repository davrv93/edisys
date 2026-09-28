// Cliente del API: mismo origen, cookie HttpOnly, cabecera anti-CSRF (§2.4).
// Con VITE_MOCK=1 las peticiones las contesta src/mock/ (solo desarrollo;
// en el build de producción esa rama desaparece y se llama al API real).

export const BASE = '/api/v1';
export const MOCK = import.meta.env.VITE_MOCK === '1';

export class ApiError extends Error {
  constructor(status, cuerpo = {}) {
    const e = cuerpo?.error || {};
    super(e.mensaje || mensajePorEstado(status));
    this.name = 'ApiError';
    this.status = status;
    this.codigo = e.codigo || `HTTP_${status}`;
    this.campos = e.campos || {};
    this.permiso = e.permiso;
    this.detalle = e;
    this.idPeticion = e.id_peticion || e.request_id || null;
  }
}

function mensajePorEstado(status) {
  if (status === 0) return 'Revisa tu conexión e intenta otra vez.';
  if (status === 401) return 'Tu sesión terminó. Vuelve a entrar.';
  if (status === 403) return 'No tienes permiso para esto.';
  if (status === 404) return 'No encontramos lo que buscas.';
  if (status === 409) return 'Alguien cambió estos datos antes que tú.';
  if (status === 422) return 'Revisa los campos marcados.';
  if (status >= 500) return 'El servidor tuvo un problema. Intenta de nuevo en un momento.';
  return 'No se pudo completar la operación.';
}

export function construirQuery(query) {
  if (!query) return '';
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null || v === '') continue;
    sp.set(k, String(v));
  }
  const s = sp.toString();
  return s ? `?${s}` : '';
}

export function irAlLogin() {
  const siguiente = window.location.pathname + window.location.search;
  window.location.assign(`/login/?next=${encodeURIComponent(siguiente)}`);
}

let refrescando = null;
async function refrescar() {
  if (!refrescando) {
    refrescando = fetch(`${BASE}/auth/refresh`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'X-EDISYS': '1' },
    })
      .then((r) => r.ok)
      .catch(() => false)
      .finally(() => {
        setTimeout(() => (refrescando = null), 0);
      });
  }
  return refrescando;
}

async function leerCuerpo(res) {
  if (res.status === 204) return null;
  const tipo = res.headers.get('content-type') || '';
  if (tipo.includes('application/json')) return res.json().catch(() => null);
  return res.text().catch(() => null);
}

/**
 * Petición al API.
 * @param {string} metodo GET | POST | PUT | PATCH | DELETE
 * @param {string} ruta   sin el prefijo /api/v1 (ej. «/yo»)
 * @param {{ query?: object, body?: any, form?: FormData, sinRedirigir?: boolean }} op
 */
export async function peticion(metodo, ruta, op = {}) {
  const url = BASE + ruta + construirQuery(op.query);
  if (MOCK) {
    const { responderMock } = await import('../mock/servidor.js');
    return responderMock(metodo, ruta, op);
  }
  const hacer = () => {
    const headers = { Accept: 'application/json' };
    if (metodo !== 'GET') headers['X-EDISYS'] = '1';
    let body;
    if (op.form) body = op.form;
    else if (op.body !== undefined) {
      headers['Content-Type'] = 'application/json';
      body = JSON.stringify(op.body);
    }
    return fetch(url, { method: metodo, credentials: 'include', headers, body });
  };
  let res;
  try {
    res = await hacer();
    if (res.status === 401 && !ruta.startsWith('/auth/')) {
      if (await refrescar()) res = await hacer();
    }
  } catch {
    throw new ApiError(0, {});
  }
  const cuerpo = await leerCuerpo(res);
  if (!res.ok) {
    if (res.status === 401 && !op.sinRedirigir) irAlLogin();
    throw new ApiError(res.status, cuerpo || {});
  }
  return cuerpo;
}

export const api = {
  get: (ruta, query, op) => peticion('GET', ruta, { ...op, query }),
  post: (ruta, body, op) => peticion('POST', ruta, { ...op, body }),
  put: (ruta, body, op) => peticion('PUT', ruta, { ...op, body }),
  patch: (ruta, body, op) => peticion('PATCH', ruta, { ...op, body }),
  del: (ruta, op) => peticion('DELETE', ruta, op),
  form: (metodo, ruta, form, op) => peticion(metodo, ruta, { ...op, form }),
};

/**
 * Subida multipart con progreso y 3 reintentos ante fallos de red (§2.2 SubirFoto).
 * No reintenta respuestas 4xx: esas son del usuario, no de la red.
 */
export async function subir(ruta, form, { metodo = 'POST', onProgreso, intentos = 3 } = {}) {
  if (MOCK) {
    const { responderMock } = await import('../mock/servidor.js');
    for (let p = 20; p <= 100; p += 40) {
      onProgreso?.(p);
      await new Promise((r) => setTimeout(r, 120));
    }
    return responderMock(metodo, ruta, { form });
  }
  let ultimoError;
  for (let i = 0; i < intentos; i++) {
    try {
      return await subirUnaVez(ruta, form, metodo, onProgreso);
    } catch (e) {
      ultimoError = e;
      if (e.status && e.status !== 0 && e.status < 500) throw e;
      await new Promise((r) => setTimeout(r, 800 * (i + 1)));
    }
  }
  throw ultimoError;
}

function subirUnaVez(ruta, form, metodo, onProgreso) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open(metodo, BASE + ruta);
    xhr.withCredentials = true;
    xhr.setRequestHeader('X-EDISYS', '1');
    xhr.setRequestHeader('Accept', 'application/json');
    xhr.upload.onprogress = (ev) => {
      if (ev.lengthComputable) onProgreso?.(Math.round((ev.loaded / ev.total) * 100));
    };
    xhr.onerror = () => reject(new ApiError(0, {}));
    xhr.onload = () => {
      let cuerpo = null;
      try {
        cuerpo = xhr.responseText ? JSON.parse(xhr.responseText) : null;
      } catch {
        cuerpo = null;
      }
      if (xhr.status >= 200 && xhr.status < 300) resolve(cuerpo);
      else {
        if (xhr.status === 401) irAlLogin();
        reject(new ApiError(xhr.status, cuerpo || {}));
      }
    };
    xhr.send(form);
  });
}

/** Normaliza listas: acepta [..], {datos:[..]}, {items:[..]} o {<clave>:[..]}. */
export function lista(resp, clave) {
  if (Array.isArray(resp)) return resp;
  if (!resp || typeof resp !== 'object') return [];
  if (clave && Array.isArray(resp[clave])) return resp[clave];
  if (Array.isArray(resp.datos)) return resp.datos;
  if (Array.isArray(resp.items)) return resp.items;
  return [];
}

/** URL absoluta de un recurso descargable (PDF, plantilla). */
export function urlApi(ruta) {
  return BASE + ruta;
}
