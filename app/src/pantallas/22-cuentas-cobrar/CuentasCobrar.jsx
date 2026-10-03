import { useEffect, useMemo, useState } from 'react';
import { api, lista, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Tabla, TarjetaKPI, Vacio } from '../../ui/index.js';

// Tramos de antigüedad de la deuda (días desde el vencimiento del recibo).
const TRAMOS = [
  { clave: 'por_vencer_cts', titulo: 'Por vencer' },
  { clave: 'd1_30_cts', titulo: '1–30 días' },
  { clave: 'd31_60_cts', titulo: '31–60 días' },
  { clave: 'd61_90_cts', titulo: '61–90 días' },
  { clave: 'd90_mas_cts', titulo: '+90 días' },
];

/** Bloque B3 · Cuentas por cobrar (staff) y estado de cuenta de la unidad (staff y propietario). */
export default function CuentasCobrar() {
  const s = useSesion();
  const staff = s.tiene('morosidad.ver');
  const [vista, setVista] = useState(() => (staff ? 'cxc' : 'estado'));
  const [unidad, setUnidad] = useState(null); // unidad elegida para el estado de cuenta

  const verEstado = (u) => {
    setUnidad(u);
    setVista('estado');
  };

  return (
    <>
      <Encabezado
        titulo="Cuentas por cobrar"
        subtitulo={staff ? 'Deuda por unidad, antigüedad y estado de cuenta' : 'El estado de cuenta de tu unidad'}
        ayuda="La antigüedad cuenta desde el vencimiento del recibo. La deuda vencida es la de la morosidad: aplica los días de gracia y, si hay un acuerdo de pago activo, solo cuenta sus cuotas vencidas."
      />
      <Contenido>
        {staff && (
          <div className="flex flex-wrap items-center gap-2">
            <Chip activo={vista === 'cxc'} icono="balance" onClick={() => setVista('cxc')}>Cuentas por cobrar</Chip>
            <Chip activo={vista === 'estado'} icono="recibo" onClick={() => setVista('estado')}>Estado de cuenta</Chip>
          </div>
        )}
        {vista === 'cxc' ? <VistaCxC onVer={verEstado} /> : <VistaEstado unidad={unidad} onUnidad={setUnidad} staff={staff} />}
      </Contenido>
    </>
  );
}

function VistaCxC({ onVer }) {
  const eid = useEid();
  const cxc = useCarga(() => api.get(`/edificios/${eid}/cuentas-por-cobrar`), [eid]);
  const filas = lista(cxc.datos);
  const tot = cxc.datos?.totales || {};

  const columnas = [
    { clave: 'unidad', titulo: 'Unidad', movil: 'titulo', render: (f) => `Dpto ${f.unidad}` },
    { clave: 'propietario', titulo: 'Propietario', movil: 'sub', prioridad: 2 },
    { clave: 'deuda_cts', titulo: 'Deuda', alinear: 'der', movil: 'valor', render: (f) => formatearSoles(f.deuda_cts) },
    ...TRAMOS.map((t) => ({ clave: t.clave, titulo: t.titulo, alinear: 'der', prioridad: 3, render: (f) => (f[t.clave] ? formatearSoles(f[t.clave]) : '—') })),
    { clave: 'deuda_vencida_cts', titulo: 'Vencida', alinear: 'der', render: (f) => formatearSoles(f.deuda_vencida_cts) },
    {
      clave: 'estado', titulo: 'Estado', movil: 'valor2',
      render: (f) => (f.acuerdo ? <Insignia estado="parcial" tono="curso" texto={`Acuerdo ${f.acuerdo}`} /> : f.moroso ? <Insignia estado="moroso" /> : <Insignia estado="pendiente" texto="En plazo" />),
    },
  ];

  return (
    <>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <TarjetaKPI titulo="Deuda total" valor={formatearSoles(tot.deuda_cts || 0)} icono="balance" cargando={!cxc.datos} />
        <TarjetaKPI titulo="Deuda vencida" valor={formatearSoles(tot.deuda_vencida_cts || 0)} tono="alerta" icono="moroso" cargando={!cxc.datos} />
        <TarjetaKPI titulo="Por vencer" valor={formatearSoles(tot.por_vencer_cts || 0)} tono="aviso" icono="reloj" cargando={!cxc.datos} />
        <TarjetaKPI titulo="Más de 90 días" valor={formatearSoles(tot.d90_mas_cts || 0)} tono="alerta" icono="critico" cargando={!cxc.datos} />
      </div>
      <Seccion
        titulo="Deuda por unidad"
        padding="p-0"
        extra={<Boton tamano="sm" variante="fantasma" icono="excel" href={urlApi(`/edificios/${eid}/cuentas-por-cobrar?formato=csv`)}>Exportar CSV</Boton>}
      >
        <Tabla
          etiqueta="Cuentas por cobrar"
          columnas={columnas}
          filas={filas}
          claveFila="unidad_id"
          cargando={!cxc.datos}
          error={cxc.error}
          onReintentar={cxc.recargar}
          onFila={(f) => onVer({ id: f.unidad_id, codigo: f.unidad })}
          vacio={<Vacio icono="pagado" titulo="Nadie debe" texto="Todas las unidades están al día." />}
        />
      </Seccion>
    </>
  );
}

