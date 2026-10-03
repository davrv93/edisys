import { useState } from 'react';
import { api, lista, subir, urlApi } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, Insignia, Modal, SelectorPeriodo, SubirArchivo, Tabla, Vacio, useDialog, useToast } from '../../ui/index.js';

const ESTADO_CREP = {
  completado: { tono: 'acento', texto: 'Completado' },
  expirado: { tono: 'hecho', texto: 'Expirado' },
};
const ESTADO_PAGO = {
  emparejado: { tono: 'acento', texto: 'Emparejado' },
  ya_conciliado: { tono: 'neutro', texto: 'Ya conciliado' },
  error: { tono: 'alerta', texto: 'Error' },
};

/**
 * Bloque A2 · Cuenta recaudadora en banco, por archivo (cero conexión con el banco).
 * «Descargas»: el CREP del periodo (TXT) que se sube al banco a mano; se guarda 24 h.
 * «Cobranza masiva»: subir el CDPG que devuelve el banco, revisar la vista previa y registrar los pagos.
 */
export default function CobranzaMasiva() {
  const eid = useEid();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [periodo, setPeriodo] = usePeriodo();
  const [vista, setVista] = useState('descargas');
  const [generar, setGenerar] = useState(null); // {cuenta, layout}
  const [ocupado, setOcupado] = useState(false);

  const creps = useCarga(() => api.get(`/edificios/${eid}/crep`), [eid]);
  const cargas = useCarga(() => api.get(`/edificios/${eid}/cdpg`), [eid]);
  const cuentas = useCarga(() => api.get(`/edificios/${eid}/cuentas-bancarias`), [eid]);
  const layouts = useCarga(() => api.get(`/edificios/${eid}/crep/layouts`), [eid]);

  const listaCreps = lista(creps.datos);
  const listaCargas = lista(cargas.datos);
  const listaCuentas = lista(cuentas.datos).filter((c) => c.activo);
  const listaLayouts = lista(layouts.datos);
  const provisional = listaLayouts.some((l) => l.provisional);

  const generarAhora = async () => {
    setOcupado(true);
    try {
      const r = await api.post(`/edificios/${eid}/crep`, { periodo, cuenta_bancaria_id: Number(generar.cuenta), layout: generar.layout });
      toast(`CREP listo: ${r.filas} deuda(s) por ${formatearSoles(r.total_cts)}. Descárgalo antes de ${r.expira_en}.`, { tipo: 'exito' });
      setGenerar(null);
      await creps.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo generar el CREP', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const colCrep = [
    { clave: 'tipo', titulo: 'Tipo', movil: 'sub' },
    { clave: 'nombre_archivo', titulo: 'Archivo', movil: 'titulo' },
    { clave: 'periodo', titulo: 'Periodo', prioridad: 2 },
    { clave: 'banco', titulo: 'Cuenta', prioridad: 3, render: (f) => [f.banco, f.cuenta].filter(Boolean).join(' · ') },
    { clave: 'total_cts', titulo: 'Total', alinear: 'der', prioridad: 2, render: (f) => `${formatearSoles(f.total_cts)} · ${f.filas}` },
    {
      clave: 'estado',
      titulo: 'Estado',
      movil: 'valor2',
      render: (f) => {
        const e = ESTADO_CREP[f.estado] || { tono: 'neutro', texto: f.estado };
        return <Insignia tono={e.tono} texto={e.texto} />;
      },
    },
    { clave: 'expira', titulo: 'Disponible hasta', prioridad: 2, render: (f) => (f.estado === 'expirado' ? '—' : f.expira) },
    {
      clave: 'accion',
      titulo: '',
      alinear: 'der',
      movil: 'valor',
      render: (f) =>
        f.estado === 'completado' ? (
          <Boton tamano="sm" variante="secundario" icono="descargar" href={urlApi(`/edificios/${eid}/crep/${f.id}/descargar`)}>
            Descargar
          </Boton>
        ) : null,
    },
  ];

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Cobranza masiva"
        subtitulo="CREP y CDPG de la cuenta recaudadora"
        ayuda="Genera el CREP del periodo, descárgalo (se guarda 24 h) y súbelo al banco. Cuando el banco devuelva el CDPG, súbelo en «Cobranza masiva»: cada pago se empareja con su recibo y ninguno se registra dos veces."
        acciones={
          <Boton icono="mas_signo" disabled={!listaCuentas.length} onClick={() => setGenerar({ cuenta: listaCuentas[0] ? String(listaCuentas[0].id) : '', layout: listaLayouts[0]?.codigo || 'bcp_ref' })}>
            Generar CREP
          </Boton>
        }
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'descargas'} icono="descargar" onClick={() => setVista('descargas')} contador={listaCreps.filter((c) => c.estado === 'completado').length}>
            Descargas
          </Chip>
          <Chip activo={vista === 'masiva'} icono="subir" onClick={() => setVista('masiva')} contador={listaCargas.length}>
            Cobranza masiva
          </Chip>
        </div>

        {provisional && (
          <div className="rounded-tarjeta border border-aviso-borde bg-aviso-suave p-3 text-sm text-aviso-texto">
            El layout del banco es de referencia (provisional) hasta tener el del convenio. Revisa el archivo con el banco antes de usarlo en producción.
          </div>
        )}
        {cuentas.datos && !listaCuentas.length && (
          <div className="rounded-tarjeta border border-aviso-borde bg-aviso-suave p-3 text-sm text-aviso-texto">Registra la cuenta recaudadora del banco en «Vouchers y cuentas bancarias» para generar el CREP.</div>
        )}

        {vista === 'descargas' ? (
          <Seccion titulo="Descargas" padding="p-0 sm:p-tarjeta">
            <Tabla
              etiqueta="Archivos CREP generados"
              columnas={colCrep}
              filas={listaCreps}
              cargando={!creps.datos}
              error={creps.error}
              onReintentar={creps.recargar}
              densa
              vacio={<Vacio icono="descargar" titulo="Sin archivos" texto="Genera el CREP del periodo para subirlo al banco." />}
            />
          </Seccion>
        ) : (
          <CobranzaCDPG eid={eid} creps={listaCreps} layouts={listaLayouts} cargas={cargas} dialog={dialog} toast={toast} />
        )}
      </Contenido>

      <Modal
        abierto={!!generar}
        onCerrar={() => setGenerar(null)}
        titulo="Generar CREP"
        ancho="max-w-md"
        pie={
          <>
            <Boton variante="fantasma" onClick={() => setGenerar(null)}>
              Cancelar
            </Boton>
            <Boton cargando={ocupado} disabled={!generar?.cuenta} onClick={generarAhora}>
              Generar
            </Boton>
          </>
        }
      >
        {generar && (
          <div className="flex flex-col gap-3">
            <div className="flex items-center justify-between gap-2">
              <span className="text-sm font-semibold">Periodo</span>
              <SelectorPeriodo periodo={periodo} onCambio={setPeriodo} />
            </div>
            <Campo etiqueta="Cuenta recaudadora" tipo="select" valor={generar.cuenta} onCambio={(v) => setGenerar({ ...generar, cuenta: v })} opciones={listaCuentas.map((c) => ({ valor: String(c.id), etiqueta: `${c.banco} · ${c.numero}` }))} />
            <Campo etiqueta="Layout del banco" tipo="select" valor={generar.layout} onCambio={(v) => setGenerar({ ...generar, layout: v })} opciones={listaLayouts.map((l) => ({ valor: l.codigo, etiqueta: l.nombre + (l.provisional ? ' (provisional)' : '') }))} />
            <p className="text-sm text-texto-apoyo">Incluye los recibos del periodo con saldo y los marca como «Enviado a recaudadora». El archivo se guarda 24 h.</p>
          </div>
        )}
      </Modal>
    </>
  );
}

