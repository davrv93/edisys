import { useState } from 'react';
import { api, lista, subir, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Modal, Vacio, useDialog, useToast } from '../../ui/index.js';

/** Bloque E1 · Documentos del edificio por categorías (actas, reglamentos…). */
export default function Documentos() {
  const eid = useEid();
  const s = useSesion();
  const admin = s.tiene?.('documentos.administrar');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [cat, setCat] = useState('');
  const [vista, setVista] = useState('documentos');

  const cats = useCarga(() => api.get(`/edificios/${eid}/documentos/categorias`), [eid]);
  const docs = useCarga(() => api.get(`/edificios/${eid}/documentos`, { categoria_id: cat }), [eid, cat]);
  const listaCats = lista(cats.datos);
  const listaDocs = lista(docs.datos);

  const [formCat, setFormCat] = useState(null);
  const [formDoc, setFormDoc] = useState(null); // {categoria_id, titulo, numero, resumen, archivo}
  const [ocupado, setOcupado] = useState(false);

  const guardarCat = async (e) => {
    e.preventDefault();
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/documentos/categorias`, formCat);
      toast('Categoría creada.', { tipo: 'exito' });
      setFormCat(null);
      await cats.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo crear', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const guardarDoc = async (e) => {
    e.preventDefault();
    if (!formDoc.archivo) {
      await dialog.alert({ title: 'Falta el archivo', text: 'Adjunta el documento (PDF o imagen).' });
      return;
    }
    setOcupado(true);
    try {
      const fd = new FormData();
      fd.set('categoria_id', formDoc.categoria_id);
      fd.set('titulo', formDoc.titulo);
      fd.set('numero', formDoc.numero || '');
      fd.set('resumen', formDoc.resumen || '');
      fd.set('archivo', formDoc.archivo);
      await subir(`/edificios/${eid}/documentos`, fd);
      toast('Documento publicado.', { tipo: 'exito' });
      setFormDoc(null);
      await Promise.all([docs.recargar(), cats.recargar()]);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo subir', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const alternar = async (d) => {
    try {
      await api.post(`/edificios/${eid}/documentos/${d.id}/publicar`, { publicado: !d.publicado });
      await docs.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo cambiar', text: err.message });
    }
  };

  const acciones = admin && (
    <>
      {vista === 'categorias' ? (
        <Boton icono="mas_signo" onClick={() => setFormCat({ nombre: '' })}>Nueva categoría</Boton>
      ) : (
        <Boton icono="mas_signo" onClick={() => setFormDoc({ categoria_id: listaCats[0] ? String(listaCats[0].id) : '', titulo: '', numero: '', resumen: '', archivo: null })}>Subir documento</Boton>
      )}
    </>
  );

  return (
    <>
      {dialogEl}
      <Encabezado titulo="Documentos" subtitulo="Actas, reglamentos y comunicados del edificio" ayuda="Publica documentos por categorías para que los propietarios los consulten. El archivo queda privado, con enlace firmado." acciones={acciones} />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'documentos'} icono="recibo" onClick={() => setVista('documentos')} contador={listaDocs.length}>Documentos</Chip>
          <Chip activo={vista === 'categorias'} icono="edificio" onClick={() => setVista('categorias')} contador={listaCats.length}>Categorías</Chip>
        </div>

        {vista === 'categorias' ? (
          <Seccion titulo="Categorías">
            {cats.error ? (
              <ErrorCarga error={cats.error} onReintentar={cats.recargar} />
            ) : !cats.datos ? (
              <Esqueleto className="h-40 w-full" />
            ) : listaCats.length === 0 ? (
              <Vacio icono="edificio" titulo="Sin categorías" texto="Crea categorías como «Actas», «Reglamentos» o «Libro de reclamaciones»." />
            ) : (
              <ul className="divide-y divide-borde">
                {listaCats.map((c) => (
                  <li key={c.id} className="flex items-center gap-3 py-2.5">
                    <span className="min-w-0 flex-1 text-sm font-medium text-tinta">{c.nombre}</span>
                    <span className="text-xs text-texto-apoyo">{c.documentos} documento(s)</span>
                    <button type="button" onClick={() => setCat(String(c.id))} className="text-sm text-acento hover:text-acento-hover">Ver</button>
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        ) : (
          <Seccion titulo="Documentos" extra={cat && <Chip onQuitar={() => setCat('')}>{listaCats.find((c) => String(c.id) === cat)?.nombre || 'Categoría'}</Chip>}>
            {docs.error ? (
              <ErrorCarga error={docs.error} onReintentar={docs.recargar} />
            ) : !docs.datos ? (
              <Esqueleto className="h-40 w-full" />
            ) : listaDocs.length === 0 ? (
              <Vacio icono="recibo" titulo="Sin documentos" texto="Sube actas, reglamentos o el libro de reclamaciones." />
            ) : (
              <ul className="divide-y divide-borde">
                {listaDocs.map((d) => (
                  <li key={d.id} className="flex items-center gap-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-tinta">{d.titulo}</p>
                      <p className="truncate text-xs text-texto-apoyo">{[d.categoria, d.numero, formatearFecha(d.fecha)].filter(Boolean).join(' · ')}</p>
                    </div>
                    {!d.publicado && <Insignia estado="borrador" texto="Oculto" />}
                    <a href={d.archivo_url || urlApi(`/archivos/${d.archivo_id}`)} target="_blank" rel="noreferrer" className="text-sm text-acento hover:text-acento-hover">Abrir</a>
                    {admin && (
                      <Boton tamano="sm" variante="fantasma" onClick={() => alternar(d)}>{d.publicado ? 'Ocultar' : 'Publicar'}</Boton>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        )}
      </Contenido>

      <Modal abierto={!!formCat} onCerrar={() => setFormCat(null)} titulo="Nueva categoría" ancho="max-w-sm" pie={<><Boton variante="fantasma" onClick={() => setFormCat(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarCat}>Crear</Boton></>}>
        {formCat && <Campo etiqueta="Nombre" valor={formCat.nombre} onCambio={(v) => setFormCat({ ...formCat, nombre: v })} />}
      </Modal>

      <Modal abierto={!!formDoc} onCerrar={() => setFormDoc(null)} titulo="Subir documento" ancho="max-w-lg" pie={<><Boton variante="fantasma" onClick={() => setFormDoc(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarDoc}>Publicar</Boton></>}>
        {formDoc && (
          <form className="flex flex-col gap-3" onSubmit={guardarDoc}>
            <Campo etiqueta="Categoría" tipo="select" valor={formDoc.categoria_id} onCambio={(v) => setFormDoc({ ...formDoc, categoria_id: v })} opciones={listaCats.map((c) => ({ valor: String(c.id), etiqueta: c.nombre }))} />
            <Campo etiqueta="Título" valor={formDoc.titulo} onCambio={(v) => setFormDoc({ ...formDoc, titulo: v })} />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Número" valor={formDoc.numero} onCambio={(v) => setFormDoc({ ...formDoc, numero: v })} />
              <Campo etiqueta="Resumen" valor={formDoc.resumen} onCambio={(v) => setFormDoc({ ...formDoc, resumen: v })} />
            </div>
            <div className="flex flex-col gap-1.5">
              <label htmlFor="doc-archivo" className="text-sm font-semibold text-tinta">Archivo (PDF o imagen)</label>
              <input id="doc-archivo" type="file" accept="application/pdf,image/*" onChange={(ev) => setFormDoc({ ...formDoc, archivo: ev.target.files?.[0] || null })} className="text-sm" />
            </div>
          </form>
        )}
      </Modal>
    </>
  );
}
