import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useQuery } from '../../lib/nav.jsx';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Modal, Vacio, useDialog, useToast } from '../../ui/index.js';

// Bloque E5 · tres listas de contenido con la misma forma. Cada una describe su ruta y su formulario.
const TIPOS = {
  faq: {
    ruta: 'faq', titulo: 'Preguntas frecuentes', icono: 'info', nueva: 'Nueva pregunta',
    vacio: 'Responde lo que siempre preguntan: vencimientos, reservas, mascotas, mudanzas…',
    inicial: { pregunta: '', respuesta: '', categoria: 'General', orden: 0, publicado: true },
    campos: [
      { clave: 'pregunta', etiqueta: 'Pregunta' },
      { clave: 'respuesta', etiqueta: 'Respuesta', tipo: 'textarea' },
      { clave: 'categoria', etiqueta: 'Categoría' },
      { clave: 'orden', etiqueta: 'Orden', tipo: 'numero' },
    ],
  },
  academia: {
    ruta: 'academia', titulo: 'Academia', icono: 'documento', nueva: 'Nueva lección',
    vacio: 'Guías y videos cortos: cómo pagar, subir el voucher o reservar un área.',
    inicial: { titulo: '', resumen: '', tipo: 'articulo', url: '', contenido: '', orden: 0, publicado: true },
    campos: [
      { clave: 'titulo', etiqueta: 'Título' },
      { clave: 'tipo', etiqueta: 'Tipo', tipo: 'select', opciones: [{ valor: 'articulo', etiqueta: 'Artículo' }, { valor: 'video', etiqueta: 'Video' }, { valor: 'guia', etiqueta: 'Guía' }] },
      { clave: 'resumen', etiqueta: 'Resumen' },
      { clave: 'url', etiqueta: 'Enlace (video o PDF)', ayuda: 'https://… — o escribe el contenido abajo.' },
      { clave: 'contenido', etiqueta: 'Contenido', tipo: 'textarea' },
      { clave: 'orden', etiqueta: 'Orden', tipo: 'numero' },
    ],
  },
  beneficios: {
    ruta: 'beneficios', titulo: 'Beneficios', icono: 'pagado', nueva: 'Nuevo beneficio',
    vacio: 'Convenios para los vecinos: descuentos en lavandería, gimnasio, delivery…',
    inicial: { titulo: '', descripcion: '', proveedor: '', descuento: '', codigo: '', url: '', vigente_hasta: '', orden: 0, publicado: true },
    campos: [
      { clave: 'titulo', etiqueta: 'Título' },
      { clave: 'proveedor', etiqueta: 'Proveedor' },
      { clave: 'descuento', etiqueta: 'Descuento', ayuda: '«15 %», «2x1», «delivery gratis»' },
      { clave: 'codigo', etiqueta: 'Código' },
      { clave: 'descripcion', etiqueta: 'Descripción', tipo: 'textarea' },
      { clave: 'url', etiqueta: 'Enlace' },
      { clave: 'vigente_hasta', etiqueta: 'Vigente hasta', tipo: 'fecha', ayuda: 'Vacío = sin vencimiento.' },
      { clave: 'orden', etiqueta: 'Orden', tipo: 'numero' },
    ],
  },
};
const ETIQUETA_TIPO = { articulo: 'Artículo', video: 'Video', guia: 'Guía' };

