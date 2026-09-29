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
  const confirmar = () => onAccion(m.id, `movimientos/${m.id}/confirmar`, undefined, 'Pareja confirmada.');
  const deshacer = () => onAccion(m.id, `movimientos/${m.id}/deshacer`, undefined, 'Pareja deshecha.');
  const crear = m.monto_cts < 0 ? 'Egreso' : 'Ingreso';
  return (
    <li className="group flex items-center gap-3 px-4 py-2.5 lg:gap-4 transition-colors duration-rapida hover:bg-fondo">
      <Direccion cts={m.monto_cts} />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-3">
          <p className="truncate text-sm font-medium leading-tight text-tinta">{suave(m.descripcion) || 'Movimiento'}</p>
          <Monto cts={m.monto_cts} />
        </div>
        <p className="mt-0.5 flex min-w-0 items-center gap-1 text-xs text-texto-apoyo">
          <span className="shrink-0">{formatearFecha(m.fecha)}</span>
          {m.codigo_operacion && <span className="hidden shrink-0 xl:inline">· Op. {m.codigo_operacion}</span>}
          {pareja ? (
            <>
              <span aria-hidden="true">·</span>
              <Icono nombre="doble_check" tam={12} className={`shrink-0 ${m.estado === 'conciliado' ? 'text-acento' : 'text-aviso'}`} />
              <span className="hidden truncate sm:inline">{pareja}</span>
            </>
          ) : (
            m.estado === 'sin_pareja' && <span className="truncate text-alerta">· no está en el sistema</span>
          )}
        </p>
      </div>
      {/* Escritorio: botones con texto. Móvil: solo icono (mismo significado, con nombre accesible). */}
      <div className="hidden shrink-0 items-center justify-end gap-1.5 border-l border-borde pl-4 lg:flex lg:w-[212px]">
        {m.estado === 'sugerido' && (
          <Boton tamano="sm" icono="check" cargando={ocupado === m.id} onClick={confirmar}>
            Confirmar
          </Boton>
        )}
        {m.estado !== 'sin_pareja' && (
          <Boton tamano="sm" variante="fantasma" disabled={ocupado === m.id} onClick={deshacer}>
            Deshacer
          </Boton>
        )}
        {m.estado === 'sin_pareja' && (
          <Boton tamano="sm" variante="secundario" icono="mas_signo" onClick={() => onCrear(m)}>
            Crear {crear.toLowerCase()}
          </Boton>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-0.5 lg:hidden">
        {m.estado === 'sugerido' && <BotonIcono etiqueta="Confirmar pareja" icono="check" variante="suave" className="!text-acento" disabled={ocupado === m.id} onClick={confirmar} />}
        {m.estado !== 'sin_pareja' && <BotonIcono etiqueta="Deshacer pareja" icono="cancelado" tam={16} disabled={ocupado === m.id} onClick={deshacer} />}
        {m.estado === 'sin_pareja' && <BotonIcono etiqueta={`Crear ${crear.toLowerCase()}`} icono="mas_signo" variante="secundario" onClick={() => onCrear(m)} />}
      </div>
    </li>
  );
}

function FilaSistema({ it }) {
  const cts = it.tipo === 'pago' ? Math.abs(it.monto_cts) : -Math.abs(it.monto_cts);
  return (
    <li className="flex items-center gap-3 px-4 py-2.5 transition-colors duration-rapida hover:bg-fondo">
      <Direccion cts={cts} />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-3">
          <p className="truncate text-sm font-medium leading-tight text-tinta">{it.descripcion}</p>
          <Monto cts={it.monto_cts} />
        </div>
        <p className="mt-0.5 flex items-center justify-between gap-2 text-xs text-texto-apoyo">
          <span>
            {it.tipo === 'pago' ? 'Ingreso' : 'Egreso'} · {formatearFecha(it.fecha)}
            {it.codigo_operacion ? ` · Op. ${it.codigo_operacion}` : ''}
          </span>
          <Punto estado={it.estado} />
        </p>
      </div>
    </li>
  );
}

/** Anillo de avance: cuántos movimientos del banco ya están conciliados. */
function Anillo({ hechos, total, tam = 64 }) {
  const r = 24;
  const c = 2 * Math.PI * r;
  const pct = total ? hechos / total : 0;
  return (
    <div className="relative shrink-0" style={{ width: tam, height: tam }} role="img" aria-label={`${hechos} de ${total} conciliados`}>
      <svg viewBox="0 0 60 60" className="h-full w-full -rotate-90">
        <circle cx="30" cy="30" r={r} fill="none" strokeWidth="6" className="stroke-superficie-2" />
        <circle cx="30" cy="30" r={r} fill="none" strokeWidth="6" strokeLinecap="round" className="stroke-acento transition-[stroke-dashoffset] duration-lenta" strokeDasharray={c} strokeDashoffset={c * (1 - pct)} />
      </svg>
      <span className="absolute inset-0 flex items-center justify-center text-sm font-semibold tabular-nums text-tinta">{Math.round(pct * 100)}%</span>
    </div>
  );
}

const PASOS = [
  { icono: 'subir', titulo: 'Sube el extracto', texto: 'El CSV o Excel que descargas de tu banco.' },
  { icono: 'doble_check', titulo: 'Confirma las parejas', texto: 'EDISYS empareja cada movimiento con un pago o egreso registrado.' },
  { icono: 'mas_signo', titulo: 'Crea lo que falte', texto: 'Lo que está en el banco y no en el sistema (comisiones, depósitos sin voucher).' },
];

/** Qué es esta pantalla, en 3 pasos. Plegable: quien ya la conoce la cierra y queda cerrada. */
function ParaQueSirve() {
  const [abierto, setAbierto] = useState(() => {
    try {
      return localStorage.getItem('edisys.conciliacion.ayuda') !== 'cerrada';
    } catch {
      return true;
    }
  });
  const cambiar = (v) => {
    setAbierto(v);
    try {
      localStorage.setItem('edisys.conciliacion.ayuda', v ? 'abierta' : 'cerrada');
    } catch {
      /* sin almacenamiento: solo en esta visita */
    }
  };
  return (
    <section className="rounded-tarjeta border border-borde bg-superficie">
      <button type="button" onClick={() => cambiar(!abierto)} aria-expanded={abierto} className="flex w-full items-center gap-2 px-4 py-3 text-left">
        <Icono nombre="info" tam={16} className="text-acento" />
        <span className="flex-1 text-sm font-semibold text-tinta">¿Para qué sirve?</span>
        <Icono nombre={abierto ? 'arriba' : 'abajo'} tam={16} className="text-texto-apoyo" />
      </button>
      {abierto && (
        <div className="flex flex-col gap-3 border-t border-borde px-4 pb-4 pt-3 animate-aparecer">
          <p className="text-sm text-texto-suave">Comprueba que cada sol del balance existe de verdad en el banco. Cuando la diferencia llega a cero, el balance y su PDF dicen «Conciliado con el banco».</p>
          <ol className="flex flex-col gap-2.5">
            {PASOS.map((p, i) => (
              <li key={p.icono} className="flex gap-3">
                <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-chip bg-acento-suave text-acento">
                  <Icono nombre={p.icono} tam={14} />
                </span>
                <span className="min-w-0">
                  <span className="block text-sm font-medium text-tinta">
                    {i + 1}. {p.titulo}
                  </span>
                  <span className="block text-xs text-texto-apoyo">{p.texto}</span>
                </span>
              </li>
            ))}
          </ol>
        </div>
      )}
    </section>
  );
}

const POR_PAGINA = 12;

/** 14 · Conciliación bancaria: panel lateral con el resumen y la ayuda; a la derecha, una lista paginada. */
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
  const [texto, setTexto] = useState('');
  const [pagina, setPagina] = useState(1);

  const est = c.datos?.estado;
  const banco = c.datos?.banco || [];
  const sistema = c.datos?.sistema || [];
  const porResolver = banco.filter((m) => m.estado !== 'conciliado');
  const conciliados = banco.filter((m) => m.estado === 'conciliado');
  const sinPareja = banco.filter((m) => m.estado === 'sin_pareja').length;
  const cuadra = est?.diferencia_cts === 0;

  const base = vista === 'pendientes' ? porResolver : vista === 'conciliados' ? conciliados : vista === 'banco' ? banco : sistema;
  const q = texto.trim().toLowerCase();
  const filtradas = q ? base.filter((x) => `${x.descripcion} ${x.pareja || ''} ${x.codigo_operacion || ''} ${formatearSoles(x.monto_cts)}`.toLowerCase().includes(q)) : base;
  const paginas = Math.max(1, Math.ceil(filtradas.length / POR_PAGINA));
  const pag = Math.min(pagina, paginas);
  const visibles = filtradas.slice((pag - 1) * POR_PAGINA, pag * POR_PAGINA);
  const elegir = (v) => {
    setVista(v);
    setPagina(1);
  };

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
  const secundarias = [{ etiqueta: 'Descargar extracto de ejemplo', icono: 'excel', href: urlApi(`/edificios/${eid}/conciliacion/extracto-demo.csv?periodo=${periodo}`) }];

  const pestanas = [
    { id: 'pendientes', etiqueta: 'Por resolver', contador: porResolver.length, tono: porResolver.length ? 'aviso' : 'acento' },
    { id: 'conciliados', etiqueta: 'Conciliados', contador: conciliados.length, icono: 'doble_check' },
    { id: 'banco', etiqueta: 'Todo el banco', contador: banco.length, icono: 'conciliacion' },
    { id: 'sistema', etiqueta: 'Sistema', contador: sistema.length, icono: 'balance' },
  ];

  return (
    <>
      {dialogEl}
      <Encabezado titulo="Conciliación bancaria" subtitulo="El banco contra lo registrado en EDISYS" acciones={acciones} secundarias={secundarias} />
      <Contenido>
        {c.error ? (
          <ErrorCarga error={c.error} onReintentar={c.recargar} />
        ) : !c.datos ? (
          <div className="grid gap-4 lg:grid-cols-[300px_minmax(0,1fr)]">
            <Esqueleto className="h-56 w-full" />
            <Esqueleto className="h-96 w-full" />
          </div>
        ) : !est ? (
          <div className="mx-auto flex w-full max-w-xl flex-col gap-4">
            <ParaQueSirve />
            <Vacio icono="conciliacion" titulo="Aún no hay extracto de este mes" texto="Sube el extracto del banco en CSV o Excel. Las columnas se eligen una vez por banco.">
              <Boton icono="subir" onClick={() => setSubirAbierto(true)}>
                Subir extracto
              </Boton>
            </Vacio>
          </div>
        ) : (
          <div className="grid items-start gap-4 lg:grid-cols-[300px_minmax(0,1fr)] lg:gap-5">
            {/* Panel lateral: resumen, confirmación masiva y ayuda. Fijo al hacer scroll en escritorio. */}
            <aside className="flex flex-col gap-4 lg:sticky lg:top-20">
              <section className={`flex flex-col gap-4 rounded-tarjeta border p-4 ${cuadra ? 'border-acento-borde bg-acento-suave' : 'border-borde bg-superficie'}`} role="status">
                <div className="flex items-center gap-4">
                  <Anillo hechos={conciliados.length} total={banco.length} />
                  <div className="min-w-0">
                    <p className="flex items-center gap-1.5 text-xs font-medium text-texto-apoyo">
                      <Icono nombre="conciliacion" tam={14} />
                      {est.banco} · al {formatearFecha(est.fecha)}
                    </p>
                    {cuadra ? (
                      <p className="mt-1 flex items-center gap-1.5 text-base font-semibold text-acento-hover">
                        <Icono nombre="hecho" tam={18} /> Todo cuadra
                      </p>
                    ) : (
                      <>
                        <p className="mt-1 text-xs text-texto-apoyo">Diferencia</p>
                        <p className="text-xl font-semibold leading-tight tabular-nums text-alerta">{formatearSoles(Math.abs(est.diferencia_cts))}</p>
                      </>
                    )}
                  </div>
                </div>
                <dl className="flex flex-col gap-1.5 border-t border-borde pt-3 text-sm">
                  <div className="flex justify-between gap-2"><dt className="text-texto-apoyo">Saldo del banco</dt><dd className="font-medium tabular-nums">{formatearSoles(est.saldo_banco_cts)}</dd></div>
                  <div className="flex justify-between gap-2"><dt className="text-texto-apoyo">Saldo del sistema</dt><dd className="font-medium tabular-nums">{formatearSoles(est.saldo_sistema_cts)}</dd></div>
                  <div className="flex justify-between gap-2"><dt className="flex items-center gap-1.5 text-texto-apoyo"><span className="h-1.5 w-1.5 rounded-chip bg-aviso" />Por confirmar</dt><dd className="font-medium tabular-nums">{est.sugeridos}</dd></div>
                  <div className="flex justify-between gap-2"><dt className="flex items-center gap-1.5 text-texto-apoyo"><span className="h-1.5 w-1.5 rounded-chip bg-alerta" />Sin pareja</dt><dd className="font-medium tabular-nums">{sinPareja}</dd></div>
                </dl>
                {est.sugeridos > 0 && (
                  <Boton bloque icono="doble_check" cargando={ocupado === 'todas'} onClick={() => accion('todas', 'confirmar-sugeridos', { periodo }, 'Sugerencias confirmadas.')}>
                    Confirmar {est.sugeridos} sugeridas
                  </Boton>
                )}
              </section>
              <ParaQueSirve />
            </aside>

            <section className="flex min-w-0 flex-col gap-3">
              <div className="flex flex-col gap-2 xl:flex-row xl:items-center xl:justify-between">
                <div className="flex items-center gap-2 overflow-x-auto" role="tablist" aria-label="Qué ver">
                  {pestanas.map((t) => (
                    <Chip key={t.id} activo={vista === t.id} onClick={() => elegir(t.id)} tono={t.tono} icono={t.icono} contador={t.contador} role="tab" aria-selected={vista === t.id}>
                      {t.etiqueta}
                    </Chip>
                  ))}
                </div>
                <label className="relative block xl:w-64">
                  <span className="sr-only">Buscar movimiento</span>
                  <Icono nombre="buscar" tam={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-texto-apoyo" />
                  <input
                    type="search"
                    value={texto}
                    onChange={(e) => {
                      setTexto(e.target.value);
                      setPagina(1);
                    }}
                    placeholder="Buscar descripción, monto u operación"
                    className="h-11 w-full rounded-control border border-borde-fuerte bg-superficie pl-9 pr-3 text-base focus:outline-none focus:ring-2 focus:ring-acento lg:h-9 lg:text-sm"
                  />
                </label>
              </div>

              <div className="overflow-hidden rounded-tarjeta border border-borde bg-superficie">
                {visibles.length ? (
                  <ul className="flex flex-col divide-y divide-borde" aria-label={vista === 'sistema' ? 'Pagos y egresos del sistema' : 'Movimientos del banco'}>
                    {vista === 'sistema'
                      ? visibles.map((it) => <FilaSistema key={`${it.tipo}-${it.id}`} it={it} />)
                      : visibles.map((m) => <FilaBanco key={m.id} m={m} ocupado={ocupado} onAccion={accion} onCrear={setCrear} />)}
                  </ul>
                ) : (
                  <p className="flex items-center justify-center gap-2 px-4 py-12 text-sm text-texto-apoyo">
                    <Icono nombre={q ? 'buscar' : 'hecho'} tam={18} className="text-acento" />
                    {q ? 'Nada coincide con la búsqueda.' : vista === 'pendientes' ? 'Nada por resolver. ¡Bien!' : 'Sin movimientos aquí.'}
                  </p>
                )}
                {filtradas.length > POR_PAGINA && (
                  <div className="flex items-center justify-between gap-2 border-t border-borde px-4 py-2 text-xs text-texto-apoyo">
                    <span className="tabular-nums">
                      {(pag - 1) * POR_PAGINA + 1}–{Math.min(pag * POR_PAGINA, filtradas.length)} de {filtradas.length}
                    </span>
                    <span className="flex items-center gap-1">
                      <BotonIcono etiqueta="Página anterior" icono="izq" disabled={pag <= 1} onClick={() => setPagina(pag - 1)} />
                      <span className="tabular-nums">
                        {pag} / {paginas}
                      </span>
                      <BotonIcono etiqueta="Página siguiente" icono="der" disabled={pag >= paginas} onClick={() => setPagina(pag + 1)} />
                    </span>
                  </div>
                )}
              </div>
            </section>
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
