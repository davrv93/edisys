import { useEffect, useRef, useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { diaLima, formatearFechaHora } from '../../lib/fechas.js';
import { trazoQR } from '../../lib/operacion.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Icono, Insignia, Modal, Vacio, useDialog, useToast } from '../../ui/index.js';

const ESTADO_V = {
  autorizada: { estado: 'confirmada', texto: 'Autorizada' },
  en_curso: { estado: 'hoy', texto: 'Dentro' },
  finalizada: { estado: 'terminado', texto: 'Finalizada' },
  anulada: { estado: 'cancelada', texto: 'Anulada' },
};
const FORM = { unidad_id: '', visitante: '', documento: '', vehiculo_placa: '', motivo: '', valido_desde: '', valido_hasta: '', usos_max: '1', ingresar_ahora: false };

/** Bloque G3 · Registro de visitas e identificación QR: autorizar, mostrar el QR y validarlo en portería. */
export default function Visitas() {
  const s = useSesion();
  const porteria = s.tiene?.('visitas.validar');
  const [vista, setVista] = useState(porteria ? 'porteria' : 'visitas');
  return (
    <>
      <Encabezado
        titulo="Visitas"
        subtitulo={porteria ? 'Autorizaciones, lectura del QR y bitácora de accesos' : 'Autoriza a tus visitas y compárteles su QR'}
        ayuda="Cada visita autorizada recibe un QR con un código aleatorio que solo este sistema reconoce. En portería se escanea (o se escribe el código) y el sistema dice si puede entrar; cada lectura queda en la bitácora."
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          {porteria && <Chip activo={vista === 'porteria'} icono="camara" onClick={() => setVista('porteria')}>Portería</Chip>}
          <Chip activo={vista === 'visitas'} icono="usuario" onClick={() => setVista('visitas')}>Visitas</Chip>
          {porteria && <Chip activo={vista === 'accesos'} icono="agenda" onClick={() => setVista('accesos')}>Accesos</Chip>}
        </div>
        {vista === 'porteria' ? <Porteria /> : vista === 'accesos' ? <Accesos /> : <ListaVisitas />}
      </Contenido>
    </>
  );
}

// ---------- autorizaciones ----------

function ListaVisitas() {
  const eid = useEid();
  const s = useSesion();
  const porteria = s.tiene?.('visitas.validar');
  const autoriza = s.tiene?.('visitas.autorizar');
  const residente = s.tiene?.('portal.ver') && !porteria;
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [dia, setDia] = useState(diaLima(new Date()));
  const [form, setForm] = useState(null);
  const [qr, setQr] = useState(null); // respuesta de /qr
  const [ocupado, setOcupado] = useState(false);
  const vis = useCarga(() => api.get(`/edificios/${eid}/visitas`, { dia }), [eid, dia]);
  const unidades = useCarga(() => api.get(`/edificios/${eid}/operacion/unidades`), [eid], { activo: !residente && !!autoriza });
  const filas = lista(vis.datos);
  const listaUnidades = lista(unidades.datos);

  const verQR = async (v) => {
    try {
      setQr(await api.get(`/edificios/${eid}/visitas/${v.id}/qr`));
    } catch (err) {
      await dialog.alert({ title: 'No se pudo generar el QR', text: err.message });
    }
  };

  const guardar = async (e) => {
    e?.preventDefault?.();
    setOcupado(true);
    try {
      const cuerpo = { ...form, usos_max: parseInt(form.usos_max, 10) || 1, unidad_id: form.unidad_id ? Number(form.unidad_id) : 0 };
      const r = await api.post(`/edificios/${eid}/visitas`, cuerpo);
      toast(form.ingresar_ahora ? 'Ingreso registrado.' : 'Visita autorizada.', { tipo: 'exito' });
      setForm(null);
      await vis.recargar();
      if (!form.ingresar_ahora) await verQR({ id: r.id });
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const anular = async (v) => {
    const ok = await dialog.confirm({ title: `¿Anular la visita de ${v.visitante}?`, text: 'Su QR dejará de servir.', danger: true, okText: 'Anular' });
    if (!ok) return;
    try {
      await api.post(`/edificios/${eid}/visitas/${v.id}/anular`);
      await vis.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo anular', text: err.message });
    }
  };

  const salida = async (v) => {
    try {
      const r = await api.post(`/edificios/${eid}/visitas/${v.id}/salida`);
      toast(`Salida de ${v.visitante} registrada${r.estado === 'autorizada' ? ' (le quedan ingresos)' : ''}.`, { tipo: 'exito' });
      await vis.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar la salida', text: err.message });
    }
  };

  return (
    <>
      {dialogEl}
      <Seccion
        titulo="Visitas del día"
        extra={
          <div className="flex flex-wrap items-center gap-2">
            <Campo etiqueta="Día" ocultarEtiqueta tipo="fecha" valor={dia} onCambio={(v) => setDia(v || diaLima(new Date()))} className="w-40" />
            {autoriza && <Boton tamano="sm" icono="mas_signo" onClick={() => setForm({ ...FORM })}>Autorizar visita</Boton>}
          </div>
        }
      >
        {vis.error ? (
          <ErrorCarga error={vis.error} onReintentar={vis.recargar} />
        ) : !vis.datos ? (
          <Esqueleto className="h-40 w-full" />
        ) : filas.length === 0 ? (
          <Vacio icono="usuario" titulo="Sin visitas ese día" texto="Autoriza una visita y compártele su QR para que entre sin esperar." />
        ) : (
          <ul className="divide-y divide-borde">
            {filas.map((v) => (
              <li key={v.id} className="flex flex-col gap-1.5 py-2.5 sm:flex-row sm:items-center sm:gap-3">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium text-tinta">
                    {v.visitante} <span className="font-normal text-texto-apoyo">→ Dpto {v.unidad}</span>
                  </p>
                  <p className="truncate text-xs text-texto-apoyo">
                    {[v.documento && `Doc. ${v.documento}`, v.vehiculo_placa && `Placa ${v.vehiculo_placa}`, v.motivo, `${formatearFechaHora(v.valido_desde)} – ${formatearFechaHora(v.valido_hasta)}`, `${v.usos}/${v.usos_max} ingresos`].filter(Boolean).join(' · ')}
                  </p>
                </div>
                <Insignia estado={ESTADO_V[v.estado]?.estado} texto={ESTADO_V[v.estado]?.texto} />
                <div className="flex gap-1">
                  {v.estado === 'autorizada' && <Boton tamano="sm" variante="secundario" icono="ver" onClick={() => verQR(v)}>QR</Boton>}
                  {porteria && v.estado === 'en_curso' && <Boton tamano="sm" variante="secundario" onClick={() => salida(v)}>Salida</Boton>}
                  {autoriza && v.estado === 'autorizada' && <Boton tamano="sm" variante="fantasma" className="!text-alerta" onClick={() => anular(v)}>Anular</Boton>}
                </div>
              </li>
            ))}
          </ul>
        )}
      </Seccion>

      <Modal abierto={!!form} onCerrar={() => setForm(null)} titulo={form?.ingresar_ahora ? 'Registrar ingreso' : 'Autorizar visita'} ancho="max-w-lg" pie={<><Boton variante="fantasma" onClick={() => setForm(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardar}>{form?.ingresar_ahora ? 'Registrar ingreso' : 'Autorizar y ver QR'}</Boton></>}>
        {form && (
          <form className="flex flex-col gap-3" onSubmit={guardar}>
            {!residente && (
              <Campo etiqueta="Unidad que recibe" tipo="select" valor={form.unidad_id} onCambio={(v) => setForm({ ...form, unidad_id: v })} opciones={[{ valor: '', etiqueta: 'Elige…' }, ...listaUnidades.map((u) => ({ valor: String(u.id), etiqueta: `Dpto ${u.codigo}` }))]} />
            )}
            <Campo etiqueta="Visitante" valor={form.visitante} onCambio={(v) => setForm({ ...form, visitante: v })} />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Documento" valor={form.documento} onCambio={(v) => setForm({ ...form, documento: v })} />
              <Campo etiqueta="Placa (opcional)" valor={form.vehiculo_placa} onCambio={(v) => setForm({ ...form, vehiculo_placa: v })} />
            </div>
            <Campo etiqueta="Motivo" valor={form.motivo} onCambio={(v) => setForm({ ...form, motivo: v })} />
            {!form.ingresar_ahora && (
              <div className="grid grid-cols-3 gap-3">
                <Campo etiqueta="Desde" tipo="fecha" valor={form.valido_desde} onCambio={(v) => setForm({ ...form, valido_desde: v })} ayuda="Vacío: ahora" />
                <Campo etiqueta="Hasta" tipo="fecha" valor={form.valido_hasta} onCambio={(v) => setForm({ ...form, valido_hasta: v })} ayuda="Vacío: hoy" />
                <Campo etiqueta="Ingresos" tipo="numero" valor={form.usos_max} onCambio={(v) => setForm({ ...form, usos_max: v })} />
              </div>
            )}
            {porteria && (
              <label className="flex items-center gap-2 text-sm text-tinta">
                <input type="checkbox" checked={form.ingresar_ahora} onChange={(e) => setForm({ ...form, ingresar_ahora: e.target.checked })} />
                El visitante está en portería: registrar su ingreso ahora
              </label>
            )}
          </form>
        )}
      </Modal>

      <Modal abierto={!!qr} onCerrar={() => setQr(null)} titulo={qr ? `QR de ${qr.visita.visitante}` : ''} ancho="max-w-sm" pie={<Boton onClick={() => setQr(null)}>Listo</Boton>}>
        {qr && <TarjetaQR qr={qr} />}
      </Modal>
    </>
  );
}

/** El QR se dibuja en SVG con la matriz que arma el API: sin servicios externos. */
function TarjetaQR({ qr }) {
  const lado = qr.lado;
  const borde = 4;
  return (
    <div className="flex flex-col items-center gap-3">
      <svg viewBox={`${-borde} ${-borde} ${lado + borde * 2} ${lado + borde * 2}`} className="h-56 w-56 rounded-control border border-borde bg-white" shapeRendering="crispEdges" role="img" aria-label={`Código QR de la visita: ${qr.contenido}`}>
        <rect x={-borde} y={-borde} width={lado + borde * 2} height={lado + borde * 2} fill="#fff" />
        <path d={trazoQR(qr.matriz)} fill="#000" />
      </svg>
      <p className="font-mono text-sm tracking-wider text-tinta">{qr.visita.codigo_qr}</p>
      <p className="text-center text-xs text-texto-apoyo">
        Dpto {qr.visita.unidad} · válido del {formatearFechaHora(qr.visita.valido_desde)} al {formatearFechaHora(qr.visita.valido_hasta)} · {qr.visita.usos_max} ingreso(s).
        Haz una captura y envíasela a tu visita.
      </p>
    </div>
  );
}

// ---------- portería ----------

function Porteria() {
  const eid = useEid();
  const { dialog, dialogEl } = useDialog();
  const [codigo, setCodigo] = useState('');
  const [res, setRes] = useState(null);
  const [ocupado, setOcupado] = useState(false);
  const [camara, setCamara] = useState(false);
  const enfocar = () => document.getElementById('visita-codigo')?.focus();

  const validar = async (cod) => {
    const c = (cod ?? codigo).trim();
    if (!c) return;
    setOcupado(true);
    try {
      setRes(await api.post(`/edificios/${eid}/visitas/validar`, { codigo: c }));
      setCodigo('');
    } catch (err) {
      await dialog.alert({ title: 'No se pudo validar', text: err.message });
    } finally {
      setOcupado(false);
      enfocar();
    }
  };

  return (
    <>
      {dialogEl}
      <Seccion titulo="Validar QR">
        <form
          className="flex flex-col gap-2 sm:flex-row sm:items-end"
          onSubmit={(e) => {
            e.preventDefault();
            validar();
          }}
        >
          <Campo
            etiqueta="Código del QR"
            valor={codigo}
            onCambio={setCodigo}
            id="visita-codigo"
            autoFocus
            placeholder="Escanea con el lector o escribe el código"
            className="flex-1"
            ayuda="Los lectores USB o Bluetooth escriben el código y pulsan Enter solos."
          />
          <Boton cargando={ocupado} onClick={() => validar()}>Validar</Boton>
          {typeof window !== 'undefined' && 'BarcodeDetector' in window && (
            <Boton variante="secundario" icono="camara" onClick={() => setCamara(true)}>Cámara</Boton>
          )}
        </form>
        {res && (
          <div className={`mt-2 flex items-start gap-3 rounded-tarjeta border p-4 ${res.valido ? 'border-acento-borde bg-acento-suave' : 'border-alerta-borde bg-alerta-suave'}`} role="status" aria-live="polite">
            <Icono nombre={res.valido ? 'hecho' : 'cancelado'} tam={32} className={res.valido ? 'text-acento' : 'text-alerta'} />
            <div className="min-w-0">
              <p className={`text-lg font-semibold ${res.valido ? 'text-acento' : 'text-alerta'}`}>{res.valido ? 'Puede ingresar' : 'No puede ingresar'}</p>
              <p className="text-sm text-texto-suave">{res.mensaje}</p>
              {res.visita && (
                <p className="mt-1 text-sm text-tinta">
                  {res.visita.visitante} → Dpto {res.visita.unidad}
                  {res.visita.documento && ` · Doc. ${res.visita.documento}`}
                  {res.visita.vehiculo_placa && ` · Placa ${res.visita.vehiculo_placa}`}
                  {` · ${res.visita.usos}/${res.visita.usos_max} ingresos`}
                </p>
              )}
            </div>
          </div>
        )}
      </Seccion>
      {camara && (
        <Escaner
          onCodigo={(c) => {
            setCamara(false);
            validar(c);
          }}
          onCerrar={() => setCamara(false)}
        />
      )}
    </>
  );
}

/** Lee el QR con la cámara trasera usando BarcodeDetector del navegador (todo local). */
function Escaner({ onCodigo, onCerrar }) {
  const video = useRef(null);
  const alLeer = useRef(onCodigo);
  alLeer.current = onCodigo;
  const [error, setError] = useState(null);
  useEffect(() => {
    let flujo = null;
    let vivo = true;
    let temporizador = null;
    (async () => {
      try {
        const detector = new BarcodeDetector({ formats: ['qr_code'] });
        flujo = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' } });
        if (!vivo) return;
        video.current.srcObject = flujo;
        await video.current.play();
        const buscar = async () => {
          if (!vivo) return;
          try {
            const hallados = await detector.detect(video.current);
            if (hallados[0]?.rawValue) {
              vivo = false;
              alLeer.current(hallados[0].rawValue);
              return;
            }
          } catch {
            /* cuadro sin imagen todavía */
          }
          temporizador = setTimeout(buscar, 250);
        };
        buscar();
      } catch {
        setError('No pudimos abrir la cámara. Revisa el permiso del navegador o escribe el código.');
      }
    })();
    return () => {
      vivo = false;
      clearTimeout(temporizador);
      flujo?.getTracks().forEach((t) => t.stop());
    };
  }, []);
  return (
    <Modal abierto onCerrar={onCerrar} titulo="Escanear QR" ancho="max-w-md" pie={<Boton variante="fantasma" onClick={onCerrar}>Cerrar</Boton>}>
      {error ? <p className="text-sm text-alerta">{error}</p> : <video ref={video} className="aspect-square w-full rounded-control bg-tinta object-cover" muted playsInline />}
    </Modal>
  );
}

// ---------- bitácora ----------

function Accesos() {
  const eid = useEid();
  const acc = useCarga(() => api.get(`/edificios/${eid}/accesos`, { limite: 200 }), [eid]);
  const filas = lista(acc.datos);
  return (
    <Seccion titulo="Bitácora de accesos" extra={<Boton tamano="sm" variante="fantasma" icono="cargando" onClick={acc.recargar}>Actualizar</Boton>}>
      {acc.error ? (
        <ErrorCarga error={acc.error} onReintentar={acc.recargar} />
      ) : !acc.datos ? (
        <Esqueleto className="h-40 w-full" />
      ) : filas.length === 0 ? (
        <Vacio icono="agenda" titulo="Sin accesos" texto="Cada lectura de QR y cada salida queda aquí." />
      ) : (
        <ul className="divide-y divide-borde">
          {filas.map((a) => (
            <li key={a.id} className="flex items-center gap-3 py-2">
              <Icono nombre={a.tipo === 'entrada' ? 'entrante' : 'salir'} tam={16} className="text-texto-apoyo" />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm text-tinta">
                  {a.visitante || `Código ${a.codigo_leido}`} {a.unidad && <span className="text-texto-apoyo">→ Dpto {a.unidad}</span>}
                </p>
                <p className="truncate text-xs text-texto-apoyo">{[formatearFechaHora(a.creado_en), a.tipo === 'entrada' ? 'Entrada' : 'Salida', a.motivo_texto, a.registrado_por].filter(Boolean).join(' · ')}</p>
              </div>
              <Insignia estado={a.resultado === 'permitido' ? 'activo' : 'rechazado'} texto={a.resultado === 'permitido' ? 'Permitido' : 'Rechazado'} />
            </li>
          ))}
        </ul>
      )}
    </Seccion>
  );
}
