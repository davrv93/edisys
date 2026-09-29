import { useState } from 'react';
import { subir, api, urlApi } from '../../lib/api.js';
import { formatearPct } from '../../lib/dinero.js';
import { revisarSuma, enmascararDni } from '../../lib/participacion.js';
import { Boton, SubirArchivo, TarjetaKPI, Vacio, useDialog, useToast } from '../../ui/index.js';

const TIPOS_XLSX = ['application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', 'application/vnd.ms-excel'];

/** Importación del padrón en 3 pasos: subir → validar (vista previa con errores por fila) → confirmar. */
export default function Importar({ eid, onVerUnidades }) {
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();
  const [archivo, setArchivo] = useState(null);
  const [progreso, setProgreso] = useState(null);
  const [res, setRes] = useState(null);
  const [soloObs, setSoloObs] = useState(false);
  const [revisado, setRevisado] = useState(false);
  const [confirmando, setConfirmando] = useState(false);
  const [hecho, setHecho] = useState(null);

  const paso = hecho ? 3 : res ? 2 : 1;

  const validar = async () => {
    if (!archivo) return;
    const form = new FormData();
    form.set('archivo', archivo);
    setProgreso(0);
    try {
      const r = await subir(`/edificios/${eid}/importaciones`, form, { onProgreso: setProgreso });
      setRes(r);
      setRevisado(false);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo leer el Excel', text: err.message });
    } finally {
      setProgreso(null);
    }
  };

  const confirmar = async () => {
    setConfirmando(true);
    try {
      const r = await api.post(`/edificios/${eid}/importaciones/${res.importacion_id}/confirmar`, {});
      setHecho(r || {});
      toast('Padrón importado.', { tipo: 'exito' });
    } catch (err) {
      await dialog.alert({ title: 'No se importó nada', text: `${err.message}\n\nLa importación es todo o nada: no se creó ninguna unidad.` });
    } finally {
      setConfirmando(false);
    }
  };

  const reiniciar = () => {
    setArchivo(null);
    setRes(null);
    setHecho(null);
  };

  const errores = res?.errores || [];
  const advertencias = res?.advertencias || [];
  const suma = res ? revisarSuma([res.suma_participacion_pct ?? '0']) : null;
  const obsPorFila = new Map();
  for (const e of errores) obsPorFila.set(e.fila, { tipo: 'error', texto: `${e.campo ? e.campo + ': ' : ''}${e.mensaje}` });
  for (const a of advertencias) if (!obsPorFila.has(a.fila)) obsPorFila.set(a.fila, { tipo: 'aviso', texto: a.mensaje });
  const vista = (res?.vista_previa || []).filter((f) => !soloObs || obsPorFila.has(f.fila));
  const bloqueo = errores.length ? `Hay ${errores.length} error(es) que bloquean: corrige el Excel y súbelo otra vez.` : suma && !suma.ok ? `${suma.mensaje}. Las participaciones deben sumar 100 %.` : advertencias.length && !revisado ? 'Marca que revisaste las observaciones.' : null;

  return (
    <div className="flex flex-col gap-4 lg:gap-6">
      {dialogEl}
      <ol className="flex flex-wrap items-center gap-3 text-sm" aria-label="Pasos de la importación">
        {['Subir archivo', 'Validar', 'Confirmar'].map((t, i) => (
          <li key={t} className="flex items-center gap-3">
            {i > 0 && <span className="h-0.5 w-8 bg-borde-fuerte sm:w-12" aria-hidden="true" />}
            <span className={`flex items-center gap-2 font-semibold ${paso > i ? 'text-acento' : 'text-texto-apoyo'}`} aria-current={paso === i + 1 ? 'step' : undefined}>
              <span className={`flex h-6 w-6 items-center justify-center rounded-full text-xs ${paso > i ? 'bg-acento text-white' : 'border border-borde-fuerte'}`}>{i + 1}</span>
              {t}
            </span>
          </li>
        ))}
        {res && (
          <li className="text-texto-suave sm:ml-auto">
            {res.archivo || archivo?.name} · hoja «{res.hoja || 'Padron'}» · {res.filas} filas
          </li>
        )}
      </ol>

      {hecho ? (
        <div className="rounded-tarjeta border border-acento-borde bg-acento-suave">
          <Vacio titulo="Padrón importado" icono="check" texto={`${hecho.unidades_creadas ?? 0} unidades creadas, ${hecho.unidades_actualizadas ?? 0} actualizadas, ${hecho.personas_creadas ?? 0} personas y ${hecho.deudas_cargadas ?? 0} deudas iniciales.`}>
            <Boton onClick={onVerUnidades}>Ver las unidades</Boton>
            <Boton variante="secundario" onClick={reiniciar}>
              Importar otro archivo
            </Boton>
          </Vacio>
        </div>
      ) : !res ? (
        <div className="flex max-w-xl flex-col gap-4 rounded-tarjeta border border-borde bg-superficie p-4 lg:p-6">
          <p className="text-base text-texto-suave">
            Usa la plantilla (hoja <b>Padron</b>, y opcional <b>Deuda</b>). Reimportar el mismo archivo no duplica: cada unidad se identifica por su código.
          </p>
          <Boton variante="fantasma" icono="descargar" href={urlApi(`/edificios/${eid}/importaciones/plantilla.xlsx`)} className="self-start">
            Descargar plantilla Excel
          </Boton>
          <SubirArchivo etiqueta="Excel del padrón" ayuda=".xlsx, hasta 10 MB" aceptar=".xlsx,.xls,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" tipos={TIPOS_XLSX} extensiones={['.xlsx', '.xls']} archivo={archivo} onArchivo={setArchivo} />
          {progreso != null && (
            <div className="h-2 rounded-full bg-superficie-2" role="progressbar" aria-valuenow={progreso} aria-valuemin={0} aria-valuemax={100} aria-label="Subiendo y validando">
              <div className="h-2 rounded-full bg-acento transition-[width] duration-media" style={{ width: `${progreso}%` }} />
            </div>
          )}
          <Boton onClick={validar} disabled={!archivo} cargando={progreso != null} icono="subir" className="self-start">
            Subir y validar
          </Boton>
        </div>
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
            <TarjetaKPI titulo="Filas leídas" valor={res.filas} nota={`${res.validas ?? res.filas} válidas`} tamValor="text-3xl" />
            <TarjetaKPI tono={suma.ok ? 'acento' : 'alerta'} titulo="Suma de participaciones" valor={suma.suma} nota={suma.ok ? 'Cuadra. Se puede prorratear.' : suma.mensaje} tamValor="text-3xl" />
            <TarjetaKPI tono={advertencias.length ? 'aviso' : 'neutro'} titulo="Observaciones" valor={advertencias.length} nota={advertencias.length ? advertencias.slice(0, 2).map((a) => `Fila ${a.fila}`).join(' y ') : 'Ninguna'} tamValor="text-3xl" />
            <TarjetaKPI tono={errores.length ? 'alerta' : 'neutro'} titulo="Errores que bloquean" valor={errores.length} nota={errores.length ? 'Corrige y vuelve a subir' : 'Correo, celular y DNI con formato válido'} tamValor="text-3xl" />
          </div>

          <div className="overflow-hidden rounded-tarjeta border border-borde bg-superficie">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[720px] text-sm">
                <thead>
                  <tr className="bg-fondo text-left text-xs text-texto-apoyo">
                    {['Fila', 'Unidad', 'Tipo', 'Propietario', 'DNI / RUC', 'Inquilino', 'Participación', 'Validación'].map((t) => (
                      <th key={t} className={`px-4 py-3 font-semibold ${t === 'Participación' ? 'text-right' : ''}`}>
                        {t}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {vista.map((f) => {
                    const o = obsPorFila.get(f.fila);
                    return (
                      <tr key={f.fila} className={`border-t border-superficie-2 ${o?.tipo === 'error' ? 'bg-alerta-suave' : o ? 'bg-aviso-suave' : ''}`}>
                        <td className="px-4 py-3 text-texto-apoyo">{f.fila}</td>
                        <td className="px-4 py-3 font-semibold">Dpto {f.codigo}</td>
                        <td className="px-4 py-3">{f.tipo}</td>
                        <td className="px-4 py-3">{f.propietario_nombre}</td>
                        <td className="px-4 py-3 tabular-nums">{enmascararDni(f.propietario_dni_ruc)}</td>
                        <td className="px-4 py-3">{f.inquilino_nombre || <span className="text-texto-apoyo">—</span>}</td>
                        <td className="px-4 py-3 text-right tabular-nums">{formatearPct(f.participacion_pct, 2)}</td>
                        <td className={`px-4 py-3 ${o?.tipo === 'error' ? 'font-semibold text-alerta' : o ? 'text-aviso' : 'text-acento'}`}>{o ? o.texto : 'Correcta'}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            <div className="flex flex-wrap items-center gap-2 border-t border-borde px-4 py-3 text-sm text-texto-apoyo">
              Mostrando {vista.length} de {res.vista_previa?.length ?? 0} filas ·
              <button type="button" className="font-semibold text-acento underline" onClick={() => setSoloObs(!soloObs)}>
                {soloObs ? 'Ver todas' : 'Ver solo observaciones'}
              </button>
            </div>
          </div>

          <div className="flex flex-col gap-4 rounded-tarjeta border border-borde bg-superficie p-4 lg:flex-row lg:items-center lg:justify-between lg:p-5">
            {advertencias.length > 0 ? (
              <label className="flex min-h-[44px] items-center gap-3 text-sm">
                <input type="checkbox" checked={revisado} onChange={(e) => setRevisado(e.target.checked)} className="h-5 w-5 accent-[var(--color-acento)]" />
                Revisé las observaciones (un DNI repetido puede ser la misma persona con dos unidades)
              </label>
            ) : (
              <span className="text-sm text-texto-suave">Sin observaciones.</span>
            )}
            <div className="flex flex-col gap-2 sm:flex-row">
              <Boton variante="secundario" onClick={reiniciar}>
                Subir otro archivo
              </Boton>
              <Boton onClick={confirmar} disabled={!!bloqueo} cargando={confirmando}>
                Importar {res.validas ?? res.filas} unidades
              </Boton>
            </div>
          </div>
          {bloqueo && (
            <p className="text-sm font-semibold text-alerta" role="status">
              {bloqueo}
            </p>
          )}
        </>
      )}
    </div>
  );
}
