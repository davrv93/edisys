import { useState } from 'react';
import { api, lista, subir, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFecha, formatearFechaHora } from '../../lib/fechas.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Modal, Tabla, Vacio, useDialog, useToast } from '../../ui/index.js';

// «portal» va siempre: el anuncio se ve en el portal; los demás canales además lo envían.
export const CANALES = [
  { id: 'correo', etiqueta: 'Correo', icono: 'correo' },
  { id: 'whatsapp', etiqueta: 'WhatsApp', icono: 'whatsapp' },
  { id: 'telegram', etiqueta: 'Telegram', icono: 'enviar' },
];
const ETIQUETA_CANAL = { portal: 'Portal', correo: 'Correo', whatsapp: 'WhatsApp', telegram: 'Telegram' };
const ESTADO_ANUNCIO = {
  borrador: { estado: 'borrador', texto: 'Borrador' },
  publicado: { estado: 'activo', texto: 'Publicado' },
  archivado: { estado: 'descartado', texto: 'Archivado' },
};
const VACIO = { titulo: '', cuerpo: '', fecha: '', canales: ['correo'], archivo: null };

/** Bloque E2 · Anuncios y comunicados: redactar, publicar por canales y ver la evidencia de envío. */
export default function Anuncios() {
  const eid = useEid();
  const s = useSesion();
  const admin = s.tiene?.('anuncios.administrar');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [estado, setEstado] = useState('');
  const anuncios = useCarga(() => api.get(`/edificios/${eid}/anuncios`, { estado }), [eid, estado]);
  const filas = lista(anuncios.datos);
  const [form, setForm] = useState(null); // {id?, ...VACIO}
  const [evidencia, setEvidencia] = useState(null); // anuncio
  const [ocupado, setOcupado] = useState(false);

  const alternarCanal = (c) =>
    setForm((f) => ({ ...f, canales: f.canales.includes(c) ? f.canales.filter((x) => x !== c) : [...f.canales, c] }));

  const guardar = async (e) => {
    e?.preventDefault();
    setOcupado(true);
    try {
      if (form.id) {
        await api.put(`/edificios/${eid}/anuncios/${form.id}`, { titulo: form.titulo, cuerpo: form.cuerpo, fecha: form.fecha, canales: form.canales });
      } else {
        const fd = new FormData();
        fd.set('titulo', form.titulo);
        fd.set('cuerpo', form.cuerpo);
        fd.set('fecha', form.fecha || '');
        fd.set('canales', form.canales.join(','));
        if (form.archivo) fd.set('archivo', form.archivo);
        await subir(`/edificios/${eid}/anuncios`, fd);
      }
      toast('Borrador guardado.', { tipo: 'exito' });
      setForm(null);
      await anuncios.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo guardar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const publicar = async (a) => {
    const envia = (a.canales || []).filter((c) => c !== 'portal');
    const ok = await dialog.confirm({
      title: '¿Publicar el anuncio?',
      text: envia.length
        ? `Se verá en el portal y se enviará por ${envia.map((c) => ETIQUETA_CANAL[c]).join(', ')} a cada vecino. No se puede enviar dos veces.`
        : 'Se verá en el portal de los vecinos. No se envía por ningún canal.',
    });
    if (!ok) return;
    try {
      const r = await api.post(`/edificios/${eid}/anuncios/${a.id}/publicar`);
      const n = (r.envios?.correo || 0) + (r.envios?.whatsapp || 0) + (r.envios?.telegram || 0);
      const faltan = [r.sin_correo?.length ? `${r.sin_correo.length} unidad(es) sin correo` : '', r.sin_celular?.length ? `${r.sin_celular.length} sin celular` : '']
        .filter(Boolean)
        .join(' · ');
      toast(`Publicado${n ? ` · ${n} envío(s) en cola` : ''}${faltan ? ` · ${faltan}` : ''}.`, { tipo: 'exito' });
      await anuncios.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo publicar', text: err.message });
    }
  };

  const archivar = async (a) => {
    const ok = await dialog.confirm({ title: '¿Archivar el anuncio?', text: 'Deja de verse en el portal. La evidencia de envío se conserva.' });
    if (!ok) return;
    try {
      await api.post(`/edificios/${eid}/anuncios/${a.id}/archivar`);
      await anuncios.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo archivar', text: err.message });
    }
  };

  const borrar = async (a) => {
    const ok = await dialog.confirm({ title: '¿Borrar el borrador?', text: 'No se puede deshacer.', danger: true });
    if (!ok) return;
    try {
      await api.del(`/edificios/${eid}/anuncios/${a.id}`);
      await anuncios.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo borrar', text: err.message });
    }
  };

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Anuncios"
        subtitulo={admin ? 'Comunicados con evidencia de envío' : 'Comunicados de la administración'}
        ayuda="Un anuncio se ve en el portal de los vecinos. Al publicarlo puedes además enviarlo por correo, WhatsApp o Telegram; cada envío queda registrado con su estado."
        acciones={admin && <Boton icono="mas_signo" onClick={() => setForm({ ...VACIO })}>Nuevo anuncio</Boton>}
      />
      <Contenido>
        {admin && (
          <div className="flex flex-wrap items-center gap-2">
            {[['', 'Todos'], ['borrador', 'Borradores'], ['publicado', 'Publicados'], ['archivado', 'Archivados']].map(([v, t]) => (
              <Chip key={v || 'todos'} activo={estado === v} onClick={() => setEstado(v)}>{t}</Chip>
            ))}
          </div>
        )}
        {anuncios.error ? (
          <ErrorCarga error={anuncios.error} onReintentar={anuncios.recargar} />
        ) : !anuncios.datos ? (
          <Esqueleto className="h-48 w-full" />
        ) : filas.length === 0 ? (
          <Seccion>
            <Vacio icono="mensaje" titulo="Sin anuncios" texto={admin ? 'Redacta un comunicado: corte de agua, asamblea, fumigación…' : 'Cuando la administración publique un comunicado, aparecerá aquí.'} />
          </Seccion>
        ) : (
          <div className="flex flex-col gap-3">
            {filas.map((a) => {
              const est = ESTADO_ANUNCIO[a.estado] || ESTADO_ANUNCIO.borrador;
              return (
                <Seccion key={a.id}>
                  <div className="flex flex-wrap items-start gap-2">
                    <div className="min-w-0 flex-1">
                      <h2 className="text-base font-semibold text-tinta">{a.titulo}</h2>
                      <p className="text-xs text-texto-apoyo">
                        {formatearFecha(a.fecha)}
                        {admin && a.creado_por ? ` · ${a.creado_por}` : ''}
                        {admin && a.enviado_en ? ` · enviado ${formatearFechaHora(a.enviado_en)}` : ''}
                      </p>
                    </div>
                    {admin && <Insignia estado={est.estado} texto={est.texto} />}
                  </div>
                  <p className="whitespace-pre-line text-sm text-tinta">{a.cuerpo}</p>
                  {a.archivo_id && (
                    <a href={a.archivo_url || urlApi(`/archivos/${a.archivo_id}`)} target="_blank" rel="noreferrer" className="text-sm text-acento hover:text-acento-hover">
                      Ver adjunto
                    </a>
                  )}
                  {admin && (
                    <div className="flex flex-wrap items-center gap-2 border-t border-borde pt-3">
                      <span className="text-xs text-texto-apoyo">{(a.canales || []).map((c) => ETIQUETA_CANAL[c] || c).join(' · ')}</span>
                      <span className="flex-1" />
                      {a.estado === 'borrador' && (
                        <>
                          <Boton tamano="sm" variante="fantasma" onClick={() => borrar(a)}>Borrar</Boton>
                          <Boton tamano="sm" variante="secundario" onClick={() => setForm({ ...VACIO, id: a.id, titulo: a.titulo, cuerpo: a.cuerpo, fecha: a.fecha, canales: (a.canales || []).filter((c) => c !== 'portal') })}>Editar</Boton>
                          <Boton tamano="sm" icono="enviar" onClick={() => publicar(a)}>Publicar</Boton>
                        </>
                      )}
                      {a.estado !== 'borrador' && a.envios > 0 && (
                        <Boton tamano="sm" variante="secundario" onClick={() => setEvidencia(a)}>Evidencia ({a.envios})</Boton>
                      )}
                      {a.estado === 'publicado' && <Boton tamano="sm" variante="fantasma" onClick={() => archivar(a)}>Archivar</Boton>}
                    </div>
                  )}
                </Seccion>
              );
            })}
          </div>
        )}
      </Contenido>

      <Modal
        abierto={!!form}
        onCerrar={() => setForm(null)}
        titulo={form?.id ? 'Editar borrador' : 'Nuevo anuncio'}
        ancho="max-w-lg"
        pie={<><Boton variante="fantasma" onClick={() => setForm(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardar}>Guardar borrador</Boton></>}
      >
        {form && (
          <form className="flex flex-col gap-3" onSubmit={guardar}>
            <Campo etiqueta="Título" valor={form.titulo} onCambio={(v) => setForm({ ...form, titulo: v })} maxLength={160} />
            <Campo etiqueta="Contenido" tipo="textarea" valor={form.cuerpo} onCambio={(v) => setForm({ ...form, cuerpo: v })} />
            <Campo etiqueta="Fecha" tipo="fecha" valor={form.fecha} onCambio={(v) => setForm({ ...form, fecha: v })} ayuda="Vacía = hoy." />
            <fieldset className="flex flex-col gap-1.5">
              <legend className="text-sm font-semibold text-tinta">Enviar también por</legend>
              <div className="flex flex-wrap gap-2">
                {CANALES.map((c) => (
                  <Chip key={c.id} icono={c.icono} activo={form.canales.includes(c.id)} onClick={() => alternarCanal(c.id)}>{c.etiqueta}</Chip>
                ))}
              </div>
              <p className="text-xs text-texto-apoyo">Siempre se publica en el portal. El envío ocurre al publicar, una sola vez.</p>
            </fieldset>
            {!form.id && (
              <div className="flex flex-col gap-1.5">
                <label htmlFor="anuncio-archivo" className="text-sm font-semibold text-tinta">Adjunto (opcional, PDF o imagen)</label>
                <input id="anuncio-archivo" type="file" accept="application/pdf,image/*" onChange={(ev) => setForm({ ...form, archivo: ev.target.files?.[0] || null })} className="text-sm" />
              </div>
            )}
          </form>
        )}
      </Modal>

      {evidencia && <Evidencia eid={eid} anuncio={evidencia} onCerrar={() => setEvidencia(null)} />}
    </>
  );
}

/** Evidencia de envío: un renglón por destinatario y canal con el estado vivo de su bandeja. */
function Evidencia({ eid, anuncio, onCerrar }) {
  const ev = useCarga(() => api.get(`/edificios/${eid}/anuncios/${anuncio.id}/envios`), [eid, anuncio.id]);
  const resumen = ev.datos?.resumen || [];
  const columnas = [
    { clave: 'canal', titulo: 'Canal', render: (f) => ETIQUETA_CANAL[f.canal] || f.canal, movil: 'sub' },
    { clave: 'destino', titulo: 'Destino', movil: 'titulo' },
    { clave: 'unidad', titulo: 'Unidad', render: (f) => f.unidad || '—' },
    { clave: 'estado', titulo: 'Estado', render: (f) => <Insignia estado={f.estado} />, movil: 'valor2' },
    { clave: 'procesado_en', titulo: 'Procesado', render: (f) => (f.procesado_en ? formatearFechaHora(f.procesado_en) : '—'), prioridad: 2 },
    { clave: 'error', titulo: 'Error', render: (f) => f.error || '', prioridad: 3 },
  ];
  return (
    <Modal abierto onCerrar={onCerrar} titulo={`Evidencia · ${anuncio.titulo}`} ancho="max-w-3xl" pie={<Boton variante="secundario" onClick={ev.recargar}>Actualizar</Boton>}>
      <div className="flex flex-col gap-3">
        {resumen.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {resumen.map((r) => (
              <Insignia key={`${r.canal}-${r.estado}`} estado={r.estado} texto={`${ETIQUETA_CANAL[r.canal]} · ${r.cantidad} ${r.estado}`} />
            ))}
          </div>
        )}
        <Tabla columnas={columnas} filas={lista(ev.datos)} cargando={ev.cargando} error={ev.error} onReintentar={ev.recargar} etiqueta="Envíos del anuncio" densa />
      </div>
    </Modal>
  );
}
