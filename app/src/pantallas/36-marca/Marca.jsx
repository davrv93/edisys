import { useEffect, useState } from 'react';
import { api, subir, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { contraste, CONTRASTE_MINIMO } from '../../lib/marca.js';
import { formatearFechaHora } from '../../lib/fechas.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Modal, useDialog, useToast } from '../../ui/index.js';

/**
 * Bloques I2 e I1 · Marca blanca de la administradora (nombre, colores, logo: app, login y correo)
 * y plantilla del recibo en PDF del edificio (color, logo y bloques opcionales).
 */
export default function Marca() {
  const s = useSesion();
  const puedeMarca = s.tiene('marca.configurar');
  const puedePlantilla = s.tiene('recibos.plantilla');
  const [vista, setVista] = useState(puedeMarca ? 'marca' : 'plantilla');
  return (
    <>
      <Encabezado
        titulo="Marca y recibo"
        subtitulo="Tu marca en la app, el login, el correo y el recibo"
        ayuda="La marca es de la administradora: vale para todos sus edificios. La plantilla del recibo es de este edificio."
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          {puedeMarca && <Chip activo={vista === 'marca'} icono="llave" onClick={() => setVista('marca')}>Marca</Chip>}
          {puedePlantilla && <Chip activo={vista === 'plantilla'} icono="recibo" onClick={() => setVista('plantilla')}>Plantilla del recibo</Chip>}
        </div>
        {vista === 'marca' && puedeMarca ? <PanelMarca /> : <PanelPlantilla />}
      </Contenido>
    </>
  );
}

// ---------- I2 · marca ----------

function AvisoContraste({ color, fondo = '#FFFFFF' }) {
  const c = contraste(color, fondo);
  if (!c) return null;
  const ok = c >= CONTRASTE_MINIMO;
  return (
    <p className={`text-xs ${ok ? 'text-texto-apoyo' : 'text-alerta'}`}>
      Contraste con texto blanco: {c.toFixed(1).replace('.', ',')}:1 {ok ? '· se lee bien' : `· mínimo ${String(CONTRASTE_MINIMO).replace('.', ',')}:1`}
    </p>
  );
}

function CampoColor({ etiqueta, valor, onCambio, ayuda, error, vacioPermitido = false }) {
  const hex = /^#[0-9A-Fa-f]{6}$/.test(valor || '') ? valor : '#155E75';
  return (
    <div className="flex items-end gap-2">
      <input
        type="color"
        aria-label={`${etiqueta}: elegir color`}
        value={hex}
        onChange={(e) => onCambio(e.target.value.toUpperCase())}
        className="h-11 w-12 shrink-0 cursor-pointer rounded-control border border-borde-fuerte bg-superficie p-1 lg:h-9"
      />
      <Campo className="flex-1" etiqueta={etiqueta} valor={valor} onCambio={(v) => onCambio(v.toUpperCase())} ayuda={ayuda} error={error} placeholder={vacioPermitido ? 'El de la marca' : '#RRGGBB'} />
    </div>
  );
}

function PanelMarca() {
  const eid = useEid();
  const s = useSesion();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const m = useCarga(() => api.get(`/edificios/${eid}/marca`), [eid]);
  const [form, setForm] = useState(null);
  const [errores, setErrores] = useState({});
  const [ocupado, setOcupado] = useState(false);

  useEffect(() => {
    if (!m.datos) return;
    const g = m.datos.guardada || {};
    setForm({
      nombre: g.nombre || '',
      lema: g.lema || '',
      slug: m.datos.slug || '',
      color_primario: g.color_primario || m.datos.color_primario,
      color_fondo_login: g.color_fondo_login || m.datos.color_fondo_login,
    });
  }, [m.datos]);

  if (m.error) return <ErrorCarga error={m.error} onReintentar={m.recargar} />;
  if (!m.datos || !form) return <Esqueleto className="h-64 w-full" />;

  const cambiar = (k) => (v) => setForm({ ...form, [k]: v });

  const guardar = async (e) => {
    e?.preventDefault();
    setOcupado(true);
    setErrores({});
    try {
      await api.put(`/edificios/${eid}/marca`, form);
      toast('Marca guardada. La app ya la usa.', { tipo: 'exito' });
      await Promise.all([m.recargar(), s.recargar()]);
    } catch (err) {
      setErrores(err.campos || {});
      if (!Object.keys(err.campos || {}).length) await dialog.alert({ title: 'No se pudo guardar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const subirLogo = async (archivo) => {
    if (!archivo) return;
    const fd = new FormData();
    fd.set('logo', archivo);
    setOcupado(true);
    try {
      await subir(`/edificios/${eid}/marca/logo`, fd);
      toast('Logo actualizado.', { tipo: 'exito' });
      await Promise.all([m.recargar(), s.recargar()]);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo subir el logo', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const quitarLogo = async () => {
    const ok = await dialog.confirm({ title: '¿Quitar el logo?', text: 'La app, el login y el recibo mostrarán el nombre en texto.', okText: 'Quitar logo', danger: true });
    if (!ok) return;
    try {
      await api.del(`/edificios/${eid}/marca/logo`);
      await Promise.all([m.recargar(), s.recargar()]);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo quitar', text: err.message });
    }
  };

  const enlaceLogin = form.slug ? `${window.location.origin}${urlApi('').replace(/\/api\/v1$/, '')}/login/?marca=${form.slug}` : '';

  return (
    <>
      {dialogEl}
      <div className="grid gap-4 lg:grid-cols-[1fr_340px]">
        <Seccion titulo="Datos de la marca">
          <form className="flex flex-col gap-3" onSubmit={guardar}>
            <div className="grid gap-3 sm:grid-cols-2">
              <Campo etiqueta="Nombre comercial" valor={form.nombre} onCambio={cambiar('nombre')} error={errores.nombre} placeholder="EDISYS" />
              <Campo etiqueta="Lema (login)" valor={form.lema} onCambio={cambiar('lema')} error={errores.lema} placeholder="Opcional" />
            </div>
            <Campo
              etiqueta="Identificador para el login"
              valor={form.slug}
              onCambio={(v) => cambiar('slug')(v.toLowerCase())}
              error={errores.slug}
              ayuda={enlaceLogin ? `Comparte ${enlaceLogin}` : 'Minúsculas, cifras y guiones. Con él, el login sale con tu marca.'}
            />
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="flex flex-col gap-1">
                <CampoColor etiqueta="Color principal" valor={form.color_primario} onCambio={cambiar('color_primario')} error={errores.color_primario} />
                <AvisoContraste color={form.color_primario} />
              </div>
              <CampoColor etiqueta="Fondo del login" valor={form.color_fondo_login} onCambio={cambiar('color_fondo_login')} error={errores.color_fondo_login} />
            </div>
            <div className="flex justify-end">
              <Boton type="submit" cargando={ocupado}>Guardar marca</Boton>
            </div>
          </form>
        </Seccion>

        <div className="flex flex-col gap-4">
          <Seccion titulo="Logo">
            <div className="flex min-h-20 items-center justify-center rounded-control border border-dashed border-borde-fuerte bg-fondo p-3">
              {m.datos.logo_url ? <img src={m.datos.logo_url} alt="Logo actual" className="max-h-16 max-w-full object-contain" /> : <span className="text-sm text-texto-apoyo">Sin logo: se usa el nombre</span>}
            </div>
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="font-semibold text-tinta">Subir logo (PNG, JPG o WebP, hasta 2 MB)</span>
              <input type="file" accept="image/png,image/jpeg,image/webp" disabled={ocupado} onChange={(ev) => subirLogo(ev.target.files?.[0])} className="text-sm" />
            </label>
            {m.datos.logo_url && (
              <Boton variante="fantasma" tamano="sm" onClick={quitarLogo}>Quitar logo</Boton>
            )}
          </Seccion>

          <Seccion titulo="Así se ve">
            <div className="overflow-hidden rounded-control border border-borde">
              <div className="flex items-center justify-between px-3 py-2 text-white" style={{ background: form.color_primario }}>
                <span className="truncate font-semibold">{form.nombre || 'EDISYS'}</span>
                <span className="text-xs">Recibo</span>
              </div>
              <div className="flex items-center gap-2 p-3" style={{ background: form.color_fondo_login }}>
                <span className="rounded-control bg-white px-3 py-1.5 text-xs text-tinta">Login</span>
                <span className="rounded-control px-3 py-1.5 text-xs font-semibold text-white" style={{ background: form.color_primario }}>Ingresar</span>
              </div>
            </div>
          </Seccion>
        </div>
      </div>
    </>
  );
}

// ---------- I1 · plantilla del recibo ----------

const BLOQUES = [
  { clave: 'contometro', etiqueta: 'Contómetro', ayuda: 'Lectura anterior, actual y consumo de agua.' },
  { clave: 'fotos', etiqueta: 'Foto del medidor', ayuda: 'La foto de la lectura del periodo.' },
  { clave: 'qr', etiqueta: 'Código QR', ayuda: 'Abre el recibo en la app para pagarlo.' },
  { clave: 'barras', etiqueta: 'Código de barras', ayuda: 'Code 128 con el número del recibo.' },
];

function Casilla({ etiqueta, ayuda, marcado, onCambio }) {
  return (
    <label className="flex cursor-pointer items-start gap-2.5 rounded-control border border-borde p-3 hover:bg-fondo">
      <input type="checkbox" checked={!!marcado} onChange={(e) => onCambio(e.target.checked)} className="mt-0.5 h-4 w-4 accent-[var(--color-acento)]" />
      <span className="flex flex-col">
        <span className="text-sm font-semibold text-tinta">{etiqueta}</span>
        {ayuda && <span className="text-xs text-texto-apoyo">{ayuda}</span>}
      </span>
    </label>
  );
}

function PanelPlantilla() {
  const eid = useEid();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const p = useCarga(() => api.get(`/edificios/${eid}/plantilla-recibo`), [eid]);
  const [form, setForm] = useState(null);
  const [errores, setErrores] = useState({});
  const [ocupado, setOcupado] = useState(false);
  const [vistaPrevia, setVistaPrevia] = useState(null); // URL del blob

  useEffect(() => {
    if (p.datos) setForm({ ...p.datos.config, bloques: { ...(p.datos.config?.bloques || {}) } });
  }, [p.datos]);
  useEffect(() => () => vistaPrevia && URL.revokeObjectURL(vistaPrevia), [vistaPrevia]);

  if (p.error) return <ErrorCarga error={p.error} onReintentar={p.recargar} />;
  if (!p.datos || !form) return <Esqueleto className="h-64 w-full" />;

  const cambiar = (k) => (v) => setForm({ ...form, [k]: v });
  const bloque = (k) => (v) => setForm({ ...form, bloques: { ...form.bloques, [k]: v } });

  const guardar = async (e) => {
    e?.preventDefault();
    setOcupado(true);
    setErrores({});
    try {
      await api.put(`/edificios/${eid}/plantilla-recibo`, { config: form });
      toast('Plantilla guardada: los próximos PDF salen así.', { tipo: 'exito' });
      await p.recargar();
    } catch (err) {
      setErrores(err.campos || {});
      if (!Object.keys(err.campos || {}).length) await dialog.alert({ title: 'No se pudo guardar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  // La vista previa es un PDF: se pide con fetch (no JSON) y se muestra desde un blob.
  const previsualizar = async () => {
    setOcupado(true);
    try {
      const res = await fetch(urlApi(`/edificios/${eid}/plantilla-recibo/vista-previa`), {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json', 'X-EDISYS': '1' },
        body: JSON.stringify({ config: form }),
      });
      if (!res.ok) {
        const cuerpo = await res.json().catch(() => ({}));
        setErrores(cuerpo?.error?.campos || {});
        throw new Error(cuerpo?.error?.mensaje || 'No se pudo armar la vista previa.');
      }
      setVistaPrevia(URL.createObjectURL(await res.blob()));
    } catch (err) {
      await dialog.alert({ title: 'Sin vista previa', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const act = p.datos.actualizado;
  return (
    <>
      {dialogEl}
      <Seccion titulo="Plantilla del recibo" extra={act?.actualizado_en && <span className="text-xs text-texto-apoyo">Última edición: {formatearFechaHora(act.actualizado_en)}{act.actualizado_por ? ` · ${act.actualizado_por}` : ''}</span>}>
        <form className="flex flex-col gap-4" onSubmit={guardar}>
          <div className="grid gap-3 sm:grid-cols-2">
            <Campo etiqueta="Título" valor={form.titulo} onCambio={cambiar('titulo')} error={errores.titulo} />
            <div className="flex flex-col gap-1">
              <CampoColor etiqueta="Color de la cabecera" valor={form.color} onCambio={cambiar('color')} error={errores.color} vacioPermitido ayuda={form.color ? undefined : `Vacío = el de la marca (${p.datos.marca?.color_primario})`} />
              <AvisoContraste color={form.color || p.datos.marca?.color_primario} />
            </div>
          </div>
          <Campo tipo="textarea" etiqueta="Nota al pie" valor={form.nota} onCambio={cambiar('nota')} error={errores.nota} ayuda="Horarios de caja, cuentas para depositar o avisos (hasta 400 caracteres)." />
          <div className="grid gap-2 sm:grid-cols-2">
            <Casilla etiqueta="Logo de la marca" ayuda={p.datos.marca?.tiene_logo ? 'En la cabecera, sobre blanco.' : 'Aún no hay logo: sale el nombre.'} marcado={form.mostrar_logo} onCambio={cambiar('mostrar_logo')} />
            {BLOQUES.map((b) => (
              <Casilla key={b.clave} etiqueta={b.etiqueta} ayuda={b.ayuda} marcado={form.bloques?.[b.clave]} onCambio={bloque(b.clave)} />
            ))}
          </div>
          <div className="flex flex-wrap justify-end gap-2">
            <Boton variante="secundario" icono="recibo" cargando={ocupado} onClick={previsualizar}>Vista previa</Boton>
            <Boton type="submit" cargando={ocupado}>Guardar plantilla</Boton>
          </div>
        </form>
      </Seccion>

      <Modal abierto={!!vistaPrevia} onCerrar={() => setVistaPrevia(null)} titulo="Vista previa del recibo" ancho="max-w-3xl" pie={<Boton variante="fantasma" onClick={() => setVistaPrevia(null)}>Cerrar</Boton>}>
        {vistaPrevia && <iframe title="Vista previa del recibo en PDF" src={vistaPrevia} className="h-[70vh] w-full rounded-control border border-borde" />}
      </Modal>
    </>
  );
}