/** Bloque E5 · Preguntas frecuentes, Academia y Beneficios del edificio (se ven en el portal). */
export default function Ayuda() {
  const s = useSesion();
  const admin = s.tiene?.('contenido.administrar');
  const [sp, setQuery] = useQuery();
  const vista = TIPOS[sp.get('vista')] ? sp.get('vista') : 'faq';
  const t = TIPOS[vista];
  const eid = useEid();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const datos = useCarga(() => api.get(`/edificios/${eid}/${t.ruta}`), [eid, vista]);
  const filas = lista(datos.datos);
  const [form, setForm] = useState(null); // {id?, ...campos}
  const [ocupado, setOcupado] = useState(false);

  const guardar = async (e) => {
    e?.preventDefault();
    setOcupado(true);
    try {
      const { id, ...cuerpo } = form;
      cuerpo.orden = Number(cuerpo.orden) || 0;
      if (id) await api.put(`/edificios/${eid}/${t.ruta}/${id}`, cuerpo);
      else await api.post(`/edificios/${eid}/${t.ruta}`, cuerpo);
      toast('Guardado.', { tipo: 'exito' });
      setForm(null);
      await datos.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo guardar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const alternar = async (f) => {
    try {
      await api.put(`/edificios/${eid}/${t.ruta}/${f.id}`, { publicado: !f.publicado });
      await datos.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo cambiar', text: err.message });
    }
  };

  const borrar = async (f) => {
    if (!(await dialog.confirm({ title: '¿Borrar?', text: 'No se puede deshacer. Si solo quieres esconderlo, usa «Ocultar».', danger: true }))) return;
    try {
      await api.del(`/edificios/${eid}/${t.ruta}/${f.id}`);
      await datos.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo borrar', text: err.message });
    }
  };

  // Lo que el formulario edita: solo las claves descritas (sin id ni marcas de tiempo).
  const editar = (f) => {
    const v = { id: f.id, publicado: f.publicado };
    for (const c of t.campos) v[c.clave] = f[c.clave] ?? '';
    setForm(v);
  };

  const accionesAdmin = (f) => admin && (
    <div className="flex flex-wrap items-center gap-2 pt-1">
      {!f.publicado && <Insignia estado="borrador" texto="Oculto" />}
      {f.vigente === false && <Insignia estado="vencida" texto="Vencido" />}
      <span className="flex-1" />
      <Boton tamano="sm" variante="fantasma" onClick={() => alternar(f)}>{f.publicado ? 'Ocultar' : 'Publicar'}</Boton>
      <Boton tamano="sm" variante="fantasma" onClick={() => editar(f)}>Editar</Boton>
      <Boton tamano="sm" variante="fantasma" onClick={() => borrar(f)}>Borrar</Boton>
    </div>
  );

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Ayuda y beneficios"
        subtitulo="Preguntas frecuentes, academia y beneficios del edificio"
        ayuda="Lo publicado se ve en el portal de propietarios e inquilinos. Los beneficios vencidos dejan de mostrarse solos."
        acciones={admin && <Boton icono="mas_signo" onClick={() => setForm({ ...t.inicial })}>{t.nueva}</Boton>}
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          {Object.entries(TIPOS).map(([id, x]) => (
            <Chip key={id} icono={x.icono} activo={vista === id} onClick={() => setQuery({ vista: id })}>{x.titulo}</Chip>
          ))}
        </div>
        {datos.error ? (
          <ErrorCarga error={datos.error} onReintentar={datos.recargar} />
        ) : !datos.datos ? (
          <Esqueleto className="h-48 w-full" />
        ) : filas.length === 0 ? (
          <Seccion><Vacio icono={t.icono} titulo={`Sin ${t.titulo.toLowerCase()}`} texto={admin ? t.vacio : 'La administración aún no publica nada aquí.'} /></Seccion>
        ) : vista === 'faq' ? (
          <ListaFAQ filas={filas} acciones={accionesAdmin} />
        ) : (
          <div className="grid gap-3 md:grid-cols-2">
            {filas.map((f) => (
              <Seccion key={f.id}>
                {vista === 'academia' ? <Leccion f={f} /> : <Beneficio f={f} />}
                {accionesAdmin(f)}
              </Seccion>
            ))}
          </div>
        )}
      </Contenido>

      <Modal
        abierto={!!form}
        onCerrar={() => setForm(null)}
        titulo={form?.id ? 'Editar' : t.nueva}
        ancho="max-w-lg"
        pie={<><Boton variante="fantasma" onClick={() => setForm(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardar}>Guardar</Boton></>}
      >
        {form && (
          <form className="flex flex-col gap-3" onSubmit={guardar}>
            {t.campos.map((c) => (
              <Campo key={c.clave} etiqueta={c.etiqueta} tipo={c.tipo || 'texto'} ayuda={c.ayuda} opciones={c.opciones} valor={form[c.clave] ?? ''} onCambio={(v) => setForm({ ...form, [c.clave]: v })} />
            ))}
            <label className="flex items-center gap-2 text-sm text-tinta">
              <input type="checkbox" checked={!!form.publicado} onChange={(ev) => setForm({ ...form, publicado: ev.target.checked })} />
              Visible en el portal
            </label>
          </form>
        )}
      </Modal>
    </>
  );
}

/** Preguntas agrupadas por categoría, plegables (details nativo: accesible y sin JS). */
function ListaFAQ({ filas, acciones }) {
  const grupos = new Map();
  for (const f of filas) {
    const g = f.categoria || 'General';
    if (!grupos.has(g)) grupos.set(g, []);
    grupos.get(g).push(f);
  }
  return [...grupos.entries()].map(([cat, items]) => (
    <Seccion key={cat} titulo={cat}>
      <ul className="divide-y divide-borde">
        {items.map((f) => (
          <li key={f.id} className="py-2">
            <details>
              <summary className="cursor-pointer text-sm font-medium text-tinta">{f.pregunta}</summary>
              <p className="mt-2 whitespace-pre-line text-sm text-texto-suave">{f.respuesta}</p>
            </details>
            {acciones(f)}
          </li>
        ))}
      </ul>
    </Seccion>
  ));
}

function Leccion({ f }) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center gap-2">
        <Insignia estado="validado" texto={ETIQUETA_TIPO[f.tipo] || f.tipo} icono={false} />
        <h2 className="min-w-0 flex-1 truncate text-base font-semibold">{f.titulo}</h2>
      </div>
      {f.resumen && <p className="text-sm text-texto-suave">{f.resumen}</p>}
      {f.contenido && (
        <details>
          <summary className="cursor-pointer text-sm text-acento">Leer</summary>
          <p className="mt-2 whitespace-pre-line text-sm text-tinta">{f.contenido}</p>
        </details>
      )}
      {f.url && <a href={f.url} target="_blank" rel="noreferrer noopener" className="text-sm font-semibold text-acento hover:text-acento-hover">{f.tipo === 'video' ? 'Ver video' : 'Abrir'}</a>}
    </div>
  );
}

function Beneficio({ f }) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <h2 className="text-base font-semibold">{f.titulo}</h2>
          {f.proveedor && <p className="text-xs text-texto-apoyo">{f.proveedor}</p>}
        </div>
        {f.descuento && <Insignia estado="pagado" texto={f.descuento} icono={false} tam="md" />}
      </div>
      {f.descripcion && <p className="whitespace-pre-line text-sm text-texto-suave">{f.descripcion}</p>}
      <div className="flex flex-wrap items-center gap-3 text-sm">
        {f.codigo && <span>Código: <strong className="font-mono">{f.codigo}</strong></span>}
        {f.vigente_hasta && <span className="text-xs text-texto-apoyo">Hasta el {formatearFecha(f.vigente_hasta)}</span>}
        {f.url && <a href={f.url} target="_blank" rel="noreferrer noopener" className="font-semibold text-acento hover:text-acento-hover">Ver</a>}
      </div>
    </div>
  );
}
