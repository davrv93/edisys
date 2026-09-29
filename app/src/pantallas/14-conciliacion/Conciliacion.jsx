import { useState } from 'react';
import { api, subir, urlApi, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton, BotonIcono, Campo, Chip, ErrorCarga, Esqueleto, Icono, Modal, SelectorPeriodo, SubirArchivo, Vacio, useDialog, useToast } from '../../ui/index.js';

const ESTADO_MOV = {
  conciliado: { tono: 'acento', texto: 'Conciliado' },
  sugerido: { tono: 'aviso', texto: 'Por confirmar' },
  sin_pareja: { tono: 'alerta', texto: 'Sin pareja' },
};

const REGLA = { codigo: 'por código', monto_fecha: 'por monto y fecha', monto: 'por monto', manual: 'a mano', creado: 'creado desde el banco' };

/** Etiqueta de estado de un movimiento o ítem. */
export function estadoMovimiento(estado) {
  return ESTADO_MOV[estado] || ESTADO_MOV.sin_pareja;
}

/** Movimientos que faltan resolver para que la diferencia llegue a cero. */
export function pendientes(banco = []) {
  return banco.filter((m) => m.estado !== 'conciliado').length;
}

/** «ABONO TRANSFERENCIA» → «Abono transferencia»: el banco escribe en mayúsculas. */
const suave = (t = '') => (t === t.toUpperCase() ? t.charAt(0) + t.slice(1).toLowerCase() : t);

function Monto({ cts, className = '' }) {
  return <span className={`whitespace-nowrap text-sm font-semibold tabular-nums ${cts < 0 ? 'text-alerta' : 'text-tinta'} ${className}`}>{formatearSoles(cts)}</span>;
}

