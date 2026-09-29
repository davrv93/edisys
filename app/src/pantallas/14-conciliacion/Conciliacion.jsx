import { useState } from 'react';
import { api, subir, urlApi, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, ErrorCarga, Esqueleto, Insignia, Modal, SelectorPeriodo, SubirArchivo, TarjetaKPI, Vacio, useDialog, useToast } from '../../ui/index.js';

const ESTADO_MOV = {
  conciliado: { tono: 'acento', texto: 'Conciliado' },
  sugerido: { tono: 'aviso', texto: 'Por confirmar' },
  sin_pareja: { tono: 'alerta', texto: 'Sin pareja' },
};

const REGLA = { codigo: 'por código de operación', monto_fecha: 'por monto y fecha', monto: 'solo por monto', manual: 'a mano', creado: 'creado desde el banco' };

/** Etiqueta de estado de un movimiento o ítem. */
export function estadoMovimiento(estado) {
  return ESTADO_MOV[estado] || ESTADO_MOV.sin_pareja;
}

/** Movimientos que faltan resolver para que la diferencia llegue a cero. */
export function pendientes(banco = []) {
  return banco.filter((m) => m.estado !== 'conciliado').length;
}

function Monto({ cts }) {
  return <span className={`whitespace-nowrap font-semibold tabular-nums ${cts < 0 ? 'text-alerta' : 'text-tinta'}`}>{formatearSoles(cts)}</span>;
}

