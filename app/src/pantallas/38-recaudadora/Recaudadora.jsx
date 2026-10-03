import { useState } from 'react';
import { api, lista, subir, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Modal, SelectorPeriodo, SubirArchivo, Tabla, Vacio, useDialog, useToast } from '../../ui/index.js';

// Columnas que se mapean en cada reporte: [campo del formulario, clave del mapeo, etiqueta, obligatoria].
const COLUMNAS = {
  transacciones: [
    ['col_codigo_recibo', 'codigo_recibo', 'Código del recibo', true],
    ['col_monto', 'monto', 'Monto', true],
    ['col_codigo_operacion', 'codigo_operacion', 'Código de operación', true],
    ['col_fecha', 'fecha', 'Fecha', true],
    ['col_medio', 'medio', 'Medio o canal', false],
  ],
  liquidaciones: [
    ['col_codigo', 'codigo', 'Código de liquidación', false],
    ['col_fecha', 'fecha', 'Fecha', true],
    ['col_bruto', 'bruto', 'Monto bruto', true],
    ['col_comision', 'comision', 'Comisión (si no, se calcula)', false],
    ['col_neto', 'neto', 'Neto (se verifica)', false],
  ],
};

const ESTADO_FILA = {
  acreditada: { tono: 'acento', texto: 'Acreditada' },
  registrada: { tono: 'acento', texto: 'Registrada' },
  duplicada: { tono: 'neutro', texto: 'Ya estaba' },
  observada: { tono: 'aviso', texto: 'Observada' },
  error: { tono: 'alerta', texto: 'Error' },
};

const MEDIOS = [
  { valor: 'deposito', etiqueta: 'Depósito / agente' },
  { valor: 'transferencia', etiqueta: 'Transferencia' },
  { valor: 'yape', etiqueta: 'Yape' },
  { valor: 'plin', etiqueta: 'Plin' },
  { valor: 'tarjeta', etiqueta: 'Tarjeta' },
  { valor: 'efectivo', etiqueta: 'Efectivo' },
];

const CUENTA_NUEVA = { proveedor: '', codigo_convenio: '', medio: 'deposito', porcentaje: '', fijo_cts: null, rubro_id: '' };