/** Círculo con la dirección del dinero: entra (petróleo) o sale (rojo suave). */
function Direccion({ cts }) {
  const entra = cts >= 0;
  return (
    <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-chip ${entra ? 'bg-acento-suave text-acento' : 'bg-alerta-suave text-alerta'}`} aria-hidden="true">
      <Icono nombre={entra ? 'entrante' : 'sube'} tam={16} />
    </span>
  );
}

/** Punto de estado de la fila: verde petróleo, ámbar o rojo, con su texto para lectores. */
function Punto({ estado }) {
  const color = estado === 'conciliado' ? 'bg-acento' : estado === 'sugerido' ? 'bg-aviso' : 'bg-alerta';
  return (
    <span className="inline-flex items-center gap-1.5 text-xs text-texto-apoyo">
      <span className={`h-1.5 w-1.5 rounded-chip ${color}`} aria-hidden="true" />
      {estadoMovimiento(estado).texto}
    </span>
  );
}

function FilaBanco({ m, ocupado, onAccion, onCrear }) {
  const pareja = m.pareja ? `${m.pareja}${m.regla && REGLA[m.regla] ? ` · ${REGLA[m.regla]}` : ''}` : null;
  return (
    <li className="group flex items-center gap-3 px-3 py-2.5 transition-colors duration-rapida hover:bg-fondo">
      <Direccion cts={m.monto_cts} />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-3">
          <p className="truncate text-sm font-medium leading-tight text-tinta">{suave(m.descripcion) || 'Movimiento'}</p>
          <Monto cts={m.monto_cts} />
        </div>
        <p className="mt-0.5 flex min-w-0 items-center gap-1 truncate text-xs text-texto-apoyo">
          {formatearFecha(m.fecha)}
          {pareja && (
            <>
              <span aria-hidden="true">·</span>
              <Icono nombre="doble_check" tam={12} className="shrink-0" />
              <span className="hidden truncate sm:inline">{pareja}</span>
            </>
          )}
          {!pareja && m.estado === 'sin_pareja' && <span className="text-alerta"> · sin pareja</span>}
        </p>
      </div>
      <div className="flex shrink-0 items-center justify-end gap-0.5 sm:w-[76px]">
        {m.estado === 'sugerido' && (
          <BotonIcono etiqueta="Confirmar pareja" icono="check" variante="suave" className="!text-acento" disabled={ocupado === m.id} onClick={() => onAccion(m.id, `movimientos/${m.id}/confirmar`, undefined, 'Pareja confirmada.')} />
        )}
        {m.estado !== 'sin_pareja' && (
          <BotonIcono etiqueta="Deshacer pareja" icono="cancelado" tam={16} className="opacity-60 group-hover:opacity-100" disabled={ocupado === m.id} onClick={() => onAccion(m.id, `movimientos/${m.id}/deshacer`, undefined, 'Pareja deshecha.')} />
        )}
        {m.estado === 'sin_pareja' && (
          <BotonIcono etiqueta={m.monto_cts < 0 ? 'Crear egreso' : 'Registrar ingreso'} icono="mas_signo" variante="secundario" onClick={() => onCrear(m)} />
        )}
      </div>
    </li>
  );
}

function FilaSistema({ it }) {
  const cts = it.tipo === 'pago' ? Math.abs(it.monto_cts) : -Math.abs(it.monto_cts);
  return (
    <li className="flex items-center gap-3 px-3 py-2.5 transition-colors duration-rapida hover:bg-fondo">
      <Direccion cts={cts} />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium leading-tight text-tinta">{it.descripcion}</p>
        <p className="mt-0.5 text-xs text-texto-apoyo">{formatearFecha(it.fecha)}</p>
      </div>
      <div className="flex shrink-0 flex-col items-end gap-0.5">
        <Monto cts={it.monto_cts} />
        <Punto estado={it.estado} />
      </div>
    </li>
  );
}

/** Anillo de avance: cuántos movimientos del banco ya están conciliados. */
function Anillo({ hechos, total }) {
  const r = 22;
  const c = 2 * Math.PI * r;
  const pct = total ? hechos / total : 0;
  return (
    <div className="relative h-14 w-14 shrink-0" role="img" aria-label={`${hechos} de ${total} conciliados`}>
      <svg viewBox="0 0 56 56" className="h-14 w-14 -rotate-90">
        <circle cx="28" cy="28" r={r} fill="none" strokeWidth="6" className="stroke-superficie-2" />
        <circle cx="28" cy="28" r={r} fill="none" strokeWidth="6" strokeLinecap="round" className="stroke-acento transition-[stroke-dashoffset] duration-lenta" strokeDasharray={c} strokeDashoffset={c * (1 - pct)} />
      </svg>
      <span className="absolute inset-0 flex items-center justify-center text-xs font-semibold tabular-nums text-tinta">{Math.round(pct * 100)}%</span>
    </div>
  );
}

/** 14 · Conciliación bancaria, minimalista: un resumen, tres pestañas y filas compactas. */
export default function Conciliacion() {
  const eid = useEid();
  const [periodo, setPeriodo] = usePeriodo();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const c = useCarga(() => api.get(`/edificios/${eid}/conciliacion`, { periodo }), [eid, periodo]);
  const [ocupado, setOcupado] = useState(null);
  const [subirAbierto, setSubirAbierto] = useState(false);
  const [crear, setCrear] = useState(null);
  const [vista, setVista] = useState('pendientes');

  const est = c.datos?.estado;
  const banco = c.datos?.banco || [];
  const sistema = c.datos?.sistema || [];
  const porResolver = banco.filter((m) => m.estado !== 'conciliado');
  const conciliados = banco.filter((m) => m.estado === 'conciliado');
  const cuadra = est?.diferencia_cts === 0;

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
      <Boton icono="subir" onClick={() => setSubirAbierto(true)}>
        Subir extracto
      </Boton>
    </>
  );
  const secundarias = [{ etiqueta: 'Extracto de ejemplo', icono: 'excel', href: urlApi(`/edificios/${eid}/conciliacion/extracto-demo.csv?periodo=${periodo}`) }];

  const filas = vista === 'pendientes' ? porResolver : vista === 'conciliados' ? conciliados : null;

  return (
    <>
      {dialogEl}
      <Encabezado titulo="Conciliación" acciones={acciones} secundarias={secundarias} />
      <Contenido>
        {c.error ? (
          <ErrorCarga error={c.error} onReintentar={c.recargar} />
        ) : !c.datos ? (
          <div className="mx-auto flex w-full max-w-3xl flex-col gap-3">
            <Esqueleto className="h-24 w-full" />
            <Esqueleto className="h-64 w-full" />
          </div>
        ) : !est ? (
          <Vacio icono="conciliacion" titulo="Aún no hay extracto de este mes" texto="Sube el extracto del banco en CSV o Excel. Las columnas se eligen una vez por banco.">
            <Boton icono="subir" onClick={() => setSubirAbierto(true)}>
              Subir extracto
            </Boton>
          </Vacio>
        ) : (
          <div className="mx-auto flex w-full max-w-3xl flex-col gap-4 animate-aparecer">
            {/* Resumen: una sola tarjeta. La diferencia manda; banco y sistema, en pequeño. */}
            <section className={`flex flex-wrap items-center gap-x-4 gap-y-3 rounded-tarjeta border p-4 ${cuadra ? 'border-acento-borde bg-acento-suave' : 'border-borde bg-superficie'}`} role="status">
              <Anillo hechos={conciliados.length} total={banco.length} />
              <div className="min-w-0 flex-1">
                <p className="flex items-center gap-1.5 text-xs font-medium text-texto-apoyo">
                  <Icono nombre="conciliacion" tam={14} />
                  {est.banco} · al {formatearFecha(est.fecha)}
                </p>
                {cuadra ? (
                  <p className="mt-0.5 flex items-center gap-1.5 text-lg font-semibold text-acento-hover">
                    <Icono nombre="hecho" tam={18} /> Todo cuadra con el banco
                  </p>
                ) : (
                  <p className="mt-0.5 text-lg font-semibold leading-tight text-tinta">
                    Faltan <span className="tabular-nums text-alerta">{formatearSoles(Math.abs(est.diferencia_cts))}</span>
                  </p>
                )}
                <p className="mt-0.5 text-xs tabular-nums text-texto-apoyo">
                  Banco {formatearSoles(est.saldo_banco_cts)} · Sistema {formatearSoles(est.saldo_sistema_cts)}
                </p>
              </div>
              {est.sugeridos > 0 && (
                <Boton tamano="sm" icono="doble_check" className="w-full sm:w-auto" cargando={ocupado === 'todas'} onClick={() => accion('todas', 'confirmar-sugeridos', { periodo }, 'Sugerencias confirmadas.')}>
                  Confirmar {est.sugeridos}
                </Boton>
              )}
            </section>

            <div className="flex items-center gap-2 overflow-x-auto" role="tablist" aria-label="Qué ver">
              <Chip activo={vista === 'pendientes'} onClick={() => setVista('pendientes')} tono={porResolver.length ? 'aviso' : 'acento'} contador={porResolver.length} role="tab" aria-selected={vista === 'pendientes'}>
                Por resolver
              </Chip>
              <Chip activo={vista === 'conciliados'} onClick={() => setVista('conciliados')} icono="doble_check" contador={conciliados.length} role="tab" aria-selected={vista === 'conciliados'}>
                Conciliados
              </Chip>
              <Chip activo={vista === 'sistema'} onClick={() => setVista('sistema')} icono="balance" contador={sistema.length} role="tab" aria-selected={vista === 'sistema'}>
                Sistema
              </Chip>
            </div>

            <section className="overflow-hidden rounded-tarjeta border border-borde bg-superficie">
              {filas ? (
                filas.length ? (
                  <ul className="flex flex-col divide-y divide-borde" aria-label="Movimientos del banco">
                    {filas.map((m) => <FilaBanco key={m.id} m={m} ocupado={ocupado} onAccion={accion} onCrear={setCrear} />)}
                  </ul>
                ) : (
                  <p className="flex items-center justify-center gap-2 px-4 py-10 text-sm text-texto-apoyo">
                    <Icono nombre="hecho" tam={18} className="text-acento" />
                    {vista === 'pendientes' ? 'Nada por resolver. ¡Bien!' : 'Aún no hay movimientos conciliados.'}
                  </p>
                )
              ) : (
                <ul className="flex flex-col divide-y divide-borde" aria-label="Pagos y egresos del sistema">
                  {sistema.map((it) => <FilaSistema key={`${it.tipo}-${it.id}`} it={it} />)}
                  {!sistema.length && <li className="px-4 py-10 text-center text-sm text-texto-apoyo">Sin pagos ni egresos en el mes.</li>}
                </ul>
              )}
            </section>
            {vista === 'pendientes' && porResolver.length > 0 && (
              <p className="flex items-center gap-4 px-1 text-xs text-texto-apoyo">
                <span className="inline-flex items-center gap-1"><Icono nombre="check" tam={12} /> confirmar</span>
                <span className="inline-flex items-center gap-1"><Icono nombre="cancelado" tam={12} /> deshacer</span>
                <span className="inline-flex items-center gap-1"><Icono nombre="mas_signo" tam={12} /> crear en el sistema</span>
              </p>
            )}
          </div>
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
