import { useEffect, useRef, useState } from 'react';
import { api, lista, subir } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFechaHora, haceCuanto } from '../../lib/fechas.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Modal, SubirFoto, Vacio, useDialog, useToast } from '../../ui/index.js';

const ESTADO_P = {
  recibido: { estado: 'pendiente', texto: 'En portería' },
  entregado: { estado: 'entregado', texto: 'Entregado' },
  devuelto: { estado: 'anulado', texto: 'Devuelto' },
};
const FORM = { unidad_id: '', remitente: '', descripcion: '', foto: null };

/** Bloque G5 · Registro de paquetes: recibir con aviso a la unidad y entregar con firma o foto. */
export default function Paquetes() {
  const eid = useEid();
  const s = useSesion();
  const porteria = s.tiene?.('paquetes.registrar');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [estado, setEstado] = useState('recibido');
  const [form, setForm] = useState(null);
  const [entrega, setEntrega] = useState(null); // {paquete, entregado_a, modo, firma, foto}
  const [ocupado, setOcupado] = useState(false);
  const paq = useCarga(() => api.get(`/edificios/${eid}/paquetes`, { estado }), [eid, estado]);
  const unidades = useCarga(() => api.get(`/edificios/${eid}/operacion/unidades`), [eid], { activo: !!porteria });
  const filas = lista(paq.datos);

  const recibir = async (e) => {
    e?.preventDefault?.();
    setOcupado(true);
    try {
      const fd = new FormData();
      fd.set('unidad_id', form.unidad_id);
      fd.set('remitente', form.remitente);
      fd.set('descripcion', form.descripcion);
      if (form.foto?.archivo) fd.set('foto', form.foto.archivo, 'paquete.jpg');
      const r = await subir(`/edificios/${eid}/paquetes`, fd);
      toast(r.avisado ? 'Paquete registrado; avisamos a la unidad.' : 'Paquete registrado. La unidad no tiene celular para avisarle.', { tipo: r.avisado ? 'exito' : 'aviso' });
      setForm(null);
      setEstado('recibido');
      await paq.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const entregar = async () => {
    const archivo = entrega.modo === 'firma' ? entrega.firma : entrega.foto?.archivo;
    if (!entrega.entregado_a.trim() || !archivo) {
      await dialog.alert({ title: 'Falta un dato', text: 'Escribe quién recoge y toma su firma o una foto.' });
      return;
    }
    setOcupado(true);
    try {
      const fd = new FormData();
      fd.set('entregado_a', entrega.entregado_a);
      fd.set(entrega.modo === 'firma' ? 'firma' : 'foto', archivo, entrega.modo === 'firma' ? 'firma.png' : 'entrega.jpg');
      await subir(`/edificios/${eid}/paquetes/${entrega.paquete.id}/entregar`, fd);
      toast('Entrega registrada.', { tipo: 'exito' });
      setEntrega(null);
      await paq.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar la entrega', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const devolver = async (p) => {
    const motivo = await dialog.prompt({ title: 'Devolver al courier', label: 'Motivo', required: true, okText: 'Devolver' });
    if (motivo == null) return;
    try {
      await api.post(`/edificios/${eid}/paquetes/${p.id}/devolver`, { motivo });
      await paq.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo devolver', text: err.message });
    }
  };

  const reavisar = async (p) => {
    try {
      await api.post(`/edificios/${eid}/paquetes/${p.id}/reavisar`);
      toast('Aviso enviado otra vez.', { tipo: 'exito' });
      await paq.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo avisar', text: err.message });
    }
  };

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Paquetes"
        subtitulo={porteria ? 'Recepción en portería y entregas' : 'Paquetes que llegaron para tu unidad'}
        ayuda="Al registrar un paquete avisamos a la unidad por WhatsApp. Para entregarlo se anota quién lo recoge con su firma o una foto; sin eso no hay entrega."
        acciones={porteria && <Boton icono="mas_signo" onClick={() => setForm({ ...FORM })}>Recibir paquete</Boton>}
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          {Object.entries(ESTADO_P).map(([k, v]) => (
            <Chip key={k} activo={estado === k} onClick={() => setEstado(k)} contador={k === 'recibido' ? paq.datos?.pendientes : undefined}>
              {v.texto}
            </Chip>
          ))}
          <Chip activo={estado === ''} onClick={() => setEstado('')}>Todos</Chip>
        </div>
        <Seccion titulo="Paquetes">
          {paq.error ? (
            <ErrorCarga error={paq.error} onReintentar={paq.recargar} />
          ) : !paq.datos ? (
            <Esqueleto className="h-40 w-full" />
          ) : filas.length === 0 ? (
            <Vacio icono="bandeja" titulo="Sin paquetes" texto={porteria ? 'Registra aquí cada paquete que llegue a portería.' : 'Cuando llegue algo para ti, lo verás aquí.'} />
          ) : (
            <ul className="divide-y divide-borde">
              {filas.map((p) => (
                <li key={p.id} className="flex flex-col gap-2 py-3 sm:flex-row sm:items-center sm:gap-3">
                  {p.foto_url && (
                    <a href={p.foto_url} target="_blank" rel="noreferrer" className="shrink-0">
                      <img src={p.foto_url} alt="Foto del paquete" className="h-12 w-12 rounded-control border border-borde object-cover" />
                    </a>
                  )}
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm text-tinta">
                      <b>Dpto {p.unidad}</b> · {p.descripcion}
                      {p.remitente && <span className="text-texto-apoyo"> · de {p.remitente}</span>}
                    </p>
                    <p className="truncate text-xs text-texto-apoyo">
                      {p.estado === 'entregado'
                        ? `Entregado a ${p.entregado_a} · ${formatearFechaHora(p.entregado_en)}${p.entregado_por ? ` · ${p.entregado_por}` : ''}`
                        : p.estado === 'devuelto'
                          ? `Devuelto: ${p.motivo_devolucion}`
                          : [`Llegó ${haceCuanto(p.recibido_en)}`, p.recibido_por && `recibió ${p.recibido_por}`, p.aviso_en ? `avisado ${haceCuanto(p.aviso_en)}` : 'sin aviso'].filter(Boolean).join(' · ')}
                    </p>
                  </div>
                  {p.entrega_firma_url && (
                    <a href={p.entrega_firma_url} target="_blank" rel="noreferrer" className="text-sm text-acento hover:text-acento-hover">Firma</a>
                  )}
                  <Insignia estado={ESTADO_P[p.estado]?.estado} texto={ESTADO_P[p.estado]?.texto} />
                  {porteria && p.estado === 'recibido' && (
                    <div className="flex gap-1">
                      <Boton tamano="sm" onClick={() => setEntrega({ paquete: p, entregado_a: '', modo: 'firma', firma: null, foto: null })}>Entregar</Boton>
                      <Boton tamano="sm" variante="fantasma" onClick={() => reavisar(p)}>Avisar</Boton>
                      <Boton tamano="sm" variante="fantasma" className="!text-alerta" onClick={() => devolver(p)}>Devolver</Boton>
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}
        </Seccion>
      </Contenido>

      <Modal abierto={!!form} onCerrar={() => setForm(null)} titulo="Recibir paquete" ancho="max-w-md" pie={<><Boton variante="fantasma" onClick={() => setForm(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={recibir}>Registrar y avisar</Boton></>}>
        {form && (
          <form className="flex flex-col gap-3" onSubmit={recibir}>
            <Campo etiqueta="Unidad destinataria" tipo="select" valor={form.unidad_id} onCambio={(v) => setForm({ ...form, unidad_id: v })} opciones={[{ valor: '', etiqueta: 'Elige…' }, ...lista(unidades.datos).map((u) => ({ valor: String(u.id), etiqueta: `Dpto ${u.codigo}${u.residente ? ` · ${u.residente}` : ''}` }))]} />
            <Campo etiqueta="Descripción" valor={form.descripcion} onCambio={(v) => setForm({ ...form, descripcion: v })} placeholder="Caja mediana, sobre, bolsa…" />
            <Campo etiqueta="Remitente o courier" valor={form.remitente} onCambio={(v) => setForm({ ...form, remitente: v })} />
            <SubirFoto foto={form.foto} onFoto={(foto) => setForm((f) => ({ ...f, foto }))} etiqueta="Foto del paquete (opcional)" alto="h-40" oscuro={false} />
          </form>
        )}
      </Modal>

      <Modal abierto={!!entrega} onCerrar={() => setEntrega(null)} titulo={entrega ? `Entregar a Dpto ${entrega.paquete.unidad}` : ''} ancho="max-w-md" pie={<><Boton variante="fantasma" onClick={() => setEntrega(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={entregar}>Registrar entrega</Boton></>}>
        {entrega && (
          <div className="flex flex-col gap-3">
            <Campo etiqueta="Quién recoge" valor={entrega.entregado_a} onCambio={(v) => setEntrega({ ...entrega, entregado_a: v })} />
            <div className="flex gap-2">
              <Chip activo={entrega.modo === 'firma'} onClick={() => setEntrega({ ...entrega, modo: 'firma' })}>Firma</Chip>
              <Chip activo={entrega.modo === 'foto'} icono="camara" onClick={() => setEntrega({ ...entrega, modo: 'foto' })}>Foto</Chip>
            </div>
            {entrega.modo === 'firma' ? (
              <PadFirma onFirma={(firma) => setEntrega((x) => ({ ...x, firma }))} />
            ) : (
              <SubirFoto foto={entrega.foto} onFoto={(foto) => setEntrega((x) => ({ ...x, foto }))} etiqueta="Foto de quien recoge" alto="h-48" oscuro={false} />
            )}
          </div>
        )}
      </Modal>
    </>
  );
}

/** Firma con el dedo o el mouse sobre un lienzo; entrega un PNG (Blob) o null si se borra. */
function PadFirma({ onFirma }) {
  const lienzo = useRef(null);
  const dibujando = useRef(false);
  const [vacio, setVacio] = useState(true);

  useEffect(() => {
    const c = lienzo.current;
    const escala = window.devicePixelRatio || 1;
    c.width = c.offsetWidth * escala;
    c.height = c.offsetHeight * escala;
    const ctx = c.getContext('2d');
    ctx.scale(escala, escala);
    ctx.lineWidth = 2.2;
    ctx.lineCap = 'round';
    ctx.strokeStyle = '#111';
    ctx.fillStyle = '#fff';
    ctx.fillRect(0, 0, c.width, c.height);
  }, []);

  const punto = (e) => {
    const r = lienzo.current.getBoundingClientRect();
    return [e.clientX - r.left, e.clientY - r.top];
  };
  const empezar = (e) => {
    e.preventDefault();
    lienzo.current.setPointerCapture?.(e.pointerId);
    dibujando.current = true;
    const ctx = lienzo.current.getContext('2d');
    ctx.beginPath();
    ctx.moveTo(...punto(e));
  };
  const mover = (e) => {
    if (!dibujando.current) return;
    const ctx = lienzo.current.getContext('2d');
    ctx.lineTo(...punto(e));
    ctx.stroke();
  };
  const terminar = () => {
    if (!dibujando.current) return;
    dibujando.current = false;
    setVacio(false);
    lienzo.current.toBlob((b) => onFirma(b), 'image/png');
  };
  const borrar = () => {
    const c = lienzo.current;
    const ctx = c.getContext('2d');
    ctx.save();
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.fillRect(0, 0, c.width, c.height);
    ctx.restore();
    setVacio(true);
    onFirma(null);
  };

  return (
    <div className="flex flex-col gap-1.5">
      <canvas
        ref={lienzo}
        className="h-40 w-full touch-none rounded-control border border-borde-fuerte bg-white"
        aria-label="Espacio para firmar"
        onPointerDown={empezar}
        onPointerMove={mover}
        onPointerUp={terminar}
        onPointerLeave={terminar}
      />
      <div className="flex items-center justify-between text-xs text-texto-apoyo">
        <span>{vacio ? 'Firma aquí con el dedo.' : 'Firma lista.'}</span>
        <button type="button" onClick={borrar} className="text-acento hover:text-acento-hover">Borrar</button>
      </div>
    </div>
  );
}
