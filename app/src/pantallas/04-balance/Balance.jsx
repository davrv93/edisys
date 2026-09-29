import { useCallback, useEffect, useMemo, useState } from 'react';
import { api, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearPct } from '../../lib/dinero.js';
import { formatearFecha, mesDePeriodo } from '../../lib/fechas.js';
import { ruta, useQuery } from '../../lib/nav.jsx';
import { useEid, useSesion, Guarda } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton, TarjetaKPI, NodoDesplegable, SelectorPeriodo, ErrorCarga, Vacio, Esqueleto, Modal, Icono, Campo, SubirArchivo, useToast } from '../../ui/index.js';

/** Ancestros de un nodo con id legible: «egr.administracion.conserjeria» → [egr, egr.administracion, …]. */
export function ancestros(id) {
  const partes = String(id).split('.');
  return partes.map((_, i) => partes.slice(0, i + 1).join('.'));
}

/** 04 · Balance por nodos: vista ejecutiva arriba, árbol perezoso debajo, visor de documentos al lado. */
export default function Balance() {
  const eid = useEid();
  const s = useSesion();
  const [periodo, setPeriodo] = usePeriodo();
  const [q, setQuery] = useQuery();
  const { toast } = useToast();
  const resumen = useCarga(() => api.get(`/edificios/${eid}/balance`, { periodo }), [eid, periodo]);
  const [hijos, setHijos] = useState({});
  const [abiertos, setAbiertos] = useState(() => new Set(['raiz']));
  const [cargando, setCargando] = useState({});
  const [errores, setErrores] = useState({});
  const [doc, setDoc] = useState(null); // { nodo, datos, error }
  const [egresoAbierto, setEgresoAbierto] = useState(false);

  const raiz = resumen.datos?.raiz;
  const k = resumen.datos?.kpis;

  const cargarHijos = useCallback(
    async (id) => {
      if (id === 'raiz') return raiz?.hijos || [];
      setCargando((c) => ({ ...c, [id]: true }));
      setErrores((e) => ({ ...e, [id]: null }));
      try {
        const r = await api.get(`/edificios/${eid}/balance/nodos/${encodeURIComponent(id)}`, { periodo });
        const lista = Array.isArray(r) ? r : r?.hijos || r?.datos || [];
        setHijos((h) => ({ ...h, [id]: lista }));
        return lista;
      } catch (err) {
        setErrores((e) => ({ ...e, [id]: err }));
        return null;
      } finally {
        setCargando((c) => ({ ...c, [id]: false }));
      }
    },
    [eid, periodo, raiz],
  );

  // Al cambiar de periodo: raíz abierta y los dos primeros niveles; luego, lo que pida ?abrir=
  useEffect(() => {
    if (!raiz) return;
    // El API ya manda los nietos de la raíz: se aprovechan sin pedirlos otra vez.
    const iniciales0 = { raiz: raiz.hijos || [] };
    for (const h of raiz.hijos || []) if (Array.isArray(h.hijos) && h.hijos.length) iniciales0[h.id] = h.hijos;
    setHijos(iniciales0);
    const iniciales = ['raiz', ...(raiz.hijos || []).filter((h) => h.tiene_hijos).map((h) => h.id)];
    const abrir = q.get('abrir');
    const extra = abrir ? ancestros(abrir) : [];
    const todos = [...new Set([...iniciales, ...extra])];
    setAbiertos(new Set(todos));
    (async () => {
      for (const id of todos) if (id !== 'raiz' && !iniciales0[id]) await cargarHijos(id);
      if (abrir) setTimeout(() => document.querySelector(`[data-nodo="${CSS.escape(abrir)}"] [role="treeitem"]`)?.focus(), 50);
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [raiz]);

  const alternar = async (nodo) => {
    const id = nodo.id;
    const abierto = abiertos.has(id);
    setAbiertos((a) => {
      const n = new Set(a);
      if (abierto) n.delete(id);
      else n.add(id);
      return n;
    });
    if (!abierto) {
      setQuery({ abrir: id }, { reemplazar: true });
      if (!hijos[id]) await cargarHijos(id);
    }
  };

  const expandirTodo = async () => {
    const pendientes = ['raiz'];
    const nuevos = new Set(abiertos);
    const vistos = { ...hijos };
    while (pendientes.length) {
      const id = pendientes.shift();
      nuevos.add(id);
      const lista = vistos[id] || (await cargarHijos(id)) || [];
      vistos[id] = lista;
      for (const h of lista) if (h.tiene_hijos && h.tipo !== 'documento') pendientes.push(h.id);
    }
    setAbiertos(nuevos);
  };

  const abrirDocumento = async (nodo) => {
    const docId = nodo.doc_id ?? nodo.documento_id;
    if (nodo.bloqueado || nodo.documento_restringido || docId == null) {
      setDoc({ nodo, datos: null, error: null, bloqueado: true });
      return;
    }
    setDoc({ nodo, datos: null, error: null });
    try {
      const datos = await api.get(`/edificios/${eid}/balance/documentos/${encodeURIComponent(docId)}`);
      setDoc({ nodo, datos, error: null });
    } catch (error) {
      setDoc({ nodo, datos: null, error, bloqueado: error.status === 403 });
    }
  };

  // Filas visibles del árbol (orden en profundidad).
  const filas = useMemo(() => {
    if (!raiz) return [];
    const out = [{ nodo: { ...raiz, id: 'raiz', tiene_hijos: true }, nivel: 0 }];
    const recorrer = (id, nivel) => {
      if (!abiertos.has(id)) return;
      for (const h of hijos[id] || []) {
        out.push({ nodo: h, nivel });
        if (h.tipo !== 'documento') recorrer(h.id, nivel + 1);
      }
    };
    recorrer('raiz', 1);
    return out;
  }, [raiz, hijos, abiertos]);

  const migas = useMemo(() => {
    const sel = doc?.nodo?.id || q.get('abrir');
    if (!sel) return [];
    const nombres = new Map(Object.values(hijos).flat().map((n) => [n.id, n.nombre]));
    const cadena = sel.startsWith('doc.') ? [] : ancestros(sel);
    const out = cadena.map((id) => nombres.get(id)).filter(Boolean);
    if (doc?.nodo) out.push(doc.nodo.nombre);
    return out;
  }, [doc, q, hijos]);

  const acciones = (
    <>
      <SelectorPeriodo periodo={periodo} onCambio={setPeriodo} />
      <Boton variante="secundario" onClick={expandirTodo} disabled={!raiz}>
        Expandir todo
      </Boton>
      <Boton variante="secundario" icono="descargar" href={urlApi(`/edificios/${eid}/balance/${periodo}.pdf`)} target="_blank" rel="noopener">
        Descargar PDF
      </Boton>
      <Boton variante="fantasma" onClick={() => window.print()} className="hidden sm:inline-flex">
        Imprimir
      </Boton>
      <Guarda permiso="egresos.registrar">
        <Boton icono="mas_signo" onClick={() => setEgresoAbierto(true)}>
          Registrar egreso
        </Boton>
      </Guarda>
    </>
  );

  return (
    <>
      <Encabezado titulo="Balance por nodos" acciones={acciones} />
      <Contenido>
        {resumen.error ? (
          <ErrorCarga error={resumen.error} onReintentar={resumen.recargar} />
        ) : resumen.datos && (!raiz || resumen.datos.hay_datos === false) ? (
          <Vacio titulo={`Sin movimientos en ${mesDePeriodo(periodo)}`} texto="Cuando se registren pagos y egresos del periodo, aparecerán aquí con su sustento." />
        ) : (
          <>
            <div className="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
              <TarjetaKPI cargando={!k} titulo="Ingresos cobrados" valor={formatearSoles(k?.ingresos_cts)} nota="Cuenta lo cobrado, no lo emitido" />
              <TarjetaKPI cargando={!k} titulo="Egresos" valor={formatearSoles(k?.egresos_cts)} nota={k?.banco_cts != null ? `Banco: ${formatearSoles(k.banco_cts)}` : undefined} />
              <TarjetaKPI cargando={!k} tono="acento" titulo="Saldo del mes" valor={formatearSoles(k?.saldo_cts)} nota="Ingresos − egresos" />
              <TarjetaKPI
                cargando={!k}
                tono="alerta"
                titulo="Morosidad del mes"
                valor={k ? formatearPct(k.morosidad?.pct) : ''}
                nota={k ? `${formatearSoles(k.morosidad?.monto_cts)} emitido y no cobrado · histórica ${formatearPct(k.morosidad?.historica_pct ?? k.morosidad?.pct)}` : ''}
                to={s.tiene('recibos.ver') ? ruta('recibos', { periodo, estado: 'vencido' }) : undefined}
              />
            </div>
            {resumen.datos?.conciliacion && <p className={`text-sm ${resumen.datos.conciliacion.conciliado ? 'text-acento' : 'text-alerta'}`} role="status">{resumen.datos.conciliacion.texto}</p>}

            <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:gap-6">
              <section className="flex min-w-0 flex-1 flex-col gap-3">
                {migas.length > 0 && (
                  <nav aria-label="Ruta del nodo" className="flex flex-wrap items-center gap-2 text-sm text-texto-apoyo">
                    <span>{s.edificio.nombre}</span>
                    {migas.map((m, i) => (
                      <span key={i} className="flex items-center gap-2">
                        <span aria-hidden="true">›</span>
                        <span className={i === migas.length - 1 ? 'font-semibold text-tinta' : ''}>{m}</span>
                      </span>
                    ))}
                  </nav>
                )}
                <div className="overflow-hidden rounded-xl border border-borde bg-superficie">
                  <div className="hidden grid-cols-[1fr_96px_150px_72px] gap-x-3 bg-fondo py-3 pl-4 pr-4 text-xs font-semibold text-texto-apoyo sm:grid" aria-hidden="true">
                    <span>Nodo</span>
                    <span>Documentos</span>
                    <span className="text-right">Monto</span>
                    <span className="text-right">%</span>
                  </div>
                  {!raiz ? (
                    <div className="flex flex-col gap-2 p-4">
                      {Array.from({ length: 6 }, (_, i) => (
                        <Esqueleto key={i} className="h-9 w-full" />
                      ))}
                    </div>
                  ) : (
                    <div role="tree" aria-label={`Balance de ${mesDePeriodo(periodo)}`}>
                      {filas.map(({ nodo, nivel }) => (
                        <div key={nodo.id} data-nodo={nodo.id}>
                          <NodoDesplegable
                            nodo={nodo}
                            nivel={nivel}
                            raiz={nivel === 0}
                            abierto={abiertos.has(nodo.id)}
                            cargando={!!cargando[nodo.id]}
                            error={errores[nodo.id]}
                            onAlternar={() => (nivel === 0 ? null : alternar(nodo))}
                            onReintentar={() => cargarHijos(nodo.id)}
                            onDocumento={abrirDocumento}
                            seleccionado={doc?.nodo?.id === nodo.id}
                          />
                        </div>
                      ))}
                    </div>
                  )}
                </div>
                {resumen.datos?.conciliacion && (
                  <div className="flex items-start gap-2 rounded-xl border border-acento-borde bg-acento-suave p-4 text-sm text-acento-hover">
                    <Icono nombre="check" tam={18} className="mt-0.5" />
                    {resumen.datos.conciliacion}
                  </div>
                )}
                <p className="text-xs text-texto-apoyo">Toca un nodo para bajar de lo general al documento. Con teclado: flechas para moverte y abrir, Enter para ver el documento.</p>
              </section>

              {doc && (
                <aside className="hidden w-[340px] shrink-0 lg:block">
                  <VisorDocumento doc={doc} onCerrar={() => setDoc(null)} />
                </aside>
              )}
            </div>
          </>
        )}
      </Contenido>

      {/* En móvil y tableta el visor sale como hoja, sin salir de la pantalla */}
      <div className="lg:hidden">
        <Modal abierto={!!doc} onCerrar={() => setDoc(null)} titulo="Documento de sustento">
          {doc && <VisorDocumento doc={doc} sinCabecera />}
        </Modal>
      </div>

      <RegistrarEgreso
        abierto={egresoAbierto}
        onCerrar={() => setEgresoAbierto(false)}
        eid={eid}
        periodo={periodo}
        rubros={hijos.egr || []}
        onListo={(sinSustento) => {
          setEgresoAbierto(false);
          toast(sinSustento ? 'Egreso registrado. Sale marcado «sin sustento» hasta que subas el documento.' : 'Egreso registrado con su documento.', { tipo: sinSustento ? 'aviso' : 'exito' });
          resumen.recargar();
        }}
      />
    </>
  );
}

function VisorDocumento({ doc, onCerrar, sinCabecera = false }) {
  const d = doc.datos;
  const esImagen = d && (d.tipo === 'foto' || d.tipo === 'voucher' || /\.(jpe?g|png|webp)(\?|$)/i.test(d.url_firmada || ''));
  return (
    <div className={`flex flex-col gap-4 ${sinCabecera ? '' : 'rounded-xl border border-borde bg-superficie p-5'}`}>
      {!sinCabecera && (
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold">Documento de sustento</h2>
          <button type="button" onClick={onCerrar} aria-label="Cerrar" className="flex h-9 w-9 items-center justify-center rounded-lg bg-superficie-2 text-egreso hover:bg-borde">
            <Icono nombre="cerrar" tam={18} />
          </button>
        </div>
      )}
      {doc.bloqueado ? (
        <div className="flex flex-col items-center gap-2 rounded-lg bg-fondo p-6 text-center text-sm text-texto-suave">
          <Icono nombre="candado" tam={28} />
          Documento disponible para la junta.
        </div>
      ) : doc.error ? (
        <ErrorCarga error={doc.error} compacto />
      ) : !d ? (
        <Esqueleto className="h-60 w-full" />
      ) : (
        <>
          <div className="flex h-60 items-center justify-center overflow-hidden rounded-lg border border-borde bg-fondo text-sm text-texto-apoyo">
            {d.url_firmada ? (
              esImagen ? (
                <img src={d.url_firmada} alt={d.nombre} className="h-full w-full object-contain" />
              ) : (
                <iframe src={d.url_firmada} title={d.nombre} className="h-full w-full" />
              )
            ) : (
              <span className="flex flex-col items-center gap-2">
                <Icono nombre={esImagen ? 'camara' : 'documento'} tam={32} />
                Vista previa no disponible en modo demostración
              </span>
            )}
          </div>
          <dl className="grid grid-cols-[110px_1fr] gap-x-3 gap-y-2 text-sm">
            <dt className="text-texto-apoyo">Documento</dt>
            <dd className="font-semibold">{d.nombre}</dd>
            {d.emisor && (
              <>
                <dt className="text-texto-apoyo">Emisor</dt>
                <dd>{d.emisor}</dd>
              </>
            )}
            {d.ruc && (
              <>
                <dt className="text-texto-apoyo">RUC</dt>
                <dd>{d.ruc}</dd>
              </>
            )}
            <dt className="text-texto-apoyo">Fecha</dt>
            <dd>{formatearFecha(d.fecha)}</dd>
            <dt className="text-texto-apoyo">Monto</dt>
            <dd className="tabular-nums">{formatearSoles(d.monto_cts)}</dd>
            {d.origen && (
              <>
                <dt className="text-texto-apoyo">Origen</dt>
                <dd className="capitalize">{d.origen}</dd>
              </>
            )}
            {d.banco && (
              <>
                <dt className="text-texto-apoyo">Banco</dt>
                <dd className="text-acento">{d.banco}</dd>
              </>
            )}
            {d.nota && (
              <>
                <dt className="text-texto-apoyo">Nota</dt>
                <dd>{d.nota}</dd>
              </>
            )}
          </dl>
          {d.url_firmada && (
            <Boton variante="secundario" href={d.url_firmada} target="_blank" rel="noopener" icono="descargar" bloque>
              Descargar documento
            </Boton>
          )}
          <p className="text-xs text-texto-apoyo">El enlace es privado y caduca en 10 minutos.</p>
        </>
      )}
    </div>
  );
}

function RegistrarEgreso({ abierto, onCerrar, eid, periodo, rubros, onListo }) {
  const [f, setF] = useState({ rubro_id: '', concepto: '', monto_cts: null, fecha: '', documento: null });
  const [errores, setErrores] = useState({});
  const [enviando, setEnviando] = useState(false);
  useEffect(() => {
    if (abierto) {
      setF({ rubro_id: rubros[0]?.id || '', concepto: '', monto_cts: null, fecha: `${periodo}-01`, documento: null });
      setErrores({});
    }
  }, [abierto, periodo, rubros]);
  const enviar = async () => {
    const e = {};
    if (!f.rubro_id) e.rubro_id = 'Elige el rubro.';
    if (!f.concepto.trim()) e.concepto = 'Escribe el concepto.';
    if (!f.monto_cts || f.monto_cts <= 0) e.monto_cts = 'El monto debe ser mayor a cero.';
    setErrores(e);
    if (Object.keys(e).length) return;
    setEnviando(true);
    try {
      const form = new FormData();
      form.set('rubro_id', f.rubro_id);
      form.set('concepto', f.concepto.trim());
      form.set('monto_cts', String(f.monto_cts));
      form.set('fecha', f.fecha);
      if (f.documento) form.set('documento', f.documento);
      await api.form('POST', `/edificios/${eid}/egresos`, form);
      onListo(!f.documento);
    } catch (err) {
      setErrores(err.campos && Object.keys(err.campos).length ? err.campos : { general: err.message });
    } finally {
      setEnviando(false);
    }
  };
  return (
    <Modal
      abierto={abierto}
      onCerrar={onCerrar}
      titulo="Registrar egreso"
      pie={
        <>
          <Boton variante="secundario" onClick={onCerrar}>
            Cancelar
          </Boton>
          <Boton onClick={enviar} cargando={enviando}>
            Registrar egreso
          </Boton>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Campo etiqueta="Rubro" tipo="select" valor={f.rubro_id} onCambio={(v) => setF({ ...f, rubro_id: v })} opciones={rubros.length ? rubros.map((r) => ({ valor: r.id, etiqueta: r.nombre })) : [{ valor: '', etiqueta: 'Abre «Egresos» en el árbol para cargar los rubros' }]} error={errores.rubro_id} />
        <Campo etiqueta="Concepto" valor={f.concepto} onCambio={(v) => setF({ ...f, concepto: v })} placeholder="Ej. Luz de áreas comunes" error={errores.concepto} />
        <div className="grid grid-cols-2 gap-3">
          <Campo etiqueta="Monto" tipo="dinero" valor={f.monto_cts} onCambio={(v) => setF({ ...f, monto_cts: v })} error={errores.monto_cts} />
          <Campo etiqueta="Fecha" tipo="fecha" valor={f.fecha} onCambio={(v) => setF({ ...f, fecha: v })} />
        </div>
        <SubirArchivo etiqueta="Documento de sustento" archivo={f.documento} onArchivo={(a) => setF({ ...f, documento: a })} />
        {!f.documento && <p className="rounded-lg border border-aviso-borde bg-aviso-suave p-3 text-sm text-aviso-texto">Sin documento se guarda, pero sale marcado «sin sustento» en el balance.</p>}
        {errores.general && (
          <p className="text-sm text-alerta" role="alert">
            {errores.general}
          </p>
        )}
      </div>
    </Modal>
  );
}