/** 14 · Conciliación bancaria: el extracto del banco (izquierda) contra los pagos y egresos del sistema (derecha). */
export default function Conciliacion() {
  const eid = useEid();
  const [periodo, setPeriodo] = usePeriodo();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const c = useCarga(() => api.get(`/edificios/${eid}/conciliacion`, { periodo }), [eid, periodo]);
  const [ocupado, setOcupado] = useState(null);
  const [subirAbierto, setSubirAbierto] = useState(false);
  const [crear, setCrear] = useState(null); // movimiento sin pareja para crear egreso/ingreso

  const est = c.datos?.estado;
  const banco = c.datos?.banco || [];
  const sistema = c.datos?.sistema || [];

  const accion = async (id, ruta, cuerpo, mensaje) => {
    setOcupado(id);
    try {
      await api.post(`/edificios/${eid}/conciliacion/${ruta}`, cuerpo);
      toast(mensaje, { tipo: 'exito' });
      await c.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo completar', text: err.message });
    } finally {
      setOcupado(null);
    }
  };

  const acciones = (
    <>
      <SelectorPeriodo periodo={periodo} onCambio={setPeriodo} />
      <Boton variante="secundario" icono="descargar" href={urlApi(`/edificios/${eid}/conciliacion/extracto-demo.csv?periodo=${periodo}`)}>
        Extracto de ejemplo
      </Boton>
      <Boton icono="subir" onClick={() => setSubirAbierto(true)}>
        Subir extracto
      </Boton>
    </>
  );

  return (
    <>
      {dialogEl}
      <Encabezado titulo="Conciliación bancaria" subtitulo="El extracto del banco contra los pagos y egresos registrados" acciones={acciones} />
      <Contenido>
        {c.error ? (
          <ErrorCarga error={c.error} onReintentar={c.recargar} />
        ) : !c.datos ? (
          <div className="flex flex-col gap-3">
            <Esqueleto className="h-24 w-full" />
            <Esqueleto className="h-64 w-full" />
          </div>
        ) : !est ? (
          <Vacio titulo="Aún no hay extracto de este mes" texto="Sube el extracto del banco en CSV o Excel. Las columnas se eligen una vez por banco y quedan guardadas.">
            <Boton icono="subir" onClick={() => setSubirAbierto(true)}>
              Subir extracto
            </Boton>
          </Vacio>
        ) : (
          <>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3 lg:gap-4">
              <TarjetaKPI titulo={`Saldo del banco · ${est.banco}`} valor={formatearSoles(est.saldo_banco_cts)} nota={`Al ${formatearFecha(est.fecha)}`} />
              <TarjetaKPI titulo="Saldo del sistema" valor={formatearSoles(est.saldo_sistema_cts)} nota="Banco acumulado del balance" />
              <TarjetaKPI
                tono={est.diferencia_cts === 0 ? 'acento' : 'alerta'}
                titulo="Diferencia"
                valor={formatearSoles(est.diferencia_cts)}
                nota={`${est.conciliados} conciliados · ${est.sugeridos} por confirmar · ${est.sin_pareja} sin pareja`}
              />
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-borde bg-superficie p-4">
              <p className="text-sm" role="status">
                {est.texto}
              </p>
              {est.sugeridos > 0 && (
                <Boton
                  variante="secundario"
                  cargando={ocupado === 'todas'}
                  onClick={() => accion('todas', 'confirmar-sugeridos', { periodo }, 'Sugerencias confirmadas.')}
                >
                  Confirmar {est.sugeridos} sugeridas
                </Boton>
              )}
            </div>
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:gap-6">
              <Seccion titulo="Banco" extra={<span className="text-sm text-texto-apoyo">{banco.length} movimientos</span>} padding="p-0">
                <ul className="flex flex-col divide-y divide-borde" aria-label="Movimientos del banco">
                  {banco.map((m) => {
                    const e = estadoMovimiento(m.estado);
                    return (
                      <li key={m.id} className="flex flex-col gap-2 px-4 py-3">
                        <div className="flex items-start justify-between gap-3">
                          <span className="flex min-w-0 flex-col">
                            <span className="truncate text-sm font-semibold">{m.descripcion || 'Movimiento'}</span>
                            <span className="text-xs text-texto-apoyo">
                              {formatearFecha(m.fecha)}
                              {m.codigo_operacion ? ` · Op. ${m.codigo_operacion}` : ''}
                            </span>
                          </span>
                          <span className="flex shrink-0 flex-col items-end gap-1">
                            <Monto cts={m.monto_cts} />
                            <Insignia tono={e.tono} texto={e.texto} />
                          </span>
                        </div>
                        {m.pareja && (
                          <span className="text-xs text-texto-apoyo">
                            ↔ {m.pareja}
                            {m.regla && REGLA[m.regla] ? ` (${REGLA[m.regla]})` : ''}
                          </span>
                        )}
                        <div className="flex flex-wrap gap-2">
                          {m.estado === 'sugerido' && (
                            <Boton tamano="sm" cargando={ocupado === m.id} onClick={() => accion(m.id, `movimientos/${m.id}/confirmar`, undefined, 'Pareja confirmada.')}>
                              Confirmar
                            </Boton>
                          )}
                          {m.estado !== 'sin_pareja' && (
                            <Boton tamano="sm" variante="fantasma" disabled={ocupado === m.id} onClick={() => accion(m.id, `movimientos/${m.id}/deshacer`, undefined, 'Pareja deshecha.')}>
                              Deshacer
                            </Boton>
                          )}
                          {m.estado === 'sin_pareja' && (
                            <Boton tamano="sm" variante="secundario" onClick={() => setCrear(m)}>
                              {m.monto_cts < 0 ? 'Crear egreso' : 'Crear ingreso'}
                            </Boton>
                          )}
                        </div>
                      </li>
                    );
                  })}
                </ul>
              </Seccion>
              <Seccion titulo="Sistema" extra={<span className="text-sm text-texto-apoyo">Pagos validados y egresos del mes</span>} padding="p-0">
                <ul className="flex flex-col divide-y divide-borde" aria-label="Pagos y egresos del sistema">
                  {sistema.map((it) => {
                    const e = estadoMovimiento(it.estado);
                    return (
                      <li key={`${it.tipo}-${it.id}`} className="flex items-start justify-between gap-3 px-4 py-3">
                        <span className="flex min-w-0 flex-col">
                          <span className="truncate text-sm font-semibold">{it.descripcion}</span>
                          <span className="text-xs text-texto-apoyo">
                            {it.tipo === 'pago' ? 'Ingreso' : 'Egreso'} · {formatearFecha(it.fecha)}
                            {it.codigo_operacion ? ` · Op. ${it.codigo_operacion}` : ''}
                          </span>
                        </span>
                        <span className="flex shrink-0 flex-col items-end gap-1">
                          <Monto cts={it.monto_cts} />
                          <Insignia tono={e.tono} texto={e.texto} />
                        </span>
                      </li>
                    );
                  })}
                  {!sistema.length && <li className="px-4 py-6 text-sm text-texto-apoyo">Sin pagos ni egresos en el mes.</li>}
                </ul>
              </Seccion>
            </div>
          </>
        )}
      </Contenido>
      <SubirExtracto
        abierto={subirAbierto}
        onCerrar={() => setSubirAbierto(false)}
        eid={eid}
        periodo={periodo}
        onListo={() => {
          setSubirAbierto(false);
          toast('Extracto cargado y emparejado.', { tipo: 'exito' });
          c.recargar();
        }}
      />
      <CrearDesdeMovimiento
        eid={eid}
        movimiento={crear}
        onCerrar={() => setCrear(null)}
        onListo={(texto) => {
          setCrear(null);
          toast(texto, { tipo: 'exito' });
          c.recargar();
        }}
      />
    </>
  );
}

