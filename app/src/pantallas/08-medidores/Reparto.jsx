import { useState } from 'react';
import { api } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles, formatearNumero } from '../../lib/dinero.js';
import { mesDePeriodo } from '../../lib/fechas.js';
import { ruta } from '../../lib/nav.jsx';
import { useEid } from '../../layout/Sesion.jsx';
import { usePeriodo } from '../../layout/usePeriodo.js';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton, Campo, ErrorCarga, Esqueleto, Insignia, Modal, SelectorPeriodo, SubirFoto, Tabla, TarjetaKPI, useDialog, useToast } from '../../ui/index.js';

/** 08 · Vista previa del reparto del recibo general (administración). */
export default function Reparto() {
  const eid = useEid();
  const [periodo, setPeriodo] = usePeriodo();
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();
  const [aprobando, setAprobando] = useState(false);
  const [generalAbierto, setGeneralAbierto] = useState(false);
  const calc = useCarga(() => api.post(`/edificios/${eid}/periodos/${periodo}/reparto-medidores/calcular?tipo=agua`, {}), [eid, periodo]);
  const d = calc.datos;
  const tarifaM3 = d?.tarifa_cts_x_1000 != null ? d.tarifa_cts_x_1000 / 1000 : null;

  const aprobar = async () => {
    const ok = await dialog.confirm({ title: `¿Aprobar el reparto de ${mesDePeriodo(periodo)}?`, text: 'Escribe las líneas de agua en los recibos borrador del periodo.', okText: 'Aprobar reparto' });
    if (!ok) return;
    setAprobando(true);
    try {
      await api.post(`/edificios/${eid}/periodos/${periodo}/reparto-medidores/aprobar?tipo=agua`, {});
      toast('Reparto aprobado: las líneas de agua ya están en los recibos borrador.', { tipo: 'exito' });
    } catch (err) {
      await dialog.alert({ title: err.codigo === 'DIFERENCIA_NEGATIVA' ? 'Los departamentos suman más que el general' : 'No se pudo aprobar', text: err.message });
    } finally {
      setAprobando(false);
    }
  };

  const columnas = [
    { clave: 'unidad', titulo: 'Unidad', movil: 'titulo' },
    { clave: 'consumo', titulo: 'Consumo', alinear: 'der', render: (l) => `${formatearNumero(l.consumo, 3)} m³` },
    { clave: 'propio_cts', titulo: 'Agua propia', alinear: 'der', render: (l) => formatearSoles(l.propio_cts) },
    { clave: 'comun_cts', titulo: 'Áreas comunes', alinear: 'der', render: (l) => formatearSoles(l.comun_cts) },
    { clave: 'total_cts', titulo: 'Total', alinear: 'der', movil: 'valor', render: (l) => formatearSoles(l.total_cts) },
  ];
  const sumaLineas = (d?.lineas || []).reduce((a, l) => a + l.total_cts, 0);

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Reparto del agua"
        volver={ruta('medidores', { periodo })}
        acciones={
          <>
            <SelectorPeriodo periodo={periodo} onCambio={setPeriodo} />
            <Boton variante="secundario" icono="camara" onClick={() => setGeneralAbierto(true)}>
              Registrar recibo general
            </Boton>
            <Boton onClick={aprobar} cargando={aprobando} disabled={!d}>
              Aprobar reparto
            </Boton>
          </>
        }
      />
      <Contenido>
        {calc.error ? (
          <ErrorCarga error={calc.error} onReintentar={calc.recargar} />
        ) : (
          <>
            <div className="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
              <TarjetaKPI cargando={!d} titulo="Recibo general" valor={formatearSoles(d?.recibo_general_cts)} nota={d?.consumo_general ? `${formatearNumero(d.consumo_general, 3)} m³` : undefined} />
              <TarjetaKPI cargando={!d} titulo="Tarifa efectiva" valor={tarifaM3 != null ? `${formatearSoles(Math.round(tarifaM3))}/m³` : '—'} nota="total ÷ m³ del general" />
              <TarjetaKPI cargando={!d} titulo="Departamentos" valor={formatearSoles(d?.total_unidades_cts)} nota="suma de consumos propios" />
              <TarjetaKPI cargando={!d} tono={d?.diferencia_cts < 0 ? 'alerta' : 'acento'} titulo="Áreas comunes" valor={formatearSoles(d?.diferencia_cts)} nota="se reparte por participación" />
            </div>
            {d?.alertas?.length > 0 && (
              <div className="flex flex-wrap items-center gap-2 rounded-tarjeta border border-aviso-borde bg-aviso-suave p-3 text-sm text-aviso-texto">
                Revisa antes de aprobar:
                {d.alertas.map((a) => (
                  <Insignia key={a.unidad} estado={a.alerta} texto={`${a.unidad} · ${a.alerta === 'PICO' ? 'pico' : 'negativo'}`} />
                ))}
              </div>
            )}
            <div className="overflow-hidden rounded-tarjeta border border-borde bg-superficie">
              {!d ? <Esqueleto className="m-4 h-64" /> : <Tabla etiqueta="Reparto por unidad" columnas={columnas} filas={d.lineas || []} claveFila="unidad" densa />}
              {d && (
                <div className="flex justify-between border-t border-borde px-4 py-3 text-sm font-semibold">
                  <span>Total repartido</span>
                  <span className={`tabular-nums ${sumaLineas === d.recibo_general_cts ? 'text-acento' : 'text-alerta'}`}>
                    {formatearSoles(sumaLineas)} {sumaLineas === d.recibo_general_cts ? '· cuadra con el general' : '· no cuadra'}
                  </span>
                </div>
              )}
            </div>
          </>
        )}
      </Contenido>
      <ReciboGeneral abierto={generalAbierto} onCerrar={() => setGeneralAbierto(false)} eid={eid} periodo={periodo} onListo={() => (setGeneralAbierto(false), toast('Recibo general registrado.', { tipo: 'exito' }), calc.recargar())} dialog={dialog} />
    </>
  );
}

function ReciboGeneral({ abierto, onCerrar, eid, periodo, onListo, dialog }) {
  const [monto, setMonto] = useState(null);
  const [consumo, setConsumo] = useState('');
  const [foto, setFoto] = useState(null);
  const [enviando, setEnviando] = useState(false);
  const enviar = async () => {
    setEnviando(true);
    try {
      const form = new FormData();
      form.set('tipo', 'agua');
      form.set('monto_cts', String(monto));
      form.set('consumo_total', consumo.replace(',', '.'));
      form.set('foto_recibo', foto.archivo);
      await api.form('POST', `/edificios/${eid}/periodos/${periodo}/recibo-general`, form);
      onListo();
    } catch (err) {
      dialog.alert({ title: 'No se pudo registrar', text: err.message });
    } finally {
      setEnviando(false);
    }
  };
  return (
    <Modal
      abierto={abierto}
      onCerrar={onCerrar}
      titulo="Recibo general de agua"
      pie={
        <>
          <Boton variante="secundario" onClick={onCerrar}>
            Cancelar
          </Boton>
          <Boton onClick={enviar} cargando={enviando} disabled={!monto || !consumo || !foto}>
            Registrar
          </Boton>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Campo etiqueta="Monto del recibo" tipo="dinero" valor={monto} onCambio={setMonto} />
        <Campo etiqueta="Consumo del medidor general (m³)" tipo="numero" valor={consumo} onCambio={setConsumo} placeholder="357,143" />
        <SubirFoto foto={foto} onFoto={setFoto} etiqueta="Foto del recibo" obligatoria alto="h-40" oscuro={false} />
      </div>
    </Modal>
  );
}
