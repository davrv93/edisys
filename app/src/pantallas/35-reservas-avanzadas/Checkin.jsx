import { useEffect, useRef, useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { diaLima, etiquetaDia, formatearHora } from '../../lib/fechas.js';
import { ruta } from '../../lib/nav.jsx';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, ErrorCarga, Esqueleto, Icono, Vacio, useDialog, useToast } from '../../ui/index.js';

/** ¿El navegador lee QR con la cámara? (Chrome/Edge en Android y escritorio; Safari no). */
const hayLector = () => typeof window !== 'undefined' && 'BarcodeDetector' in window && !!navigator.mediaDevices?.getUserMedia;

/**
 * Bloque H1 · check-in del conserje. Escanea la entrada (QR) o teclea el código R-0000;
 * el API valida que la reserva esté confirmada, que estemos en su franja y que la unidad SIGA al día hoy.
 */
export default function Checkin() {
  const eid = useEid();
  const s = useSesion();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [codigo, setCodigo] = useState('');
  const [resultado, setResultado] = useState(null); // respuesta de GET/POST /checkin
  const [leido, setLeido] = useState(''); // lo último validado (para registrar)
  const [ocupado, setOcupado] = useState(false);
  const [camara, setCamara] = useState(false);
  const hoy = useCarga(() => api.get(`/edificios/${eid}/checkin/hoy`), [eid]);

  const validar = async (txt) => {
    const c = String(txt || '').trim();
    if (!c) return;
    setOcupado(true);
    try {
      const r = await api.get(`/edificios/${eid}/checkin`, { codigo: c });
      setResultado(r);
      setLeido(c);
    } catch (err) {
      setResultado(null);
      await dialog.alert({ title: 'No se encontró la reserva', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const registrar = async (forzarMotivo) => {
    setOcupado(true);
    try {
      const r = await api.post(`/edificios/${eid}/checkin`, { codigo: leido, forzar_motivo: forzarMotivo || undefined });
      setResultado(r);
      toast(r.valido ? `Ingreso registrado: ${r.reserva.codigo}.` : `Rechazo registrado: ${r.reserva.codigo}.`, { tipo: r.valido ? 'exito' : 'aviso' });
      setCodigo('');
      hoy.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const autorizar = async () => {
    const motivo = await dialog.prompt({
      title: 'Autorizar ingreso con deuda',
      label: 'Motivo',
      required: true,
      okText: 'Autorizar',
      text: 'La unidad no está al día. Queda registrado en la auditoría con tu nombre.',
    });
    if (motivo) registrar(motivo);
  };

  const filas = lista(hoy.datos);
  const r = resultado?.reserva;
  const yaRegistrado = r?.checkin_valido === true;

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Ingreso a áreas comunes"
        subtitulo={`Hoy · ${etiquetaDia(diaLima(new Date()))}`}
        ayuda="Escanea la entrada QR de la reserva. Se valida la franja y que la unidad siga al día el día del evento: para reservar hay que estar al día, y para entrar también."
        acciones={
          <Boton variante="secundario" icono="calendario" href={ruta('reservas')}>
            Calendario
          </Boton>
        }
      />
      <Contenido>
        <div className="flex flex-col gap-4 xl:flex-row xl:items-start">
          <div className="flex min-w-0 flex-1 flex-col gap-4">
            <Seccion titulo="Escanear entrada">
              <form
                className="flex flex-col gap-3 sm:flex-row sm:items-end"
                onSubmit={(e) => {
                  e.preventDefault();
                  validar(codigo);
                }}
              >
                <Campo className="flex-1" etiqueta="Código de la reserva o del QR" valor={codigo} onCambio={setCodigo} placeholder="R-0415 o EDISYS-R:…" autoComplete="off" />
                <div className="flex gap-2">
                  {hayLector() && (
                    <Boton type="button" variante="secundario" icono="camara" onClick={() => setCamara((v) => !v)}>
                      {camara ? 'Cerrar cámara' : 'Cámara'}
                    </Boton>
                  )}
                  <Boton type="submit" cargando={ocupado} disabled={!codigo.trim()}>
                    Validar
                  </Boton>
                </div>
              </form>
              {camara && (
                <LectorQR
                  onLeer={(txt) => {
                    setCamara(false);
                    setCodigo(txt);
                    validar(txt);
                  }}
                  onError={async (msg) => {
                    setCamara(false);
                    await dialog.alert({ title: 'No se pudo abrir la cámara', text: msg });
                  }}
                />
              )}
            </Seccion>

            {resultado && r && (
              <section
                aria-live="polite"
                className={`flex flex-col gap-3 rounded-tarjeta border p-5 ${resultado.valido ? 'border-acento-borde bg-acento-suave text-acento-hover' : 'border-alerta-borde bg-alerta-suave text-alerta-texto'}`}
              >
                <div className="flex items-center gap-3">
                  <Icono nombre={resultado.valido ? 'check' : 'alerta'} tam={28} />
                  <span className="font-titulo text-2xl font-semibold">
                    {yaRegistrado ? (resultado.forzado ? 'Ingreso autorizado' : 'Ingreso registrado') : resultado.valido ? 'Puede ingresar' : 'No puede ingresar'}
                  </span>
                </div>
                <dl className="grid grid-cols-[100px_1fr] gap-x-3 gap-y-1 text-sm text-tinta">
                  <dt className="text-texto-apoyo">Reserva</dt>
                  <dd className="font-semibold">{r.codigo}</dd>
                  <dt className="text-texto-apoyo">Unidad</dt>
                  <dd>
                    Dpto {r.unidad}
                    {r.titular ? ` · ${r.titular}` : ''}
                  </dd>
                  <dt className="text-texto-apoyo">Área</dt>
                  <dd>
                    {r.area} · {r.recurso}
                  </dd>
                  <dt className="text-texto-apoyo">Franja</dt>
                  <dd>
                    {etiquetaDia(diaLima(r.inicio))} · {formatearHora(r.inicio)}–{formatearHora(r.fin)}
                  </dd>
                  {r.titulo ? (
                    <>
                      <dt className="text-texto-apoyo">Evento</dt>
                      <dd>
                        {r.titulo}
                        {r.asistentes ? ` · ${r.asistentes} personas` : ''}
                      </dd>
                    </>
                  ) : null}
                  <dt className="text-texto-apoyo">Deuda</dt>
                  <dd className={resultado.al_dia ? '' : 'font-semibold text-alerta'}>{resultado.al_dia ? 'Al día' : `${formatearSoles(resultado.deuda_cts)} vencida`}</dd>
                </dl>
                {resultado.motivos?.length > 0 && (
                  <ul className="flex flex-col gap-1 text-sm">
                    {resultado.motivos.map((m) => (
                      <li key={m.codigo}>• {m.texto}</li>
                    ))}
                  </ul>
                )}
                {!yaRegistrado && (
                  <div className="flex flex-wrap gap-2">
                    {resultado.valido ? (
                      <Boton cargando={ocupado} icono="check" onClick={() => registrar()}>
                        Registrar ingreso
                      </Boton>
                    ) : (
                      !resultado.motivos?.some((m) => m.codigo === 'YA_INGRESO') && (
                        <Boton variante="secundario" cargando={ocupado} onClick={() => registrar()}>
                          Registrar rechazo
                        </Boton>
                      )
                    )}
                    {resultado.puede_forzar && s.tiene('reservas.administrar') && (
                      <Boton variante="secundario" onClick={autorizar} disabled={ocupado}>
                        Autorizar con deuda
                      </Boton>
                    )}
                  </div>
                )}
              </section>
            )}
          </div>

          <aside className="flex w-full flex-col xl:w-[360px] xl:shrink-0">
            <Seccion titulo="Reservas de hoy">
              {hoy.error ? (
                <ErrorCarga error={hoy.error} onReintentar={hoy.recargar} compacto />
              ) : !hoy.datos ? (
                <Esqueleto className="h-40 w-full" />
              ) : filas.length === 0 ? (
                <Vacio icono="calendario" titulo="Sin reservas hoy" texto="Cuando haya reservas para hoy aparecerán aquí." />
              ) : (
                <ul className="divide-y divide-borde">
                  {filas.map((f) => (
                    <li key={f.id}>
                      <button type="button" onClick={() => (setCodigo(f.codigo), validar(f.codigo))} className="flex w-full items-center gap-3 py-2.5 text-left hover:bg-fondo">
                        <span className="w-12 shrink-0 text-sm font-semibold tabular-nums">{formatearHora(f.inicio)}</span>
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-sm font-medium text-tinta">
                            {f.recurso} · Dpto {f.unidad}
                          </span>
                          <span className="block truncate text-xs text-texto-apoyo">
                            {f.codigo}
                            {f.titulo ? ` · ${f.titulo}` : ''}
                          </span>
                        </span>
                        <EstadoIngreso f={f} />
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </Seccion>
          </aside>
        </div>
      </Contenido>
    </>
  );
}

function EstadoIngreso({ f }) {
  const [texto, clase] =
    f.checkin_valido === true
      ? [`Ingresó ${formatearHora(f.checkin_en)}`, 'border-acento-borde bg-acento-suave text-acento-hover']
      : f.checkin_valido === false
        ? ['Rechazado', 'border-alerta-borde bg-alerta-suave text-alerta-texto']
        : f.estado === 'pendiente_pago'
          ? ['Sin pagar', 'border-aviso-borde bg-aviso-suave text-aviso-texto']
          : f.moroso
            ? ['Con deuda', 'border-alerta-borde bg-alerta-suave text-alerta-texto']
            : ['Por llegar', 'border-borde bg-superficie-2 text-texto-suave'];
  return <span className={`shrink-0 rounded-chip border px-2 py-0.5 text-xs font-semibold ${clase}`}>{texto}</span>;
}

/** Cámara trasera + BarcodeDetector nativo: sin librerías, todo local. */
function LectorQR({ onLeer, onError }) {
  const video = useRef(null);
  useEffect(() => {
    let flujo = null;
    let vivo = true;
    let t = null;
    (async () => {
      try {
        flujo = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' }, audio: false });
        if (!vivo) return;
        video.current.srcObject = flujo;
        await video.current.play();
        const detector = new window.BarcodeDetector({ formats: ['qr_code'] });
        const mirar = async () => {
          if (!vivo) return;
          try {
            const cods = await detector.detect(video.current);
            if (cods.length && cods[0].rawValue) {
              vivo = false;
              onLeer(cods[0].rawValue);
              return;
            }
          } catch {
            /* fotograma sin leer: se reintenta */
          }
          t = setTimeout(mirar, 250);
        };
        mirar();
      } catch (err) {
        if (vivo) onError(err?.message || 'Permite el acceso a la cámara o escribe el código.');
      }
    })();
    return () => {
      vivo = false;
      clearTimeout(t);
      flujo?.getTracks().forEach((tr) => tr.stop());
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return (
    <div className="mt-3 overflow-hidden rounded-tarjeta border border-borde bg-tinta">
      <video ref={video} className="mx-auto aspect-square w-full max-w-sm object-cover" muted playsInline aria-label="Vista de la cámara para leer el QR" />
    </div>
  );
}
