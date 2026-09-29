import { useCallback, useEffect, useMemo, useState } from 'react';
import { api, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearSolesCorto, formatearPct } from '../../lib/dinero.js';
import { formatearFecha, mesDePeriodo } from '../../lib/fechas.js';
import { ruta, useQuery } from '../../lib/nav.jsx';
import { useEid, useSesion, Guarda } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton, FranjaKPI, NodoDesplegable, SelectorPeriodo, ErrorCarga, Vacio, Esqueleto, Modal, Icono, Campo, SubirArchivo, useToast, BotonIcono } from '../../ui/index.js';

/** Escritorio o no, por JS: el Modal usa un portal y escapa del `lg:hidden` (duplicaría el visor lateral). */
function useEscritorio() {
  const [ancho, setAncho] = useState(() => (typeof window === 'undefined' ? 1280 : window.innerWidth));
  useEffect(() => {
    const f = () => setAncho(window.innerWidth);
    window.addEventListener('resize', f);
    return () => window.removeEventListener('resize', f);
  }, []);
  return ancho >= 1024;
}

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
  const esEscritorio = useEscritorio();
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
      // Solo se piden los nodos que existen: «?abrir=egresos» (un id que no es) no debe dar 404.
      const conocidos = new Set(['raiz', ...Object.values(iniciales0).flat().map((n) => n.id)]);
      for (const id of todos) {
        if (id === 'raiz' || iniciales0[id] || !conocidos.has(id)) continue;
        const lista = await cargarHijos(id);
        for (const n of lista || []) conocidos.add(n.id);
      }
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

  // Árbol anidado: cada rama se despliega con grid-template-rows (0fr → 1fr, 200 ms).
  // Las ramas plegadas quedan «inert» (fuera del foco y de los lectores) hasta que se abren.
  const fila = (nodo, nivel) => (
    <div data-nodo={nodo.id}>
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
  );
  const rama = (id, nivel) => {
    const lista = hijos[id];
    if (!lista) return null;
    const abierta = abiertos.has(id);
    return (
      <div role="group" className="desplegable relative" data-abierto={abierta} inert={abierta ? undefined : ''}>
        <div>
          <span aria-hidden="true" className="pointer-events-none absolute bottom-2 top-0 z-10 w-px bg-borde" style={{ left: `${22 + Math.min(nivel - 1, 5) * 20}px` }} />
          {lista.map((h) => (
            <div key={h.id}>
              {fila(h, nivel)}
              {h.tipo !== 'documento' && rama(h.id, nivel + 1)}
            </div>
          ))}
        </div>
      </div>
    );
  };

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
      <Boton variante="secundario" icono="abajo" onClick={expandirTodo} disabled={!raiz}>
        Expandir todo
      </Boton>
      <Boton variante="secundario" icono="descargar" href={urlApi(`/edificios/${eid}/balance/${periodo}.pdf`)} target="_blank" rel="noopener">
        Descargar PDF
      </Boton>
      <Guarda permiso="egresos.registrar">
        <Boton icono="mas_signo" onClick={() => setEgresoAbierto(true)}>
          Registrar egreso
        </Boton>
      </Guarda>
    </>
  );
  const secundarias = [{ etiqueta: 'Imprimir', icono: 'imprimir', onClick: () => window.print() }];

  return (
    <>
      <Encabezado titulo="Balance por nodos" acciones={acciones} secundarias={secundarias} />
      <Contenido>
        {resumen.error ? (
          <ErrorCarga error={resumen.error} onReintentar={resumen.recargar} />
        ) : resumen.datos && (!raiz || resumen.datos.hay_datos === false) ? (
          <Vacio titulo={`Sin movimientos en ${mesDePeriodo(periodo)}`} texto="Cuando se registren pagos y egresos del periodo, aparecerán aquí con su sustento." icono="balance">
            <Guarda permiso="egresos.registrar">
              <Boton icono="mas_signo" onClick={() => setEgresoAbierto(true)}>
                Registrar egreso
              </Boton>
            </Guarda>
          </Vacio>
        ) : (
          <>
            <FranjaKPI
              etiqueta={`Balance de ${mesDePeriodo(periodo)}`}
              cargando={!k}
              principal={{ titulo: 'Saldo del mes', tono: 'acento', valor: formatearSoles(k?.saldo_cts), nota: 'Ingresos − egresos' }}
              items={[
                { titulo: 'Ingresos cobrados', valor: formatearSolesCorto(k?.ingresos_cts), valorCompleto: formatearSoles(k?.ingresos_cts), nota: 'Lo cobrado, no lo emitido' },
                { titulo: 'Egresos', valor: formatearSolesCorto(k?.egresos_cts), valorCompleto: formatearSoles(k?.egresos_cts), nota: k?.banco_cts != null ? `Banco: ${formatearSolesCorto(k.banco_cts)}` : undefined },
                {
                  titulo: 'Morosidad',
                  tono: 'alerta',
                  valor: k ? formatearPct(k.morosidad?.pct) : '',
                  nota: k ? `${formatearSoles(k.morosidad?.monto_cts)} por cobrar · histórica ${formatearPct(k.morosidad?.historica_pct ?? k.morosidad?.pct)}` : '',
                  to: s.tiene('recibos.ver') ? ruta('recibos', { periodo, estado: 'vencido' }) : undefined,
                },
              ]}
            />

            <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:gap-5">
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
                <div className="overflow-hidden rounded-tarjeta border border-borde bg-superficie">
                  <div className="hidden grid-cols-[1fr_88px_140px_96px] gap-x-3 bg-fondo py-2 pl-4 pr-4 text-xs font-semibold text-texto-apoyo sm:grid" aria-hidden="true">
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
                      {fila({ ...raiz, id: 'raiz', tiene_hijos: true }, 0)}
                      {rama('raiz', 1)}
                    </div>
                  )}
                </div>
                {resumen.datos?.conciliacion && <NotaConciliacion c={resumen.datos.conciliacion} />}
                <p className="text-xs text-texto-apoyo">Toca un nodo para bajar de lo general al documento. Con teclado: flechas para moverte y abrir, Enter para ver el documento.</p>
              </section>

              {doc && (
                <aside className="sticky top-4 hidden w-[340px] shrink-0 animate-entrar-derecha lg:block">
                  <VisorDocumento doc={doc} onCerrar={() => setDoc(null)} />
                </aside>
              )}
            </div>
          </>
        )}
      </Contenido>

      {/* En móvil y tableta el visor sale como hoja, sin salir de la pantalla (en escritorio vive en el lateral) */}
      {doc && !esEscritorio && (
        <Modal abierto onCerrar={() => setDoc(null)} titulo="Documento de sustento">
          <VisorDocumento doc={doc} sinCabecera />
        </Modal>
      )}

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

