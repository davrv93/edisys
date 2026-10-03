import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFechaHora } from '../../lib/fechas.js';
import { formatearNumero, formatearSoles } from '../../lib/dinero.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, FranjaKPI, Icono, Insignia, MenuAcciones, Modal, Tabla, Vacio, useDialog, useToast } from '../../ui/index.js';

/**
 * Bloque F3 · Almacén. Los artículos son productos del catálogo (0017_comercio) con control de stock.
 * Entradas y salidas las registra el personal; el ajuste por conteo y el stock mínimo, la administración.
 */

const POR_PAGINA = 25;
const TIPO = {
  entrada: { texto: 'Entrada', estado: 'pagado' },
  salida: { texto: 'Salida', estado: 'pendiente' },
  ajuste: { texto: 'Ajuste', estado: 'borrador' },
};
const cant = (v) => formatearNumero(Number(v), Number(v) % 1 ? 2 : 0);
/** «2,5» o «2.5» → 2.5; vacío o inválido → null. */
const numero = (t) => {
  const v = Number(String(t ?? '').replace(',', '.'));
  return String(t ?? '').trim() === '' || Number.isNaN(v) ? null : v;
};

export default function Almacen() {
  const eid = useEid();
  const s = useSesion();
  const registra = s.tiene?.('almacen.registrar');
  const administra = s.tiene?.('almacen.administrar');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [vista, setVista] = useState('stock');
  const [filtro, setFiltro] = useState(null); // artículo del kárdex
  const [pagina, setPagina] = useState(1);
  const [mov, setMov] = useState(null); // {articulo, tipo, cantidad, costo, motivo}
  const [nuevo, setNuevo] = useState(null);
  const [controlar, setControlar] = useState(null);
  const [ocupado, setOcupado] = useState(false);

  const arts = useCarga(() => api.get(`/edificios/${eid}/almacen/articulos`), [eid]);
  const movs = useCarga(() => api.get(`/edificios/${eid}/almacen/movimientos`, { producto_id: filtro?.id || '', pagina, por_pagina: POR_PAGINA }), [eid, filtro?.id, pagina], { activo: vista === 'kardex' });
  const sinControl = useCarga(() => api.get(`/edificios/${eid}/almacen/articulos`, { todos: 1 }), [eid], { activo: !!controlar });
  const listaArts = lista(arts.datos);
  const bajos = listaArts.filter((a) => a.bajo_minimo);

  const fallo = (titulo) => (err) => dialog.alert({ icon: '⚠️', title: titulo, text: err.message });

  const guardarMovimiento = async (e) => {
    e?.preventDefault?.();
    const c = numero(mov.cantidad);
    if (c == null || c < 0 || (mov.tipo !== 'ajuste' && c === 0)) {
      await dialog.alert({ title: 'Revisa la cantidad', text: mov.tipo === 'ajuste' ? 'Escribe lo que contaste (cero o más).' : 'Escribe una cantidad mayor que cero.' });
      return;
    }
    setOcupado(true);
    try {
      const r = await api.post(`/edificios/${eid}/almacen/movimientos`, { producto_id: mov.articulo.id, tipo: mov.tipo, cantidad: c, costo_unit_cts: mov.costo || 0, motivo: mov.motivo });
      setMov(null);
      toast(`${TIPO[mov.tipo].texto} registrada. Quedan ${cant(r.saldo)}.`, { tipo: 'exito' });
      if (r.alerta) {
        await dialog.alert({ icon: '⚠️', title: 'Stock bajo el mínimo', text: `«${r.alerta.producto}» quedó en ${cant(r.alerta.stock)} (mínimo ${cant(r.alerta.stock_minimo)}). Toca reponer.` });
      }
      await Promise.all([arts.recargar(), vista === 'kardex' ? movs.recargar() : null]);
    } catch (err) {
      await fallo('No se pudo registrar')(err);
    } finally {
      setOcupado(false);
    }
  };

  const cambiarMinimo = async (a) => {
    const v = await dialog.prompt({ title: `Stock mínimo de «${a.nombre}»`, label: `Avisar cuando queden (${a.unidad})`, type: 'number', defaultValue: String(Number(a.stock_minimo)) });
    if (v == null) return;
    const n = numero(v);
    if (n == null || n < 0) {
      await dialog.alert({ title: 'Valor inválido', text: 'Escribe cero o más. Cero desactiva la alerta.' });
      return;
    }
    try {
      await api.put(`/edificios/${eid}/almacen/articulos/${a.id}`, { stock_minimo: n });
      await arts.recargar();
    } catch (err) {
      await fallo('No se pudo cambiar')(err);
    }
  };

  const guardarNuevo = async (e) => {
    e?.preventDefault?.();
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/almacen/articulos`, {
        nombre: nuevo.nombre, codigo: nuevo.codigo, unidad: nuevo.unidad, costo_cts: nuevo.costo || 0,
        stock_minimo: numero(nuevo.minimo) || 0, stock_inicial: numero(nuevo.inicial) || 0,
      });
      toast('Artículo creado.', { tipo: 'exito' });
      setNuevo(null);
      await arts.recargar();
    } catch (err) {
      await fallo('No se pudo crear')(err);
    } finally {
      setOcupado(false);
    }
  };

  const guardarControl = async (e) => {
    e?.preventDefault?.();
    if (!controlar.id) return;
    setOcupado(true);
    try {
      await api.put(`/edificios/${eid}/almacen/articulos/${controlar.id}`, { controla_stock: true, stock_minimo: numero(controlar.minimo) || 0 });
      toast('El producto ya lleva control de stock.', { tipo: 'exito' });
      setControlar(null);
      await arts.recargar();
    } catch (err) {
      await fallo('No se pudo activar')(err);
    } finally {
      setOcupado(false);
    }
  };

  const abrirMov = (a, tipo) => setMov({ articulo: a, tipo, cantidad: '', costo: tipo === 'entrada' ? a.costo_cts : 0, motivo: '' });
  const verKardex = (a) => {
    setFiltro(a);
    setPagina(1);
    setVista('kardex');
  };

  const columnas = [
    {
      clave: 'nombre', titulo: 'Artículo', movil: 'titulo',
      render: (a) => (
        <span className="min-w-0">
          <span className="block truncate font-medium text-tinta">{a.nombre}</span>
          {a.codigo && <span className="text-xs text-texto-apoyo">{a.codigo}</span>}
        </span>
      ),
    },
    { clave: 'stock', titulo: 'Stock', alinear: 'der', movil: 'valor', render: (a) => <span className={`tabular-nums ${a.bajo_minimo ? 'font-semibold text-alerta' : ''}`}>{cant(a.stock)} <span className="text-xs text-texto-apoyo">{a.unidad}</span></span> },
    { clave: 'stock_minimo', titulo: 'Mínimo', alinear: 'der', prioridad: 2, render: (a) => (Number(a.stock_minimo) > 0 ? cant(a.stock_minimo) : '—') },
    { clave: 'bajo_minimo', titulo: 'Estado', movil: 'valor2', render: (a) => (a.bajo_minimo ? <Insignia estado="vencido" texto="Bajo mínimo" /> : <Insignia estado="al_dia" texto="Suficiente" />) },
    { clave: 'costo_cts', titulo: 'Último costo', alinear: 'der', prioridad: 3, render: (a) => (a.costo_cts ? formatearSoles(a.costo_cts) : '—') },
    {
      clave: 'acciones', titulo: '', alinear: 'der', movil: 'oculto',
      render: (a) => (
        <span className="inline-flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
          {registra && <Boton tamano="sm" variante="secundario" onClick={() => abrirMov(a, 'salida')}>Salida</Boton>}
          <MenuAcciones
            etiqueta={`Acciones de ${a.nombre}`}
            items={[
              registra && { etiqueta: 'Registrar entrada', icono: 'entrante', onClick: () => abrirMov(a, 'entrada') },
              administra && { etiqueta: 'Ajustar por conteo', icono: 'ajustes', onClick: () => abrirMov(a, 'ajuste') },
              administra && { etiqueta: 'Cambiar stock mínimo', icono: 'alerta', onClick: () => cambiarMinimo(a) },
              { etiqueta: 'Ver kárdex', icono: 'ver', onClick: () => verKardex(a) },
            ].filter(Boolean)}
          />
        </span>
      ),
    },
  ];

  const columnasMov = [
    { clave: 'fecha', titulo: 'Fecha', movil: 'sub', render: (m) => formatearFechaHora(m.fecha + '-05:00') },
    { clave: 'producto', titulo: 'Artículo', movil: 'titulo' },
    { clave: 'tipo', titulo: 'Tipo', movil: 'valor2', render: (m) => <Insignia estado={TIPO[m.tipo]?.estado} texto={TIPO[m.tipo]?.texto} /> },
    { clave: 'delta', titulo: 'Cantidad', alinear: 'der', movil: 'valor', render: (m) => <span className={`tabular-nums ${Number(m.delta) < 0 ? 'text-alerta' : 'text-acento'}`}>{Number(m.delta) > 0 ? '+' : ''}{cant(m.delta)}</span> },
    { clave: 'saldo', titulo: 'Saldo', alinear: 'der', render: (m) => <span className="tabular-nums">{cant(m.saldo)}</span> },
    { clave: 'motivo', titulo: 'Motivo', prioridad: 2 },
    { clave: 'usuario', titulo: 'Registró', prioridad: 3 },
  ];

  const acciones = administra && (
    <>
      <Boton variante="secundario" onClick={() => setControlar({ id: '', minimo: '' })}>Controlar producto</Boton>
      <Boton icono="mas_signo" onClick={() => setNuevo({ nombre: '', codigo: '', unidad: 'unidad', costo: 0, minimo: '', inicial: '' })}>Nuevo artículo</Boton>
    </>
  );

  return (
    <>
      {dialogEl}
      <Encabezado titulo="Almacén" subtitulo="Insumos del edificio: stock, mínimos y movimientos" ayuda="Cada entrada, salida o ajuste queda en el kárdex con el saldo que deja. Cuando un artículo llega a su stock mínimo, aparece en alerta." acciones={acciones} />
      <Contenido>
        <FranjaKPI
          cargando={!arts.datos && !arts.error}
          principal={{ titulo: 'Bajo el mínimo', valor: String(bajos.length), tono: bajos.length ? 'alerta' : 'acento', icono: 'alerta', nota: bajos.length ? 'Toca reponer' : 'Todo con stock suficiente' }}
          items={[{ titulo: 'Artículos con control', valor: String(listaArts.length) }]}
        />

        {bajos.length > 0 && vista === 'stock' && (
          <div role="status" className="flex flex-wrap items-center gap-2 rounded-control border border-alerta-borde bg-alerta-suave p-3 text-sm text-alerta-texto">
            <Icono nombre="alerta" tam={16} />
            <span>Reponer: {bajos.map((a) => `${a.nombre} (${cant(a.stock)}/${cant(a.stock_minimo)})`).join(' · ')}</span>
          </div>
        )}

        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'stock'} icono="bandeja" onClick={() => setVista('stock')} contador={listaArts.length}>Stock</Chip>
          <Chip activo={vista === 'kardex'} icono="documento" onClick={() => setVista('kardex')}>Kárdex</Chip>
          {vista === 'kardex' && filtro && <Chip onQuitar={() => { setFiltro(null); setPagina(1); }}>{filtro.nombre}</Chip>}
        </div>

        {vista === 'stock' ? (
          <div className="overflow-hidden rounded-tarjeta border border-borde bg-superficie">
            <Tabla etiqueta="Stock del almacén" columnas={columnas} filas={listaArts} cargando={arts.cargando} error={arts.error} onReintentar={arts.recargar}
              onFila={verKardex}
              vacio={<Vacio icono="bandeja" titulo="Sin artículos en el almacén" texto={administra ? 'Crea los insumos (lejía, focos, bolsas) o activa el control de stock de un producto del catálogo.' : 'La administración aún no registra artículos.'} />} />
          </div>
        ) : (
          <div className="overflow-hidden rounded-tarjeta border border-borde bg-superficie">
            <Tabla etiqueta="Kárdex" columnas={columnasMov} filas={lista(movs.datos)} cargando={movs.cargando} error={movs.error} onReintentar={movs.recargar}
              vacio={<Vacio icono="documento" titulo="Sin movimientos" compacto />}
              paginacion={(movs.datos?.total || 0) > POR_PAGINA ? { pagina, porPagina: POR_PAGINA, total: movs.datos.total, onPagina: setPagina, unidad: 'movimientos' } : undefined} />
          </div>
        )}
      </Contenido>

      <Modal abierto={!!mov} onCerrar={() => setMov(null)} titulo={mov ? `${TIPO[mov.tipo].texto} · ${mov.articulo.nombre}` : ''} ancho="max-w-md"
        pie={<><Boton variante="fantasma" onClick={() => setMov(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarMovimiento}>Registrar</Boton></>}>
        {mov && (
          <form className="flex flex-col gap-3" onSubmit={guardarMovimiento}>
            <p className="text-sm text-texto-suave">Stock actual: <b className="tabular-nums">{cant(mov.articulo.stock)} {mov.articulo.unidad}</b></p>
            <Campo etiqueta={mov.tipo === 'ajuste' ? 'Conteo físico' : 'Cantidad'} tipo="numero" valor={mov.cantidad} onCambio={(v) => setMov({ ...mov, cantidad: v })} autoFocus />
            {mov.tipo === 'entrada' && <Campo etiqueta="Costo unitario" tipo="dinero" valor={mov.costo} onCambio={(v) => setMov({ ...mov, costo: v || 0 })} />}
            <Campo etiqueta={mov.tipo === 'entrada' ? 'Nota (opcional)' : mov.tipo === 'salida' ? 'Para qué sale' : 'Motivo del ajuste'} valor={mov.motivo} onCambio={(v) => setMov({ ...mov, motivo: v })} />
          </form>
        )}
      </Modal>

      <Modal abierto={!!nuevo} onCerrar={() => setNuevo(null)} titulo="Nuevo artículo" ancho="max-w-md"
        pie={<><Boton variante="fantasma" onClick={() => setNuevo(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarNuevo}>Crear</Boton></>}>
        {nuevo && (
          <form className="flex flex-col gap-3" onSubmit={guardarNuevo}>
            <Campo etiqueta="Nombre" valor={nuevo.nombre} onCambio={(v) => setNuevo({ ...nuevo, nombre: v })} placeholder="Lejía 1 L" />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Código" valor={nuevo.codigo} onCambio={(v) => setNuevo({ ...nuevo, codigo: v })} />
              <Campo etiqueta="Unidad" tipo="select" valor={nuevo.unidad} onCambio={(v) => setNuevo({ ...nuevo, unidad: v })} opciones={[{ valor: 'unidad', etiqueta: 'Unidad' }, { valor: 'kg', etiqueta: 'Kilo' }]} />
              <Campo etiqueta="Costo unitario" tipo="dinero" valor={nuevo.costo} onCambio={(v) => setNuevo({ ...nuevo, costo: v || 0 })} />
              <Campo etiqueta="Stock inicial" tipo="numero" valor={nuevo.inicial} onCambio={(v) => setNuevo({ ...nuevo, inicial: v })} />
              <Campo etiqueta="Stock mínimo" tipo="numero" valor={nuevo.minimo} onCambio={(v) => setNuevo({ ...nuevo, minimo: v })} ayuda="Cero: sin alerta." />
            </div>
          </form>
        )}
      </Modal>

      <Modal abierto={!!controlar} onCerrar={() => setControlar(null)} titulo="Controlar un producto del catálogo" ancho="max-w-md"
        pie={<><Boton variante="fantasma" onClick={() => setControlar(null)}>Cancelar</Boton><Boton cargando={ocupado} disabled={!controlar?.id} onClick={guardarControl}>Activar</Boton></>}>
        {controlar && (
          <form className="flex flex-col gap-3" onSubmit={guardarControl}>
            <Campo etiqueta="Producto" tipo="select" valor={controlar.id} onCambio={(v) => setControlar({ ...controlar, id: v })}
              opciones={[{ valor: '', etiqueta: 'Elige…' }, ...lista(sinControl.datos).filter((p) => !p.controla_stock).map((p) => ({ valor: String(p.id), etiqueta: p.nombre }))]} />
            <Campo etiqueta="Stock mínimo" tipo="numero" valor={controlar.minimo} onCambio={(v) => setControlar({ ...controlar, minimo: v })} />
            <p className="text-xs text-texto-apoyo">El stock que ya tenga el producto entra al kárdex como saldo inicial.</p>
          </form>
        )}
      </Modal>
    </>
  );
}
