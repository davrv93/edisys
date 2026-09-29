import { useRef, useState } from 'react';
import { subir } from '../../lib/api.js';
import { comprimirImagen } from '../../lib/imagen.js';
import { CATEGORIAS } from '../../lib/kanban.js';
import { ruta } from '../../lib/nav.jsx';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import { useModoTarea } from '../../layout/Armazon.jsx';
import { CabeceraTarea } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Icono, Spinner, useDialog } from '../../ui/index.js';

const LUGARES = ['Piso 1 – pasadizo', 'Piso 2 – pasadizo', 'Piso 3 – pasadizo', 'Escalera', 'Cuarto de bombas', 'Cochera', 'Hall de ingreso', 'Azotea', 'Ascensor'];
const MAX_FOTOS = 5;

/** 09 · Reportar una incidencia desde el celular: fotos (obligatoria al menos una), tipo, dónde y qué pasa. */
export default function Reportar() {
  useModoTarea();
  const eid = useEid();
  const s = useSesion();
  const { dialog, dialogEl } = useDialog();
  const input = useRef(null);
  const [fotos, setFotos] = useState([]);
  const [procesando, setProcesando] = useState(false);
  const [categoria, setCategoria] = useState('estructura');
  const [donde, setDonde] = useState('');
  const [que, setQue] = useState('');
  const [progreso, setProgreso] = useState(null);
  const [enviado, setEnviado] = useState(null);
  const volver = s.tiene('portal.ver') ? ruta('portal') : s.tiene('incidencias.ver') ? ruta('mantenimiento') : ruta('medidores');
  const unidad = s.unidades?.[0]?.codigo;
  const subtitulo = `${unidad ? `Dpto ${unidad} · ` : ''}${s.edificio.nombre}`;

  const agregar = async (e) => {
    const archivos = [...(e.target.files || [])].slice(0, MAX_FOTOS - fotos.length);
    e.target.value = '';
    if (!archivos.length) return;
    setProcesando(true);
    try {
      const nuevas = [];
      for (const f of archivos) {
        if (!f.type.startsWith('image/')) continue;
        const r = await comprimirImagen(f);
        nuevas.push({ ...r, url: URL.createObjectURL(r.archivo) });
      }
      setFotos((l) => [...l, ...nuevas]);
    } finally {
      setProcesando(false);
    }
  };

  const quitar = (i) =>
    setFotos((l) => {
      URL.revokeObjectURL(l[i].url);
      return l.filter((_, j) => j !== i);
    });

  const listo = fotos.length > 0 && donde.trim() && que.trim();

  const enviar = async () => {
    if (!listo) return;
    const form = new FormData();
    form.set('descripcion', que.trim());
    form.set('ubicacion', donde.trim());
    form.set('categoria', categoria);
    fotos.forEach((f) => form.append('fotos[]', f.archivo));
    setProgreso(0);
    try {
      const r = await subir(`/edificios/${eid}/incidencias`, form, { onProgreso: setProgreso });
      setEnviado(r || {});
    } catch (err) {
      await dialog.alert({ title: 'No se pudo enviar el reporte', text: err.status === 0 ? 'Revisa tu conexión e intenta otra vez. Tus fotos siguen aquí.' : err.message });
    } finally {
      setProgreso(null);
    }
  };

  if (enviado) {
    return (
      <div className="flex min-h-screen flex-col bg-fondo">
        <CabeceraTarea titulo="Reporte enviado" subtitulo={subtitulo} volver={volver} />
        <div className="mx-auto flex w-full max-w-xl flex-col gap-4 p-4">
          <div className="flex flex-col gap-2 rounded-xl border border-acento-borde bg-acento-suave p-5 text-acento-hover">
            <Icono nombre="check" tam={28} />
            <span className="font-titulo text-2xl font-semibold">{enviado.codigo || 'Reporte recibido'}</span>
            <span className="text-base">La administración lo revisará y fijará la urgencia. Verás aquí cada cambio de estado.</span>
            <div className="mt-2 flex gap-1" aria-label="Estado: reportado">
              {Array.from({ length: 6 }, (_, i) => (
                <span key={i} className={`h-1.5 flex-1 rounded-full ${i === 0 ? 'bg-acento' : 'bg-borde'}`} />
              ))}
            </div>
            <span className="text-xs">Reportado · la administración lo revisará</span>
          </div>
          <Boton href={volver} tamano="lg" bloque>
            Listo
          </Boton>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen flex-col bg-fondo">
      {dialogEl}
      <CabeceraTarea titulo="Reportar incidencia" subtitulo={subtitulo} volver={volver} />
      <form
        className="mx-auto flex w-full max-w-xl flex-1 flex-col gap-5 p-4 pb-32"
        onSubmit={(e) => {
          e.preventDefault();
          enviar();
        }}
      >
        <section className="flex flex-col gap-2">
          <span className="text-sm font-semibold">Fotos</span>
          <input ref={input} type="file" accept="image/*" capture="environment" multiple className="sr-only" onChange={agregar} aria-label="Tomar foto" />
          <div className="grid grid-cols-3 gap-2">
            {fotos.map((f, i) => (
              <div key={f.url} className="relative aspect-square overflow-hidden rounded-lg bg-superficie-oscura">
                <img src={f.url} alt={`Foto ${i + 1}`} className="h-full w-full object-cover" />
                <button type="button" onClick={() => quitar(i)} aria-label={`Quitar foto ${i + 1}`} className="absolute right-1 top-1 flex h-8 w-8 items-center justify-center rounded-full bg-tinta/70 text-white">
                  <Icono nombre="cerrar" tam={14} />
                </button>
              </div>
            ))}
            {fotos.length < MAX_FOTOS && (
              <button type="button" onClick={() => input.current?.click()} disabled={procesando} aria-label="Tomar otra foto" className="flex aspect-square flex-col items-center justify-center gap-1 rounded-lg border-2 border-dashed border-acento-borde bg-acento-suave text-xs font-semibold text-acento">
                {procesando ? <Spinner /> : <Icono nombre="camara" tam={24} />}
                {fotos.length ? 'Agregar' : 'Tomar foto'}
              </button>
            )}
          </div>
          {fotos.length === 0 && <span className="text-xs text-texto-apoyo">Al menos una foto es obligatoria.</span>}
        </section>

        <section className="flex flex-col gap-2">
          <span className="text-sm font-semibold">Tipo</span>
          <div className="flex flex-wrap gap-2">
            {CATEGORIAS.map((c) => (
              <button key={c.valor} type="button" aria-pressed={categoria === c.valor} onClick={() => setCategoria(c.valor)} className={`h-11 rounded-full border px-4 text-sm font-semibold ${categoria === c.valor ? 'border-acento bg-acento text-white' : 'border-borde-fuerte bg-superficie'}`}>
                {c.etiqueta}
              </button>
            ))}
          </div>
        </section>

        <Campo etiqueta="¿Dónde?" valor={donde} onCambio={setDonde} placeholder="Escalera, piso 2, muro junto a la ventana" list="lugares" alto="h-12" />
        <datalist id="lugares">
          {LUGARES.map((l) => (
            <option key={l} value={l} />
          ))}
        </datalist>
        <Campo etiqueta="¿Qué pasa?" tipo="textarea" valor={que} onCambio={setQue} rows={3} placeholder="Cuenta en pocas palabras qué viste." />
        <p className="rounded-lg bg-superficie-2 p-3 text-sm text-egreso">La administración valida el reporte y fija la urgencia. Verás aquí el informe, el costo y cada cambio de estado.</p>
        {progreso != null && (
          <div className="h-2 rounded-full bg-superficie-2" role="progressbar" aria-valuenow={progreso} aria-valuemin={0} aria-valuemax={100} aria-label="Enviando reporte">
            <div className="h-2 rounded-full bg-acento transition-all" style={{ width: `${progreso}%` }} />
          </div>
        )}
      </form>
      <footer className="fixed inset-x-0 bottom-0 z-20 border-t border-borde bg-superficie p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
        <div className="mx-auto max-w-xl">
          <Boton tamano="lg" bloque disabled={!listo} cargando={progreso != null} onClick={enviar}>
            Enviar reporte
          </Boton>
        </div>
      </footer>
    </div>
  );
}