const COLUMNAS = [
  ['col_fecha', 'fecha', 'Fecha', true],
  ['col_descripcion', 'descripcion', 'Descripción', false],
  ['col_monto', 'monto', 'Monto con signo', false],
  ['col_cargo', 'cargo', 'Cargo (sale)', false],
  ['col_abono', 'abono', 'Abono (entra)', false],
  ['col_codigo', 'codigo_operacion', 'Código de operación', false],
  ['col_saldo', 'saldo', 'Saldo', false],
];

function SubirExtracto({ abierto, onCerrar, eid, periodo, onListo }) {
  const [archivo, setArchivo] = useState(null);
  const [banco, setBanco] = useState('BCP');
  const [cols, setCols] = useState(null); // { cabeceras, mapeo }
  const [mapeo, setMapeo] = useState({});
  const [errores, setErrores] = useState({});
  const [enviando, setEnviando] = useState(false);

  const elegir = async (f) => {
    setArchivo(f);
    setCols(null);
    setErrores({});
    if (!f) return;
    const form = new FormData();
    form.set('archivo', f);
    form.set('banco', banco);
    try {
      const r = await subir(`/edificios/${eid}/conciliacion/columnas`, form);
      setCols(r);
      const m = {};
      for (const [campo, clave] of COLUMNAS) m[campo] = r.mapeo?.[clave] || '';
      setMapeo(m);
    } catch (err) {
      setErrores({ archivo: err.message });
    }
  };

  const enviar = async () => {
    const form = new FormData();
    form.set('archivo', archivo);
    form.set('banco', banco);
    form.set('periodo', periodo);
    for (const [campo] of COLUMNAS) form.set(campo, mapeo[campo] || '');
    setEnviando(true);
    try {
      await subir(`/edificios/${eid}/conciliacion/extractos`, form);
      setArchivo(null);
      setCols(null);
      onListo();
    } catch (err) {
      setErrores({ ...err.campos, general: Object.keys(err.campos || {}).length ? '' : err.message });
    } finally {
      setEnviando(false);
    }
  };

  const opciones = [{ valor: '', etiqueta: '— No está —' }, ...(cols?.cabeceras || []).map((c) => ({ valor: c, etiqueta: c }))];
  return (
    <Modal
      abierto={abierto}
      onCerrar={onCerrar}
      titulo="Subir extracto del banco"
      ancho="max-w-lg"
      pie={
        <>
          <Boton variante="secundario" onClick={onCerrar}>
            Cancelar
          </Boton>
          <Boton disabled={!archivo || !cols} cargando={enviando} onClick={enviar}>
            Cargar y emparejar
          </Boton>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Campo etiqueta="Banco" valor={banco} onCambio={setBanco} error={errores.banco} ayuda="El mapeo de columnas se guarda por banco." />
        <SubirArchivo
          etiqueta="Extracto (CSV o Excel)"
          ayuda="CSV separado por comas o punto y coma, o XLSX; hasta 10 MB"
          aceptar=".csv,.xlsx,text/csv"
          tipos={['text/csv', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet']}
          extensiones={['.csv', '.xlsx']}
          archivo={archivo}
          onArchivo={elegir}
          error={errores.archivo}
        />
        {cols && (
          <fieldset className="flex flex-col gap-3">
            <legend className="mb-1 text-sm font-semibold">Columnas del extracto{cols.mapeo_guardado ? ' (mapeo guardado del banco)' : ''}</legend>
            {COLUMNAS.map(([campo, , etiqueta]) => (
              <Campo key={campo} etiqueta={etiqueta} tipo="select" opciones={opciones} valor={mapeo[campo]} onCambio={(v) => setMapeo((m) => ({ ...m, [campo]: v }))} error={errores[campo]} />
            ))}
            <p className="text-xs text-texto-apoyo">Usa «Monto con signo» o, si el banco los separa, «Cargo» y «Abono».</p>
          </fieldset>
        )}
        {errores.saldo_final_cts && <p className="text-sm text-alerta">{errores.saldo_final_cts}</p>}
        {errores.general && <p className="text-sm text-alerta">{errores.general}</p>}
      </div>
    </Modal>
  );
}

function CrearDesdeMovimiento({ eid, movimiento: m, onCerrar, onListo }) {
  const egreso = m && m.monto_cts < 0;
  const rubros = useCarga(() => api.get(`/edificios/${eid}/rubros`), [eid], { activo: !!egreso });
  const unidades = useCarga(() => api.get(`/edificios/${eid}/unidades`, { por_pagina: 200 }), [eid], { activo: !!m && !egreso });
  const [valor, setValor] = useState('');
  const [concepto, setConcepto] = useState('');
  const [error, setError] = useState('');
  const [enviando, setEnviando] = useState(false);

  const cerrar = () => {
    setValor('');
    setConcepto('');
    setError('');
    onCerrar();
  };

  const enviar = async () => {
    setEnviando(true);
    setError('');
    try {
      if (egreso) {
        await api.post(`/edificios/${eid}/conciliacion/movimientos/${m.id}/crear-egreso`, { rubro_id: Number(valor), concepto });
        onListo('Egreso creado y conciliado.');
      } else {
        await api.post(`/edificios/${eid}/conciliacion/movimientos/${m.id}/crear-ingreso`, { unidad_id: Number(valor) });
        onListo('Pago registrado (del cargo más antiguo) y conciliado.');
      }
      setValor('');
      setConcepto('');
    } catch (err) {
      setError(err.message);
    } finally {
      setEnviando(false);
    }
  };

  const opciones = egreso
    ? [{ valor: '', etiqueta: 'Elige el rubro' }, ...lista(rubros.datos).map((r) => ({ valor: String(r.id), etiqueta: r.nombre }))]
    : [{ valor: '', etiqueta: 'Elige la unidad que pagó' }, ...lista(unidades.datos).map((u) => ({ valor: String(u.id), etiqueta: `Dpto ${u.codigo}` }))];

  return (
    <Modal
      abierto={!!m}
      onCerrar={cerrar}
      titulo={egreso ? 'Crear egreso desde el banco' : 'Registrar ingreso desde el banco'}
      pie={
        <>
          <Boton variante="secundario" onClick={cerrar}>
            Cancelar
          </Boton>
          <Boton disabled={!valor} cargando={enviando} onClick={enviar}>
            {egreso ? 'Crear egreso' : 'Registrar pago'}
          </Boton>
        </>
      }
    >
      {m && (
        <div className="flex flex-col gap-4">
          <p className="text-sm">
            {m.descripcion} · {formatearFecha(m.fecha)} · <Monto cts={m.monto_cts} />
          </p>
          <Campo etiqueta={egreso ? 'Rubro' : 'Unidad'} tipo="select" opciones={opciones} valor={valor} onCambio={setValor} />
          {egreso && <Campo etiqueta="Concepto (opcional)" valor={concepto} onCambio={setConcepto} ayuda="Por ejemplo: Comisiones bancarias." />}
          {!egreso && <p className="text-xs text-texto-apoyo">El pago se aplica del cargo más antiguo al más nuevo de la unidad.</p>}
          {error && <p className="text-sm text-alerta">{error}</p>}
        </div>
      )}
    </Modal>
  );
}