/** Subir el CDPG, ver la vista previa (emparejados y errores) y registrar. */
function CobranzaCDPG({ eid, creps, layouts, cargas, dialog, toast }) {
  const [archivo, setArchivo] = useState(null);
  const [layout, setLayout] = useState('');
  const [crep, setCrep] = useState('');
  const [previa, setPrevia] = useState(null);
  const [cargando, setCargando] = useState(false);

  const formulario = (vistaPrevia) => {
    const form = new FormData();
    form.set('archivo', archivo);
    form.set('layout', layout || layouts[0]?.codigo || 'bcp_ref');
    if (crep) form.set('crep_archivo_id', crep);
    if (vistaPrevia) form.set('vista_previa', '1');
    return form;
  };

  const llamar = async (vistaPrevia) => {
    if (!vistaPrevia) {
      const ok = await dialog.confirm({
        title: `¿Registrar ${previa.filas_ok} pago(s) por ${formatearSoles(previa.total_cts)}?`,
        text: previa.filas_error ? `${previa.filas_error} fila(s) con error quedan fuera; puedes corregirlas y volver a subir el archivo.` : 'Cada pago se aplica a su recibo y queda validado.',
      });
      if (!ok) return;
    }
    setCargando(true);
    try {
      const r = await subir(`/edificios/${eid}/cdpg`, formulario(vistaPrevia));
      if (vistaPrevia) setPrevia(r);
      else {
        toast(`${r.filas_ok} pago(s) registrados por ${formatearSoles(r.total_cts)}.`, { tipo: 'exito' });
        setPrevia(null);
        setArchivo(null);
        await cargas.recargar();
      }
    } catch (err) {
      await dialog.alert({ title: 'No se pudo leer el CDPG', text: err.message });
    } finally {
      setCargando(false);
    }
  };

  const colCargas = [
    { clave: 'creado', titulo: 'Subido', movil: 'sub' },
    { clave: 'nombre_archivo', titulo: 'Archivo', movil: 'titulo' },
    { clave: 'crep', titulo: 'Responde a', prioridad: 3 },
    { clave: 'filas_ok', titulo: 'Emparejados', alinear: 'der', movil: 'valor2' },
    { clave: 'filas_error', titulo: 'Con error', alinear: 'der', prioridad: 2 },
    { clave: 'total_cts', titulo: 'Total', alinear: 'der', movil: 'valor', render: (f) => formatearSoles(f.total_cts) },
  ];

  return (
    <>
      <Seccion titulo="Subir CDPG del banco">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Campo etiqueta="Layout del banco" tipo="select" valor={layout || layouts[0]?.codigo || ''} onCambio={(v) => (setLayout(v), setPrevia(null))} opciones={layouts.map((l) => ({ valor: l.codigo, etiqueta: l.nombre + (l.provisional ? ' (provisional)' : '') }))} />
          <Campo etiqueta="Responde al CREP (opcional)" tipo="select" valor={crep} onCambio={setCrep} opciones={[{ valor: '', etiqueta: '—' }, ...creps.map((c) => ({ valor: String(c.id), etiqueta: `${c.nombre_archivo} · ${c.periodo}` }))]} />
        </div>
        <SubirArchivo
          etiqueta="Archivo CDPG (TXT)"
          ayuda="El archivo de pagos que devuelve el banco; hasta 10 MB"
          aceptar=".txt,text/plain"
          tipos={['text/plain']}
          extensiones={['.txt', '.dat']}
          archivo={archivo}
          onArchivo={(f) => (setArchivo(f), setPrevia(null))}
        />
        <div className="flex flex-wrap justify-end gap-2">
          <Boton variante="secundario" disabled={!archivo} cargando={cargando && !previa} onClick={() => llamar(true)}>
            Vista previa
          </Boton>
          <Boton disabled={!previa || !previa.filas_ok} cargando={cargando && !!previa} onClick={() => llamar(false)}>
            Registrar pagos
          </Boton>
        </div>
        {previa && (
          <div className="flex flex-col gap-2">
            <p className="text-sm font-semibold text-tinta">
              {previa.filas_ok} emparejado(s) por {formatearSoles(previa.total_cts)} · {previa.filas_error} con error
            </p>
            <ul className="max-h-80 divide-y divide-borde overflow-y-auto rounded-control border border-borde">
              {(previa.filas || []).map((f) => {
                const e = ESTADO_PAGO[f.estado] || { tono: 'neutro', texto: f.estado };
                return (
                  <li key={f.linea} className="flex items-center gap-3 px-3 py-2 text-sm">
                    <span className="w-12 shrink-0 text-xs text-texto-apoyo">L{f.linea}</span>
                    <span className="min-w-0 flex-1 truncate">
                      {f.recibo || f.referencia || '—'}
                      {f.unidad ? ` · Dpto ${f.unidad}` : ''} · op. {f.numero_operacion} · {formatearFecha(f.fecha)} · {formatearSoles(f.monto_cts)}
                      {f.motivo && <span className="block truncate text-xs text-texto-apoyo">{f.motivo}</span>}
                    </span>
                    <Insignia tono={e.tono} texto={e.texto} />
                  </li>
                );
              })}
              {(previa.errores_lectura || []).map((x) => (
                <li key={`l${x.linea}`} className="flex items-center gap-3 px-3 py-2 text-sm">
                  <span className="w-12 shrink-0 text-xs text-texto-apoyo">L{x.linea}</span>
                  <span className="min-w-0 flex-1 truncate text-texto-apoyo">{x.motivo}</span>
                  <Insignia tono="alerta" texto="Ilegible" />
                </li>
              ))}
            </ul>
          </div>
        )}
      </Seccion>
      <Seccion titulo="Cargas anteriores" padding="p-0 sm:p-tarjeta">
        <Tabla
          etiqueta="Cargas de CDPG"
          columnas={colCargas}
          filas={lista(cargas.datos)}
          cargando={!cargas.datos}
          error={cargas.error}
          onReintentar={cargas.recargar}
          densa
          vacio={<Vacio icono="subir" titulo="Sin cargas" texto="Aquí quedan los CDPG subidos y su resultado." />}
        />
      </Seccion>
    </>
  );
}