/** Bloque A1 · Recaudadora externa: configuración y reportes de transacciones y liquidaciones (por archivo). */
export default function Recaudadora() {
  const eid = useEid();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [periodo, setPeriodo] = usePeriodo();
  const [vista, setVista] = useState('transacciones');
  const [cuentaForm, setCuentaForm] = useState(null);
  const [importar, setImportar] = useState(null); // 'transacciones' | 'liquidaciones'
  const [ocupado, setOcupado] = useState(false);

  const cuentas = useCarga(() => api.get(`/edificios/${eid}/recaudadora/cuentas`), [eid]);
  const tx = useCarga(() => api.get(`/edificios/${eid}/recaudadora/transacciones`), [eid]);
  const liq = useCarga(() => api.get(`/edificios/${eid}/recaudadora/liquidaciones`), [eid]);
  const recibos = useCarga(() => api.get(`/edificios/${eid}/recaudadora/recibos`, { periodo }), [eid, periodo]);
  const rubros = useCarga(() => api.get(`/edificios/${eid}/rubros`).catch(() => []), [eid]);

  const listaCuentas = lista(cuentas.datos);
  const listaTx = lista(tx.datos);
  const listaLiq = lista(liq.datos);
  const listaRecibos = lista(recibos.datos);
  const enviados = listaRecibos.filter((r) => r.enviado_recaudadora).length;

  const guardarCuenta = async () => {
    setOcupado(true);
    try {
      const cuerpo = {
        proveedor: cuentaForm.proveedor,
        codigo_convenio: cuentaForm.codigo_convenio,
        medio: cuentaForm.medio,
        activo: cuentaForm.activo ?? true,
        porcentaje_pbs: Math.round(Number(String(cuentaForm.porcentaje || '0').replace(',', '.')) * 100) || 0,
        fijo_cts: cuentaForm.fijo_cts || 0,
        rubro_id: Number(cuentaForm.rubro_id) || 0,
      };
      if (cuentaForm.id) await api.put(`/edificios/${eid}/recaudadora/cuentas/${cuentaForm.id}`, cuerpo);
      else await api.post(`/edificios/${eid}/recaudadora/cuentas`, cuerpo);
      toast('Recaudadora guardada.', { tipo: 'exito' });
      setCuentaForm(null);
      await cuentas.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo guardar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const enviar = async () => {
    const ok = await dialog.confirm({
      title: `¿Marcar los recibos de ${periodo} como enviados?`,
      text: 'Descarga antes el archivo de deudas y súbelo en el portal de la recaudadora. Solo se marcan los recibos con saldo que aún no se enviaron.',
    });
    if (!ok) return;
    try {
      const r = await api.post(`/edificios/${eid}/recaudadora/enviar`, { periodo });
      toast(`${r.marcados} recibo(s) marcados como enviados a la recaudadora.`, { tipo: 'exito' });
      await recibos.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo marcar', text: err.message });
    }
  };

  const insignia = (estado) => {
    const e = ESTADO_FILA[estado] || { tono: 'neutro', texto: estado };
    return <Insignia tono={e.tono} texto={e.texto} />;
  };

  const colTx = [
    { clave: 'fecha', titulo: 'Fecha', render: (f) => formatearFecha(f.fecha), movil: 'sub' },
    { clave: 'codigo_recibo', titulo: 'Código', render: (f) => f.recibo || f.codigo_recibo, movil: 'titulo' },
    { clave: 'unidad', titulo: 'Unidad', prioridad: 2 },
    { clave: 'codigo_operacion', titulo: 'Operación', prioridad: 2 },
    { clave: 'proveedor', titulo: 'Recaudadora', prioridad: 3 },
    { clave: 'monto_cts', titulo: 'Monto', alinear: 'der', render: (f) => formatearSoles(f.monto_cts), movil: 'valor' },
    { clave: 'estado', titulo: 'Estado', render: (f) => <span title={f.motivo || undefined}>{insignia(f.estado)}</span>, movil: 'valor2' },
  ];
  const colLiq = [
    { clave: 'fecha', titulo: 'Fecha', render: (f) => formatearFecha(f.fecha), movil: 'sub' },
    { clave: 'codigo_liquidacion', titulo: 'Liquidación', movil: 'titulo' },
    { clave: 'proveedor', titulo: 'Recaudadora', prioridad: 2 },
    { clave: 'monto_bruto_cts', titulo: 'Bruto', alinear: 'der', render: (f) => formatearSoles(f.monto_bruto_cts) },
    { clave: 'comision_cts', titulo: 'Comisión', alinear: 'der', render: (f) => formatearSoles(f.comision_cts), movil: 'valor2' },
    { clave: 'monto_neto_cts', titulo: 'Neto', alinear: 'der', render: (f) => formatearSoles(f.monto_neto_cts), movil: 'valor' },
  ];
  const colRec = [
    { clave: 'unidad', titulo: 'Unidad', movil: 'titulo' },
    { clave: 'numero', titulo: 'Recibo', movil: 'sub' },
    { clave: 'titular', titulo: 'Titular', prioridad: 2 },
    { clave: 'saldo_cts', titulo: 'Saldo', alinear: 'der', render: (f) => formatearSoles(f.saldo_cts), movil: 'valor' },
    {
      clave: 'enviado_recaudadora',
      titulo: 'Estado',
      movil: 'valor2',
      render: (f) => (f.enviado_recaudadora ? <Insignia tono="curso" texto="Enviado a recaudadora" /> : <Insignia tono="neutro" texto="Sin enviar" />),
    },
  ];

  const sinCuentas = cuentas.datos && listaCuentas.length === 0;

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Recaudadora"
        subtitulo="Pagos por código en banco, agentes y billeteras"
        ayuda="La recaudadora cobra el recibo por su código y te entrega dos reportes: transacciones (cada pago) y liquidaciones (lo que te abona, neto de su comisión). Súbelos aquí: un mismo código de operación nunca se cobra dos veces."
        acciones={
          <>
            <Boton variante="secundario" icono="engranaje" onClick={() => setVista('cuentas')}>
              Configuración
            </Boton>
            <Boton icono="subir" disabled={sinCuentas} onClick={() => setImportar(vista === 'liquidaciones' ? 'liquidaciones' : 'transacciones')}>
              Subir reporte
            </Boton>
          </>
        }
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'transacciones'} icono="entrante" onClick={() => setVista('transacciones')} contador={listaTx.length}>
            Transacciones
          </Chip>
          <Chip activo={vista === 'liquidaciones'} icono="balance" onClick={() => setVista('liquidaciones')} contador={listaLiq.length}>
            Liquidaciones
          </Chip>
          <Chip activo={vista === 'recibos'} icono="recibo" onClick={() => setVista('recibos')} contador={enviados}>
            Recibos enviados
          </Chip>
          <Chip activo={vista === 'cuentas'} icono="engranaje" onClick={() => setVista('cuentas')} contador={listaCuentas.length}>
            Recaudadoras
          </Chip>
        </div>

        {sinCuentas && vista !== 'cuentas' && (
          <div className="rounded-tarjeta border border-aviso-borde bg-aviso-suave p-3 text-sm text-aviso-texto">
            Primero registra la recaudadora (proveedor, convenio y comisión) en «Recaudadoras».
          </div>
        )}

        {vista === 'transacciones' && (
          <Seccion titulo="Reporte de transacciones" extra={tx.datos && <span className="text-sm text-texto-apoyo">Acreditado {formatearSoles(tx.datos.acreditado_cts || 0)}</span>} padding="p-0 sm:p-tarjeta">
            <Tabla
              etiqueta="Transacciones de la recaudadora"
              columnas={colTx}
              filas={listaTx}
              cargando={!tx.datos}
              error={tx.error}
              onReintentar={tx.recargar}
              densa
              vacio={<Vacio icono="entrante" titulo="Sin transacciones" texto="Sube el reporte de transacciones que te entrega la recaudadora." />}
            />
          </Seccion>
        )}

        {vista === 'liquidaciones' && (
          <Seccion
            titulo="Reporte de liquidaciones"
            extra={liq.datos && <span className="text-sm text-texto-apoyo">Bruto {formatearSoles(liq.datos.bruto_cts || 0)} − comisión {formatearSoles(liq.datos.comision_cts || 0)} = neto {formatearSoles(liq.datos.neto_cts || 0)}</span>}
            padding="p-0 sm:p-tarjeta"
          >
            <Tabla
              etiqueta="Liquidaciones de la recaudadora"
              columnas={colLiq}
              filas={listaLiq}
              cargando={!liq.datos}
              error={liq.error}
              onReintentar={liq.recargar}
              densa
              vacio={<Vacio icono="balance" titulo="Sin liquidaciones" texto="Cada liquidación registra su comisión como egreso." />}
            />
          </Seccion>
        )}

        {vista === 'recibos' && (
          <Seccion
            titulo="Recibos con saldo del periodo"
            extra={
              <div className="flex flex-wrap items-center gap-2">
                <SelectorPeriodo periodo={periodo} onCambio={setPeriodo} />
                <Boton tamano="sm" variante="secundario" icono="descargar" href={urlApi(`/edificios/${eid}/recaudadora/deudas.csv?periodo=${periodo}`)}>
                  Archivo de deudas
                </Boton>
                <Boton tamano="sm" icono="enviar" onClick={enviar} disabled={!listaRecibos.length}>
                  Marcar como enviados
                </Boton>
              </div>
            }
            padding="p-0 sm:p-tarjeta"
          >
            <Tabla
              etiqueta="Recibos y su envío a la recaudadora"
              columnas={colRec}
              filas={listaRecibos}
              cargando={!recibos.datos}
              error={recibos.error}
              onReintentar={recibos.recargar}
              densa
              vacio={<Vacio icono="recibo" titulo="Sin recibos con saldo" texto="El periodo no tiene recibos por cobrar." />}
            />
          </Seccion>
        )}

        {vista === 'cuentas' && (
          <Seccion titulo="Recaudadoras" extra={<Boton tamano="sm" icono="mas_signo" onClick={() => setCuentaForm({ ...CUENTA_NUEVA })}>Nueva recaudadora</Boton>}>
            {cuentas.error ? (
              <ErrorCarga error={cuentas.error} onReintentar={cuentas.recargar} />
            ) : !cuentas.datos ? (
              <Esqueleto className="h-24 w-full" />
            ) : listaCuentas.length === 0 ? (
              <Vacio icono="engranaje" titulo="Sin recaudadoras" texto="Registra el banco, agente o billetera con el que tienes convenio de recaudación." />
            ) : (
              <ul className="divide-y divide-borde">
                {listaCuentas.map((c) => (
                  <li key={c.id} className="flex flex-wrap items-center gap-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-tinta">
                        {c.proveedor}
                        {c.codigo_convenio && ` · convenio ${c.codigo_convenio}`}
                      </p>
                      <p className="truncate text-xs text-texto-apoyo">
                        Comisión {(c.porcentaje_pbs / 100).toLocaleString('es-PE')} % + {formatearSoles(c.fijo_cts)} por liquidación · {c.acreditadas} acreditadas · {c.observadas} observadas
                      </p>
                    </div>
                    {!c.activo && <Insignia tono="hecho" texto="Inactiva" />}
                    <Boton
                      tamano="sm"
                      variante="secundario"
                      onClick={() =>
                        setCuentaForm({ id: c.id, proveedor: c.proveedor, codigo_convenio: c.codigo_convenio, medio: c.medio, activo: c.activo, porcentaje: String(c.porcentaje_pbs / 100), fijo_cts: c.fijo_cts, rubro_id: c.rubro_id ? String(c.rubro_id) : '' })
                      }
                    >
                      Editar
                    </Boton>
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        )}
      </Contenido>

      <Modal
        abierto={!!cuentaForm}
        onCerrar={() => setCuentaForm(null)}
        titulo={cuentaForm?.id ? 'Editar recaudadora' : 'Nueva recaudadora'}
        ancho="max-w-md"
        pie={
          <>
            <Boton variante="fantasma" onClick={() => setCuentaForm(null)}>
              Cancelar
            </Boton>
            <Boton cargando={ocupado} onClick={guardarCuenta}>
              Guardar
            </Boton>
          </>
        }
      >
        {cuentaForm && (
          <div className="flex flex-col gap-3">
            <Campo etiqueta="Proveedor" valor={cuentaForm.proveedor} onCambio={(v) => setCuentaForm({ ...cuentaForm, proveedor: v })} ayuda="Banco, red de agentes o billetera." />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Código de convenio" valor={cuentaForm.codigo_convenio} onCambio={(v) => setCuentaForm({ ...cuentaForm, codigo_convenio: v })} />
              <Campo etiqueta="Medio por defecto" tipo="select" opciones={MEDIOS} valor={cuentaForm.medio} onCambio={(v) => setCuentaForm({ ...cuentaForm, medio: v })} />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Comisión %" tipo="numero" valor={cuentaForm.porcentaje} onCambio={(v) => setCuentaForm({ ...cuentaForm, porcentaje: v })} />
              <Campo etiqueta="Fijo por liquidación" tipo="dinero" valor={cuentaForm.fijo_cts} onCambio={(v) => setCuentaForm({ ...cuentaForm, fijo_cts: v })} />
            </div>
            <Campo
              etiqueta="Rubro del egreso de comisión"
              tipo="select"
              valor={cuentaForm.rubro_id}
              onCambio={(v) => setCuentaForm({ ...cuentaForm, rubro_id: v })}
              opciones={[{ valor: '', etiqueta: '— Administración —' }, ...lista(rubros.datos).map((r) => ({ valor: String(r.id), etiqueta: r.nombre }))]}
            />
            {cuentaForm.id && (
              <Campo etiqueta="Estado" tipo="select" valor={cuentaForm.activo === false ? 'no' : 'si'} onCambio={(v) => setCuentaForm({ ...cuentaForm, activo: v === 'si' })} opciones={[{ valor: 'si', etiqueta: 'Activa' }, { valor: 'no', etiqueta: 'Inactiva' }]} />
            )}
          </div>
        )}
      </Modal>

      <ImportarReporte
        abierto={!!importar}
        tipo={importar}
        onTipo={setImportar}
        cuentas={listaCuentas.filter((c) => c.activo)}
        eid={eid}
        insignia={insignia}
        onCerrar={() => setImportar(null)}
        onListo={async (r) => {
          setImportar(null);
          toast(importar === 'liquidaciones' ? `${r.registradas} liquidación(es) registradas.` : `${r.acreditadas} pago(s) acreditados.`, { tipo: 'exito' });
          await Promise.all([tx.recargar(), liq.recargar(), cuentas.recargar(), recibos.recargar()]);
        }}
        dialog={dialog}
      />
    </>
  );
}

/** Subir un reporte: elegir columnas, ver la vista previa exacta y confirmar. */
function ImportarReporte({ abierto, tipo, onTipo, cuentas, eid, insignia, onCerrar, onListo, dialog }) {
  const [cuenta, setCuenta] = useState('');
  const [archivo, setArchivo] = useState(null);
  const [mapeo, setMapeo] = useState({});
  const [previa, setPrevia] = useState(null);
  const [cargando, setCargando] = useState(false);
  const cols = COLUMNAS[tipo] || [];
  const cid = cuenta || (cuentas[0] && String(cuentas[0].id)) || '';

  const reiniciar = () => {
    setArchivo(null);
    setMapeo({});
    setPrevia(null);
  };

  const formulario = (vistaPrevia) => {
    const form = new FormData();
    form.set('archivo', archivo);
    for (const [campo] of cols) if (mapeo[campo]) form.set(campo, mapeo[campo]);
    if (vistaPrevia) form.set('vista_previa', '1');
    return form;
  };

  const llamar = async (vistaPrevia) => {
    setCargando(true);
    try {
      const r = await subir(`/edificios/${eid}/recaudadora/cuentas/${cid}/${tipo}/importar`, formulario(vistaPrevia));
      const m = {};
      for (const [campo, clave] of cols) m[campo] = r.mapeo?.[clave] || '';
      setMapeo(m);
      if (vistaPrevia) setPrevia(r);
      else {
        reiniciar();
        onListo(r);
      }
    } catch (err) {
      // 422 con cabeceras: falta mapear columnas; se muestran para elegir.
      if (err.detalle?.cabeceras) setPrevia({ cabeceras: err.detalle.cabeceras, filas: [], falta: true });
      else await dialog.alert({ title: 'No se pudo leer el reporte', text: err.message });
    } finally {
      setCargando(false);
    }
  };

  const opciones = [{ valor: '', etiqueta: '— No está —' }, ...(previa?.cabeceras || []).map((c) => ({ valor: c, etiqueta: c }))];
  const filas = previa?.filas || [];
  const resumen =
    tipo === 'liquidaciones'
      ? previa && !previa.falta && `${previa.registradas} se registran · ${previa.duplicadas} ya estaban · ${previa.errores} con error · neto ${formatearSoles(previa.neto_cts || 0)}`
      : previa && !previa.falta && `${previa.acreditadas} se acreditan (${formatearSoles(previa.monto_acreditado_cts || 0)}) · ${previa.duplicadas} ya estaban · ${previa.observadas} observadas · ${previa.errores} con error`;

  return (
    <Modal
      abierto={abierto}
      onCerrar={() => (reiniciar(), onCerrar())}
      titulo="Subir reporte de la recaudadora"
      ancho="max-w-2xl"
      pie={
        <>
          <Boton variante="fantasma" onClick={() => (reiniciar(), onCerrar())}>
            Cancelar
          </Boton>
          <Boton variante="secundario" disabled={!archivo || !cid} cargando={cargando} onClick={() => llamar(true)}>
            Vista previa
          </Boton>
          <Boton disabled={!previa || previa.falta} cargando={cargando} onClick={() => llamar(false)}>
            Importar
          </Boton>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="grid grid-cols-2 gap-3">
          <Campo
            etiqueta="Reporte"
            tipo="select"
            valor={tipo || 'transacciones'}
            onCambio={(v) => (reiniciar(), onTipo(v))}
            opciones={[
              { valor: 'transacciones', etiqueta: 'Transacciones' },
              { valor: 'liquidaciones', etiqueta: 'Liquidaciones' },
            ]}
          />
          <Campo etiqueta="Recaudadora" tipo="select" valor={cid} onCambio={(v) => (setCuenta(v), setPrevia(null))} opciones={cuentas.map((c) => ({ valor: String(c.id), etiqueta: c.proveedor + (c.codigo_convenio ? ` · ${c.codigo_convenio}` : '') }))} />
        </div>
        <SubirArchivo
          etiqueta="Archivo (CSV o Excel)"
          ayuda="CSV con comas o punto y coma, o XLSX; hasta 10 MB"
          aceptar=".csv,.txt,.xlsx,text/csv"
          tipos={['text/csv', 'text/plain', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet']}
          extensiones={['.csv', '.txt', '.xlsx']}
          archivo={archivo}
          onArchivo={(f) => (setArchivo(f), setPrevia(null))}
        />
        {previa && (
          <fieldset className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <legend className="mb-1 text-sm font-semibold">Columnas del archivo</legend>
            {cols.map(([campo, , etiqueta, obligatoria]) => (
              <Campo key={campo} etiqueta={etiqueta + (obligatoria ? '' : ' (opcional)')} tipo="select" opciones={opciones} valor={mapeo[campo] || ''} onCambio={(v) => (setMapeo((m) => ({ ...m, [campo]: v })), setPrevia((p) => ({ ...p, falta: true })))} />
            ))}
          </fieldset>
        )}
        {previa?.falta && <p className="text-sm text-aviso-texto">Elige las columnas y vuelve a pedir la vista previa.</p>}
        {resumen && <p className="text-sm font-semibold text-tinta">{resumen}</p>}
        {filas.length > 0 && (
          <ul className="max-h-72 divide-y divide-borde overflow-y-auto rounded-control border border-borde">
            {filas.map((f) => (
              <li key={f.fila} className="flex items-center gap-3 px-3 py-2 text-sm">
                <span className="w-10 shrink-0 text-xs text-texto-apoyo">#{f.fila}</span>
                <span className="min-w-0 flex-1 truncate">
                  {tipo === 'liquidaciones'
                    ? `${f.codigo || '—'} · bruto ${formatearSoles(f.bruto_cts || 0)} − comisión ${formatearSoles(f.comision_cts || 0)}`
                    : `${f.recibo || f.codigo_recibo || '—'}${f.unidad ? ` · Dpto ${f.unidad}` : ''} · ${f.codigo_operacion || '—'} · ${formatearSoles(f.monto_cts || 0)}`}
                  {f.motivo && <span className="block truncate text-xs text-texto-apoyo">{f.motivo}</span>}
                </span>
                {insignia(f.estado)}
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}
