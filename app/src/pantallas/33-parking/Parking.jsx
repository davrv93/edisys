import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFechaHora, haceCuanto } from '../../lib/fechas.js';
import { duracion, normalizarPlaca } from '../../lib/operacion.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, FranjaKPI, Insignia, Modal, Vacio, useDialog, useToast } from '../../ui/index.js';

const TIPOS = { visitas: 'Visitas', propio: 'Propio', alquiler: 'Alquiler' };
const COBRO = { recibo: 'Al recibo', inmediato: 'Inmediato', sin_cobro: 'Sin cobro' };
const ESPACIO = { codigo: '', tipo: 'visitas', unidad_id: '', tarifa_hora_cts: 0, fraccion_min: '60', tolerancia_min: '0', activo: true };

/** Bloque G4 · Parking: espacios, entradas y salidas con cobro al recibo o inmediato. */
export default function Parking() {
  const eid = useEid();
  const s = useSesion();
  const opera = s.tiene?.('parking.operar');
  const admin = s.tiene?.('parking.administrar');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [vista, setVista] = useState('espacios');
  const [entrada, setEntrada] = useState(null); // {espacio, placa, unidad_id}
  const [salida, setSalida] = useState(null); // {cotizacion, cobro, medio}
  const [espacio, setEspacio] = useState(null); // formulario de configuración
  const [ocupado, setOcupado] = useState(false);

  const esp = useCarga(() => api.get(`/edificios/${eid}/parking/estacionamientos`), [eid]);
  const hist = useCarga(() => api.get(`/edificios/${eid}/parking/sesiones`), [eid], { activo: vista === 'historial' });
  const unidades = useCarga(() => api.get(`/edificios/${eid}/operacion/unidades`), [eid], { activo: !!(opera || admin) });
  const espacios = lista(esp.datos);
  const opcionesUnidad = [{ valor: '', etiqueta: 'Sin unidad' }, ...lista(unidades.datos).map((u) => ({ valor: String(u.id), etiqueta: `Dpto ${u.codigo}${u.residente ? ` · ${u.residente}` : ''}` }))];

  const registrarEntrada = async (e) => {
    e?.preventDefault?.();
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/parking/sesiones`, {
        estacionamiento_id: entrada.espacio.id,
        placa: normalizarPlaca(entrada.placa),
        unidad_id: entrada.unidad_id ? Number(entrada.unidad_id) : null,
      });
      toast(`Entrada de ${normalizarPlaca(entrada.placa)} en ${entrada.espacio.codigo}.`, { tipo: 'exito' });
      setEntrada(null);
      await esp.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar la entrada', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const abrirSalida = async (e) => {
    try {
      const c = await api.get(`/edificios/${eid}/parking/sesiones/${e.sesion_id}/cotizar`);
      setSalida({ cotizacion: c, cobro: c.sesion.unidad_id ? 'recibo' : 'inmediato', medio: 'efectivo' });
    } catch (err) {
      await dialog.alert({ title: 'No se pudo cotizar', text: err.message });
    }
  };

  const registrarSalida = async () => {
    setOcupado(true);
    try {
      const r = await api.post(`/edificios/${eid}/parking/sesiones/${salida.cotizacion.sesion.id}/salida`, { cobro: salida.cobro, medio: salida.medio });
      const txt = r.cobro === 'sin_cobro' ? 'sin cobro' : r.cobro === 'recibo' ? `${formatearSoles(r.monto_cts)} al próximo recibo` : `${formatearSoles(r.monto_cts)} cobrados`;
      toast(`Salida registrada: ${r.duracion}, ${txt}.`, { tipo: 'exito' });
      setSalida(null);
      await Promise.all([esp.recargar(), vista === 'historial' ? hist.recargar() : null]);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar la salida', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const guardarEspacio = async (e) => {
    e?.preventDefault?.();
    setOcupado(true);
    try {
      const cuerpo = {
        ...espacio,
        unidad_id: espacio.unidad_id ? Number(espacio.unidad_id) : null,
        tarifa_hora_cts: espacio.tarifa_hora_cts || 0,
        fraccion_min: parseInt(espacio.fraccion_min, 10) || 60,
        tolerancia_min: parseInt(espacio.tolerancia_min, 10) || 0,
      };
      if (espacio.id) await api.put(`/edificios/${eid}/parking/estacionamientos/${espacio.id}`, cuerpo);
      else await api.post(`/edificios/${eid}/parking/estacionamientos`, cuerpo);
      toast('Estacionamiento guardado.', { tipo: 'exito' });
      setEspacio(null);
      await esp.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo guardar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const cot = salida?.cotizacion;
  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Parking"
        subtitulo="Estacionamientos, entradas y salidas"
        ayuda="Toca un espacio libre para registrar la entrada y uno ocupado para la salida. El cobro sale de la tarifa del espacio (por fracción empezada, con tolerancia gratis): al recibo entra como nota de cargo en el próximo recibo de la unidad; inmediato queda como ingreso del mes."
        acciones={admin && <Boton icono="mas_signo" onClick={() => setEspacio({ ...ESPACIO })}>Nuevo espacio</Boton>}
      />
      <Contenido>
        <FranjaKPI
          cargando={!esp.datos}
          items={[
            { titulo: 'Libres', valor: String(esp.datos?.libres ?? 0) },
            { titulo: 'Ocupados', valor: String(esp.datos?.ocupados ?? 0) },
          ]}
        />
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'espacios'} icono="temporizador" onClick={() => setVista('espacios')}>Espacios</Chip>
          <Chip activo={vista === 'historial'} icono="agenda" onClick={() => setVista('historial')}>Historial</Chip>
        </div>

        {vista === 'espacios' ? (
          <Seccion titulo="Espacios">
            {esp.error ? (
              <ErrorCarga error={esp.error} onReintentar={esp.recargar} />
            ) : !esp.datos ? (
              <Esqueleto className="h-40 w-full" />
            ) : espacios.length === 0 ? (
              <Vacio icono="temporizador" titulo="Sin estacionamientos" texto={admin ? 'Crea los espacios del edificio con su tarifa por hora.' : 'La administración aún no configuró los espacios.'} />
            ) : (
              <ul className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
                {espacios.map((x) => (
                  <li key={x.id}>
                    <div className={`flex h-full flex-col gap-1 rounded-control border p-3 ${!x.activo ? 'border-borde opacity-60' : x.ocupado ? 'border-aviso-borde bg-aviso-suave' : 'border-acento-borde bg-acento-suave'}`}>
                      <div className="flex items-center justify-between gap-2">
                        <b className="text-base text-tinta">{x.codigo}</b>
                        <span className="text-xs text-texto-apoyo">{TIPOS[x.tipo]}{x.unidad ? ` · ${x.unidad}` : ''}</span>
                      </div>
                      {x.ocupado ? (
                        <p className="text-sm text-tinta">
                          <span className="font-mono font-semibold">{x.placa}</span>
                          <span className="block text-xs text-texto-apoyo">entró {haceCuanto(x.entrada_en)}</span>
                        </p>
                      ) : (
                        <p className="text-sm text-texto-suave">{x.activo ? 'Libre' : 'Desactivado'}</p>
                      )}
                      <p className="text-xs text-texto-apoyo">{x.tarifa_hora_cts ? `${formatearSoles(x.tarifa_hora_cts)}/h · fracción ${x.fraccion_min} min` : 'Sin cobro'}</p>
                      <div className="mt-auto flex flex-wrap gap-1 pt-1">
                        {opera && x.activo && !x.ocupado && <Boton tamano="sm" onClick={() => setEntrada({ espacio: x, placa: '', unidad_id: x.unidad_id ? String(x.unidad_id) : '' })}>Entrada</Boton>}
                        {opera && x.ocupado && <Boton tamano="sm" variante="secundario" onClick={() => abrirSalida(x)}>Salida</Boton>}
                        {admin && (
                          <Boton tamano="sm" variante="fantasma" onClick={() => setEspacio({ ...x, unidad_id: x.unidad_id ? String(x.unidad_id) : '', fraccion_min: String(x.fraccion_min), tolerancia_min: String(x.tolerancia_min) })}>
                            Editar
                          </Boton>
                        )}
                      </div>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        ) : (
          <Seccion titulo="Historial de sesiones" extra={hist.datos && <span className="text-sm text-texto-suave">Cobrado: {formatearSoles(hist.datos.cobrado_cts || 0)}</span>}>
            {hist.error ? (
              <ErrorCarga error={hist.error} onReintentar={hist.recargar} />
            ) : !hist.datos ? (
              <Esqueleto className="h-40 w-full" />
            ) : lista(hist.datos).length === 0 ? (
              <Vacio icono="agenda" titulo="Sin sesiones" texto="Las entradas y salidas del parking aparecen aquí." />
            ) : (
              <ul className="divide-y divide-borde">
                {lista(hist.datos).map((x) => (
                  <li key={x.id} className="flex items-center gap-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm text-tinta">
                        <span className="font-mono font-semibold">{x.placa}</span> · {x.estacionamiento}
                        {x.unidad && <span className="text-texto-apoyo"> · Dpto {x.unidad}</span>}
                      </p>
                      <p className="truncate text-xs text-texto-apoyo">
                        {[formatearFechaHora(x.entrada_en), x.salida_en ? `salió ${formatearFechaHora(x.salida_en)}` : `dentro hace ${duracion(x.minutos_actuales)}`, x.minutos != null && duracion(x.minutos)].filter(Boolean).join(' · ')}
                      </p>
                    </div>
                    {x.salida_en ? (
                      <>
                        <span className="text-sm font-semibold tabular-nums text-tinta">{formatearSoles(x.monto_cts || 0)}</span>
                        <Insignia estado={x.cobro === 'recibo' ? 'emitido' : x.cobro === 'inmediato' ? 'pagado' : 'anulado'} texto={COBRO[x.cobro]} />
                      </>
                    ) : (
                      <Insignia estado="hoy" texto="Dentro" />
                    )}
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        )}
      </Contenido>

      <Modal abierto={!!entrada} onCerrar={() => setEntrada(null)} titulo={entrada ? `Entrada en ${entrada.espacio.codigo}` : ''} ancho="max-w-sm" pie={<><Boton variante="fantasma" onClick={() => setEntrada(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={registrarEntrada}>Registrar</Boton></>}>
        {entrada && (
          <form className="flex flex-col gap-3" onSubmit={registrarEntrada}>
            <Campo etiqueta="Placa" valor={entrada.placa} onCambio={(v) => setEntrada({ ...entrada, placa: v.toUpperCase() })} autoFocus />
            <Campo etiqueta="Unidad a la que se cobra" tipo="select" valor={entrada.unidad_id} onCambio={(v) => setEntrada({ ...entrada, unidad_id: v })} opciones={opcionesUnidad} ayuda="Sin unidad, el cobro solo puede ser inmediato." />
          </form>
        )}
      </Modal>

      <Modal abierto={!!salida} onCerrar={() => setSalida(null)} titulo={cot ? `Salida de ${cot.sesion.placa}` : ''} ancho="max-w-sm" pie={<><Boton variante="fantasma" onClick={() => setSalida(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={registrarSalida}>Registrar salida</Boton></>}>
        {cot && (
          <div className="flex flex-col gap-3">
            <div className="rounded-control border border-borde bg-fondo p-3 text-center">
              <p className="text-xs text-texto-apoyo">{cot.sesion.estacionamiento} · {cot.duracion}</p>
              <p className="text-2xl font-semibold tabular-nums text-tinta">{formatearSoles(cot.monto_cts)}</p>
              {cot.monto_cts === 0 && <p className="text-xs text-texto-apoyo">Dentro de la tolerancia o sin tarifa: no se cobra.</p>}
            </div>
            {cot.monto_cts > 0 && (
              <>
                <Campo
                  etiqueta="Cobro"
                  tipo="select"
                  valor={salida.cobro}
                  onCambio={(v) => setSalida({ ...salida, cobro: v })}
                  opciones={[
                    { valor: 'recibo', etiqueta: cot.sesion.unidad ? `Al recibo del Dpto ${cot.sesion.unidad}` : 'Al recibo (necesita unidad)', deshabilitado: !cot.sesion.unidad_id },
                    { valor: 'inmediato', etiqueta: 'Inmediato' },
                  ]}
                />
                {salida.cobro === 'inmediato' && (
                  <Campo etiqueta="Medio" tipo="select" valor={salida.medio} onCambio={(v) => setSalida({ ...salida, medio: v })} opciones={[{ valor: 'efectivo', etiqueta: 'Efectivo' }, { valor: 'yape', etiqueta: 'Yape' }, { valor: 'plin', etiqueta: 'Plin' }, { valor: 'tarjeta', etiqueta: 'Tarjeta' }]} />
                )}
              </>
            )}
          </div>
        )}
      </Modal>

      <Modal abierto={!!espacio} onCerrar={() => setEspacio(null)} titulo={espacio?.id ? `Editar ${espacio.codigo}` : 'Nuevo estacionamiento'} ancho="max-w-md" pie={<><Boton variante="fantasma" onClick={() => setEspacio(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarEspacio}>Guardar</Boton></>}>
        {espacio && (
          <form className="flex flex-col gap-3" onSubmit={guardarEspacio}>
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Código" valor={espacio.codigo} onCambio={(v) => setEspacio({ ...espacio, codigo: v })} placeholder="E-03" />
              <Campo etiqueta="Tipo" tipo="select" valor={espacio.tipo} onCambio={(v) => setEspacio({ ...espacio, tipo: v })} opciones={Object.entries(TIPOS).map(([valor, etiqueta]) => ({ valor, etiqueta }))} />
            </div>
            <Campo etiqueta="Unidad dueña" tipo="select" valor={espacio.unidad_id} onCambio={(v) => setEspacio({ ...espacio, unidad_id: v })} opciones={opcionesUnidad} ayuda="Obligatoria si el espacio es propio." />
            <div className="grid grid-cols-3 gap-3">
              <Campo etiqueta="Tarifa por hora" tipo="dinero" valor={espacio.tarifa_hora_cts} onCambio={(v) => setEspacio({ ...espacio, tarifa_hora_cts: v })} />
              <Campo etiqueta="Fracción (min)" tipo="numero" valor={espacio.fraccion_min} onCambio={(v) => setEspacio({ ...espacio, fraccion_min: v })} />
              <Campo etiqueta="Tolerancia (min)" tipo="numero" valor={espacio.tolerancia_min} onCambio={(v) => setEspacio({ ...espacio, tolerancia_min: v })} />
            </div>
            {espacio.id && (
              <label className="flex items-center gap-2 text-sm text-tinta">
                <input type="checkbox" checked={!!espacio.activo} onChange={(e) => setEspacio({ ...espacio, activo: e.target.checked })} />
                Activo
              </label>
            )}
          </form>
        )}
      </Modal>
    </>
  );
}