/**
 * Nota de conciliación bancaria. El API la mandaba como texto; ahora puede mandar un objeto
 * { texto, conciliado, diferencia_cts, … }: se aceptan las dos formas.
 */
function NotaConciliacion({ c }) {
  const texto = typeof c === 'string' ? c : c?.texto;
  if (!texto) return null;
  const ok = typeof c === 'string' || c.conciliado !== false;
  return (
    <div className={`flex items-start gap-2 rounded-tarjeta border p-3 text-sm ${ok ? 'border-acento-borde bg-acento-suave text-acento-hover' : 'border-aviso-borde bg-aviso-suave text-aviso-texto'}`} role="status">
      <Icono nombre={ok ? 'check' : 'conciliacion'} tam={18} className="mt-0.5 shrink-0" />
      {texto}
    </div>
  );
}

function VisorDocumento({ doc, onCerrar, sinCabecera = false }) {
  const d = doc.datos;
  const esImagen = d && (d.tipo === 'foto' || d.tipo === 'voucher' || /\.(jpe?g|png|webp)(\?|$)/i.test(d.url_firmada || ''));
  return (
    <div className={`flex flex-col gap-4 ${sinCabecera ? '' : 'rounded-tarjeta border border-borde bg-superficie p-5'}`}>
      {!sinCabecera && (
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold">Documento de sustento</h2>
          <BotonIcono etiqueta="Cerrar documento" icono="cerrar" variante="suave" lado="izquierda" onClick={onCerrar} />
        </div>
      )}
      {doc.bloqueado ? (
        <div className="flex flex-col items-center gap-2 rounded-control bg-fondo p-6 text-center text-sm text-texto-suave">
          <Icono nombre="candado" tam={28} />
          Documento disponible para la junta.
        </div>
      ) : doc.error ? (
        <ErrorCarga error={doc.error} compacto />
      ) : !d ? (
        <Esqueleto className="h-60 w-full" />
      ) : (
        <>
          <div className="flex h-72 items-center justify-center overflow-hidden rounded-control border border-borde bg-fondo text-sm text-texto-apoyo sm:h-80">
            {d.url_firmada ? (
              esImagen ? (
                <a href={d.url_firmada} target="_blank" rel="noopener" title="Abrir en tamaño completo" className="h-full w-full cursor-zoom-in">
                  <img src={d.url_firmada} alt={d.nombre} className="h-full w-full object-contain" loading="lazy" />
                </a>
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
            <dd className="break-all font-semibold">{d.nombre}</dd>
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
        {!f.documento && <p className="rounded-control border border-aviso-borde bg-aviso-suave p-3 text-sm text-aviso-texto">Sin documento se guarda, pero sale marcado «sin sustento» en el balance.</p>}
        {errores.general && (
          <p className="text-sm text-alerta" role="alert">
            {errores.general}
          </p>
        )}
      </div>
    </Modal>
  );
}