function VistaEstado({ unidad, onUnidad, staff }) {
  const eid = useEid();
  const s = useSesion();
  const [desde, setDesde] = useState('');
  const [hasta, setHasta] = useState('');
  // El staff elige cualquier unidad; el propietario, solo las suyas (las trae la sesión).
  const unidades = useCarga(() => (staff ? api.get(`/edificios/${eid}/unidades`, { por_pagina: 500 }) : Promise.resolve({ datos: s.unidades || [] })), [eid, staff]);
  const opciones = useMemo(() => lista(unidades.datos).map((u) => ({ valor: String(u.id), etiqueta: `Dpto ${u.codigo}` })), [unidades.datos]);
  useEffect(() => {
    if (!unidad && opciones.length) onUnidad({ id: Number(opciones[0].valor), codigo: opciones[0].etiqueta.replace('Dpto ', '') });
  }, [unidad, opciones, onUnidad]);

  const ec = useCarga(
    () => (unidad ? api.get(`/edificios/${eid}/unidades/${unidad.id}/estado-cuenta`, { desde, hasta }) : Promise.resolve(null)),
    [eid, unidad?.id, desde, hasta],
  );
  const d = ec.datos;
  const columnas = [
    { clave: 'fecha', titulo: 'Fecha', movil: 'sub', render: (m) => formatearFecha(m.fecha) },
    { clave: 'concepto', titulo: 'Concepto', movil: 'titulo' },
    { clave: 'referencia', titulo: 'Referencia', prioridad: 2 },
    { clave: 'cargo_cts', titulo: 'Cargo', alinear: 'der', render: (m) => (m.cargo_cts ? formatearSoles(m.cargo_cts) : '') },
    { clave: 'abono_cts', titulo: 'Abono', alinear: 'der', render: (m) => (m.abono_cts ? formatearSoles(m.abono_cts) : '') },
    { clave: 'saldo_cts', titulo: 'Saldo', alinear: 'der', movil: 'valor', render: (m) => formatearSoles(m.saldo_cts) },
  ];
  const csv = unidad && urlApi(`/edificios/${eid}/unidades/${unidad.id}/estado-cuenta?formato=csv&desde=${desde}&hasta=${hasta}`);

  return (
    <>
      <div className="flex flex-wrap items-end gap-3">
        <Campo
          etiqueta="Unidad"
          tipo="select"
          className="w-40"
          valor={unidad ? String(unidad.id) : ''}
          onCambio={(v) => {
            const o = opciones.find((x) => x.valor === v);
            onUnidad({ id: Number(v), codigo: o ? o.etiqueta.replace('Dpto ', '') : '' });
          }}
          opciones={opciones}
        />
        <Campo etiqueta="Desde" tipo="fecha" className="w-44" valor={desde} onCambio={setDesde} />
        <Campo etiqueta="Hasta" tipo="fecha" className="w-44" valor={hasta} onCambio={setHasta} />
        {csv && <Boton variante="fantasma" icono="excel" href={csv}>Exportar CSV</Boton>}
        <Boton variante="fantasma" icono="imprimir" onClick={() => window.print()}>Imprimir</Boton>
      </div>

      {unidades.error ? (
        <ErrorCarga error={unidades.error} onReintentar={unidades.recargar} />
      ) : !opciones.length && unidades.datos ? (
        <Vacio icono="edificio" titulo="Sin unidades" texto="No tienes unidades asociadas en este edificio." />
      ) : !d ? (
        ec.error ? <ErrorCarga error={ec.error} onReintentar={ec.recargar} /> : <Esqueleto className="h-40 w-full" />
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <TarjetaKPI titulo="Saldo inicial" valor={formatearSoles(d.saldo_inicial_cts)} icono="recibo" />
            <TarjetaKPI titulo="Cargos" valor={formatearSoles(d.cargos_cts)} icono="sube" />
            <TarjetaKPI titulo="Abonos" valor={formatearSoles(d.abonos_cts)} tono="acento" icono="baja" />
            <TarjetaKPI titulo="Saldo" valor={formatearSoles(d.saldo_final_cts)} tono={d.moroso ? 'alerta' : 'neutro'} icono="balance"
              nota={d.moroso ? `Vencido ${formatearSoles(d.deuda_vencida_cts)}` : 'Sin deuda vencida'} />
          </div>
          {lista(d.acuerdos).length > 0 && (
            <Seccion titulo="Acuerdo de pago activo">
              <ul className="divide-y divide-borde">
                {d.acuerdos.map((a) => (
                  <li key={a.id} className="flex flex-wrap items-center gap-3 py-2 text-sm">
                    <span className="font-medium text-tinta">{a.numero}</span>
                    <span className="text-texto-apoyo">{formatearSoles(a.monto_acordado_cts)} en {a.n_cuotas} cuotas · avance {formatearSoles(a.avance_cts)}</span>
                    {a.proxima_cuota && <span className="text-texto-apoyo">Próxima cuota: {formatearFecha(a.proxima_cuota)}</span>}
                    {a.vencido_cts > 0 && <Insignia estado="vencido" texto={`Cuotas vencidas ${formatearSoles(a.vencido_cts)}`} />}
                  </li>
                ))}
              </ul>
            </Seccion>
          )}
          <Seccion titulo={`Movimientos · Dpto ${d.unidad}${d.propietario ? ` · ${d.propietario}` : ''}`} padding="p-0">
            <Tabla
              etiqueta="Estado de cuenta"
              columnas={columnas}
              filas={(d.movimientos || []).map((m, i) => ({ ...m, _k: i }))}
              claveFila="_k"
              cargando={ec.cargando}
              vacio={<Vacio icono="recibo" titulo="Sin movimientos" texto="No hay cargos ni abonos en el rango elegido." />}
            />
          </Seccion>
        </>
      )}
    </>
  );
}
