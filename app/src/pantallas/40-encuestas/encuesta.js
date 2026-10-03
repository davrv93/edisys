// Lógica pura de la pantalla de encuestas (J1): formulario ↔ API y respuestas.
// El API valida todo; esto solo arma el cuerpo y avisa antes de enviar.

export const TIPOS_PREGUNTA = [
  { valor: 'unica', etiqueta: 'Opción única' },
  { valor: 'multiple', etiqueta: 'Varias opciones' },
  { valor: 'texto', etiqueta: 'Respuesta libre' },
];

export function preguntaVacia() {
  return { texto: '', tipo: 'unica', obligatoria: true, opciones: 'Sí\nNo' };
}

export function formularioVacio() {
  return { titulo: '', descripcion: '', cierra_en: '', anonima: true, preguntas: [preguntaVacia()] };
}

/** Opciones escritas una por línea → lista limpia y sin repetidos (sin distinguir mayúsculas). */
export function opcionesDeTexto(texto) {
  const vistas = new Set();
  const out = [];
  for (const linea of String(texto || '').split('\n')) {
    const o = linea.trim();
    if (!o || vistas.has(o.toLowerCase())) continue;
    vistas.add(o.toLowerCase());
    out.push(o);
  }
  return out;
}

/** Detalle del API → formulario editable (opciones como texto, una por línea). */
export function formularioDeEncuesta(enc) {
  return {
    titulo: enc.titulo || '',
    descripcion: enc.descripcion || '',
    cierra_en: enc.cierra_en || '',
    anonima: !!enc.anonima,
    preguntas: (enc.preguntas || []).map((p) => ({
      texto: p.texto,
      tipo: p.tipo,
      obligatoria: !!p.obligatoria,
      opciones: (p.opciones || []).map((o) => o.texto).join('\n'),
    })),
  };
}

/** Formulario → cuerpo del POST/PUT. Devuelve { cuerpo, error } (error en español o null). */
export function cuerpoEncuesta(f) {
  const titulo = String(f.titulo || '').trim();
  if (!titulo) return { cuerpo: null, error: 'Escribe el título de la encuesta.' };
  if (!f.preguntas?.length) return { cuerpo: null, error: 'Agrega al menos una pregunta.' };
  const preguntas = [];
  for (const [i, p] of f.preguntas.entries()) {
    const texto = String(p.texto || '').trim();
    if (!texto) return { cuerpo: null, error: `La pregunta ${i + 1} no tiene texto.` };
    const q = { texto, tipo: p.tipo, obligatoria: !!p.obligatoria };
    if (p.tipo !== 'texto') {
      q.opciones = opcionesDeTexto(p.opciones);
      if (q.opciones.length < 2) return { cuerpo: null, error: `La pregunta ${i + 1} necesita al menos dos opciones distintas.` };
    }
    preguntas.push(q);
  }
  return {
    cuerpo: { titulo, descripcion: String(f.descripcion || '').trim(), cierra_en: f.cierra_en || '', anonima: !!f.anonima, preguntas },
    error: null,
  };
}

/**
 * Respuestas marcadas en pantalla → cuerpo del POST /respuestas.
 * `marcadas`: { [pregunta_id]: number[] | string }. Devuelve { cuerpo, faltan } con las obligatorias sin responder.
 */
export function cuerpoRespuestas(preguntas, marcadas) {
  const respuestas = [];
  const faltan = [];
  for (const p of preguntas || []) {
    const v = marcadas[p.id];
    if (p.tipo === 'texto') {
      const t = String(v || '').trim();
      if (t) respuestas.push({ pregunta_id: p.id, texto: t });
      else if (p.obligatoria) faltan.push(p.texto);
    } else {
      const ids = Array.isArray(v) ? v : [];
      if (ids.length) respuestas.push({ pregunta_id: p.id, opcion_ids: ids });
      else if (p.obligatoria) faltan.push(p.texto);
    }
  }
  return { cuerpo: { respuestas }, faltan };
}

/** Marca o desmarca una opción respetando el tipo (única reemplaza; múltiple alterna). */
export function alternarOpcion(marcadas, pregunta, opcionId) {
  const actual = Array.isArray(marcadas[pregunta.id]) ? marcadas[pregunta.id] : [];
  let nuevo;
  if (pregunta.tipo === 'unica') nuevo = [opcionId];
  else nuevo = actual.includes(opcionId) ? actual.filter((x) => x !== opcionId) : [...actual, opcionId];
  return { ...marcadas, [pregunta.id]: nuevo };
}
