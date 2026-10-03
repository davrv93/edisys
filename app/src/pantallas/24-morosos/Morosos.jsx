import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha, formatearFechaHora, nombrePeriodo } from '../../lib/fechas.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Modal, Tabla, Vacio, useDialog, useToast } from '../../ui/index.js';

// Color de cada celda de la grilla (tokens del sistema; el texto siempre acompaña al color).
const CELDA = {
  puntual: 'bg-acento-suave text-acento',
  en_gracia: 'bg-acento-suave text-acento',
  tardio: 'bg-aviso-suave text-aviso-texto',
  moroso: 'bg-alerta-suave text-alerta',
  pendiente: 'bg-superficie-2 text-texto-suave',
};

/** Bloques D3 (morosos, detallado, puntualidad) y D2 (avisos de cobranza). */
export default function Morosos() {
  const s = useSesion();
  const avisos = s.tiene('avisos.gestionar');
  const configura = s.tiene('acuerdos.gestionar');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [vista, setVista] = useState('morosos');
  const [config, setConfig] = useState(null);
  const eid = useEid();
  const [desde, setDesde] = useState('');
  const [hasta, setHasta] = useState('');
  // Solo se piden periodos completos (AAAA-MM); mientras se escribe, se mantiene el rango anterior.
  const per = (p) => (/^\d{4}-(0[1-9]|1[0-2])$/.test(p) ? p : '');
  const g = useCarga(() => api.get(`/edificios/${eid}/morosos`, { desde: per(desde), hasta: per(hasta) }), [eid, per(desde), per(hasta)]);
  const et = g.datos?.etiquetas || { moroso: 'Moroso', puntual: 'Puntual' };

  const abrirConfig = async () => {
    try {
      setConfig(await api.get(`/edificios/${eid}/deuda/config`));
    } catch (err) {
      await dialog.alert({ title: 'No se pudo abrir', text: err.message });
    }
  };
  const guardarConfig = async () => {
    try {
      await api.put(`/edificios/${eid}/deuda/config`, { ...config, tasa_mora_bp: Math.round(Number(config.tasa_pct ?? config.tasa_mora_bp / 100) * 100) });
      toast('Configuración guardada.', { tipo: 'exito' });
      setConfig(null);
      await g.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo guardar', text: err.message });
    }
  };

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo={`${et.moroso}s y puntualidad`}
        subtitulo="De un vistazo, quién paga a tiempo y quién no"
        ayuda="Cada celda es el recibo del mes: puntual si se pagó completo al vencimiento, tardío si se pagó después de la gracia (con la fecha), y moroso si sigue sin pagar pasada la gracia."
        acciones={configura && <Boton variante="secundario" icono="engranaje" onClick={abrirConfig}>Configurar</Boton>}
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'morosos'} icono="moroso" onClick={() => setVista('morosos')}>{et.moroso}s</Chip>
          <Chip activo={vista === 'detallado'} icono="recibo" onClick={() => setVista('detallado')}>{et.moroso}s detallado</Chip>
          <Chip activo={vista === 'puntualidad'} icono="hecho" onClick={() => setVista('puntualidad')}>Puntualidad</Chip>
          {avisos && <Chip activo={vista === 'avisos'} icono="correo" onClick={() => setVista('avisos')}>Avisos de cobranza</Chip>}
        </div>

        {vista === 'avisos' ? (
          <VistaAvisos dialog={dialog} toast={toast} />
        ) : (
          <>
            <div className="flex flex-wrap items-end gap-3">
              <Campo etiqueta="Desde (AAAA-MM)" className="w-40" valor={desde} onCambio={setDesde} placeholder="2026-04" />
              <Campo etiqueta="Hasta (AAAA-MM)" className="w-40" valor={hasta} onCambio={setHasta} placeholder="2026-09" />
            </div>
            {g.error ? (
              <ErrorCarga error={g.error} onReintentar={g.recargar} />
            ) : !g.datos ? (
              <Esqueleto className="h-64 w-full" />
            ) : (
              <Grilla datos={g.datos} vista={vista} et={et} />
            )}
          </>
        )}
      </Contenido>

      <Modal abierto={!!config} onCerrar={() => setConfig(null)} titulo="Configuración de deuda" ancho="max-w-sm"
        pie={<><Boton variante="fantasma" onClick={() => setConfig(null)}>Cancelar</Boton><Boton onClick={guardarConfig}>Guardar</Boton></>}>
        {config && (
          <div className="flex flex-col gap-3">
            <Campo etiqueta="Cómo llamar al moroso" valor={config.etiqueta_moroso} onCambio={(v) => setConfig({ ...config, etiqueta_moroso: v })} ayuda="Algunos edificios prefieren «Deudor» o «Pendiente»." />
            <Campo etiqueta="Cómo llamar al puntual" valor={config.etiqueta_puntual} onCambio={(v) => setConfig({ ...config, etiqueta_puntual: v })} />
            <Campo etiqueta="Tasa de mora mensual (%)" tipo="numero" valor={config.tasa_pct ?? String(config.tasa_mora_bp / 100)} onCambio={(v) => setConfig({ ...config, tasa_pct: v })}
              ayuda="Sugiere el recargo de un acuerdo de pago. 0 = sin mora." />
          </div>
        )}
      </Modal>
    </>
  );
}

function Grilla({ datos, vista, et }) {
  const meses = datos.meses || [];
  const unidades = datos.unidades || [];
  if (!meses.length) return <Vacio icono="calendario" titulo="Sin periodos" texto="No hay periodos emitidos en el rango." />;

  const texto = (c) => {
    if (!c) return '—';
    if (vista === 'puntualidad') return c.estado === 'puntual' ? et.puntual : c.estado === 'pendiente' ? 'En plazo' : 'Fuera de plazo';
    if (vista === 'detallado') {
      if (c.estado === 'moroso') return `${formatearSoles(c.saldo_cts)} · ${c.dias_atraso} d`;
      if (c.estado === 'tardio') return `Pagó ${formatearFecha(c.pagado_en)}`;
      if (c.estado === 'pendiente') return `Vence ${formatearFecha(c.vence)}`;
      return c.pagado_en ? formatearFecha(c.pagado_en) : 'Al día';
    }
    return { moroso: et.moroso, tardio: 'Tardío', pendiente: 'En plazo' }[c.estado] || 'Al día';
  };
  const clase = (c) => {
    if (!c) return 'text-texto-apoyo';
    if (vista === 'puntualidad') return c.estado === 'puntual' ? CELDA.puntual : c.estado === 'pendiente' ? CELDA.pendiente : CELDA.moroso;
    return CELDA[c.estado] || '';
  };
  const resumen = datos.por_mes || {};

  return (
    <Seccion titulo={vista === 'puntualidad' ? 'Puntualidad por mes' : vista === 'detallado' ? `${et.moroso}s detallado` : `${et.moroso}s por mes`} padding="p-0">
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-xs" aria-label="Grilla de morosidad">
          <thead>
            <tr className="border-b border-borde bg-fondo text-left text-texto-apoyo">
              <th scope="col" className="sticky left-0 bg-fondo px-3 py-2 font-semibold">Dpto</th>
              {meses.map((m) => (
                <th key={m} scope="col" className="px-2 py-2 text-center font-semibold">{nombrePeriodo(m, { corto: true })}</th>
              ))}
              <th scope="col" className="px-3 py-2 text-right font-semibold">{vista === 'puntualidad' ? '% puntual' : `Meses ${et.moroso.toLowerCase()}`}</th>
            </tr>
          </thead>
          <tbody>
            {unidades.map((u) => (
              <tr key={u.unidad_id} className="border-b border-borde">
                <th scope="row" className="sticky left-0 bg-superficie px-3 py-1.5 text-left font-medium text-tinta" title={u.propietario}>{u.unidad}</th>
                {meses.map((m) => {
                  const c = u.celdas?.[m];
                  return (
                    <td key={m} className="px-1 py-1 text-center">
                      <span className={`block whitespace-nowrap rounded-control px-1.5 py-1 ${clase(c)}`}>{texto(c)}</span>
                    </td>
                  );
                })}
                <td className="px-3 py-1.5 text-right tabular-nums">{vista === 'puntualidad' ? `${u.puntualidad_pct} %` : u.meses_morosos}</td>
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr className="bg-fondo text-texto-apoyo">
              <th scope="row" className="sticky left-0 bg-fondo px-3 py-2 text-left font-semibold">Total</th>
              {meses.map((m) => (
                <td key={m} className="px-2 py-2 text-center tabular-nums">
                  {vista === 'puntualidad' ? `${resumen[m]?.puntuales ?? 0}/${resumen[m]?.recibos ?? 0}` : vista === 'detallado' ? formatearSoles(resumen[m]?.moroso_cts ?? 0) : resumen[m]?.morosos ?? 0}
                </td>
              ))}
              <td />
            </tr>
          </tfoot>
        </table>
      </div>
    </Seccion>
  );
}

const ESCALA_VACIA = { desde_cts: null, hasta_cts: null, asunto: '', cuerpo: 'Hola {{nombre}}, el Dpto {{unidad}} tiene una deuda vencida de {{deuda}}.' };
const AVISO_VACIO = { nombre: '', frecuencia: 'mensual', dia: '15', tipo_escala: 'monto', canales: ['correo'], adjunta_pdf: false, activo: true, escalas: [{ ...ESCALA_VACIA }] };
const FRECUENCIA = { diaria: 'Cada día', semanal: 'Cada semana', mensual: 'Cada mes' };

/** D2 · Avisos de cobranza: automatizados por escalas, aviso manual y evidencia de envíos. */
function VistaAvisos({ dialog, toast }) {
  const eid = useEid();
  const avs = useCarga(() => api.get(`/edificios/${eid}/avisos-cobranza`), [eid]);
  const env = useCarga(() => api.get(`/edificios/${eid}/avisos-cobranza/envios`), [eid]);
  const [form, setForm] = useState(null); // aviso en edición (id opcional)
  const [manual, setManual] = useState(null);
  const [ocupado, setOcupado] = useState(false);

  // En el formulario, los tramos por monto van en soles (céntimos); por recibos, en número.
  const aForm = (a) => ({
    ...a,
    dia: String(a.dia),
    escalas: a.escalas.map((x) => ({ ...x, desde_cts: x.desde, hasta_cts: x.hasta, desde_n: String(x.desde), hasta_n: x.hasta == null ? '' : String(x.hasta) })),
  });
  const aAPI = (f) => ({
    ...f,
    dia: Number(f.dia),
    escalas: f.escalas.map((x) => (f.tipo_escala === 'monto'
      ? { desde: x.desde_cts || 0, hasta: x.hasta_cts ?? null, asunto: x.asunto, cuerpo: x.cuerpo }
      : { desde: Number(x.desde_n || 0), hasta: x.hasta_n === '' || x.hasta_n == null ? null : Number(x.hasta_n), asunto: x.asunto, cuerpo: x.cuerpo })),
  });

  const guardar = async () => {
    setOcupado(true);
    try {
      if (form.id) await api.put(`/edificios/${eid}/avisos-cobranza/${form.id}`, aAPI(form));
      else await api.post(`/edificios/${eid}/avisos-cobranza`, aAPI(form));
      toast('Aviso guardado.', { tipo: 'exito' });
      setForm(null);
      await avs.recargar();
    } catch (err) {
      const campos = Object.values(err.campos || {}).join(' ');
      await dialog.alert({ title: 'No se pudo guardar el aviso', text: campos || err.message });
    } finally {
      setOcupado(false);
    }
  };

  const ejecutar = async (a) => {
    const ok = await dialog.confirm({ title: `¿Enviar «${a.nombre}» ahora?`, text: 'Se avisa a todas las unidades con deuda vencida según sus tramos. Hoy no se repite a quien ya se le avisó.' });
    if (!ok) return;
    try {
      const r = await api.post(`/edificios/${eid}/avisos-cobranza/${a.id}/ejecutar`, {});
      toast(`${r.encolados} aviso(s) en la bandeja para ${r.deudores} unidad(es).`, { tipo: 'exito' });
      await Promise.all([avs.recargar(), env.recargar()]);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo enviar', text: err.message });
    }
  };

  const borrar = async (a) => {
    const ok = await dialog.confirm({ title: `¿Eliminar «${a.nombre}»?`, text: 'La evidencia de los envíos se conserva.', danger: true });
    if (!ok) return;
    try {
      await api.del(`/edificios/${eid}/avisos-cobranza/${a.id}`);
      await avs.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo eliminar', text: err.message });
    }
  };

  const enviarManual = async () => {
    setOcupado(true);
    try {
      let ids = [];
      const cods = manual.unidades.split(/[\s,;]+/).filter(Boolean);
      if (cods.length) {
        const r = await api.get(`/edificios/${eid}/unidades`, { por_pagina: 500 });
        const porCod = new Map(lista(r).map((u) => [String(u.codigo), u.id]));
        const faltan = cods.filter((c) => !porCod.has(c));
        if (faltan.length) throw new Error(`No encontré: ${faltan.join(', ')}.`);
        ids = cods.map((c) => porCod.get(c));
      }
      const r = await api.post(`/edificios/${eid}/avisos-cobranza/manual`, { unidad_ids: ids, canales: manual.canales, asunto: manual.asunto, cuerpo: manual.cuerpo, adjunta_pdf: manual.adjunta_pdf });
      toast(`${r.encolados} aviso(s) en la bandeja para ${r.deudores} unidad(es) con deuda vencida.`, { tipo: 'exito' });
      setManual(null);
      await env.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo enviar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const canalesChips = (valor, onCambio) => (
    <div className="flex flex-wrap gap-2">
      {[['correo', 'Correo'], ['whatsapp', 'WhatsApp']].map(([c, t]) => (
        <Chip key={c} activo={valor.includes(c)} onClick={() => onCambio(valor.includes(c) ? valor.filter((x) => x !== c) : [...valor, c])}>{t}</Chip>
      ))}
    </div>
  );
  const setEscala = (i, cambio) => setForm({ ...form, escalas: form.escalas.map((x, j) => (j === i ? { ...x, ...cambio } : x)) });

  const listaAvs = lista(avs.datos);
  return (
    <>
      <div className="flex flex-wrap gap-2">
        <Boton icono="mas_signo" onClick={() => setForm({ ...AVISO_VACIO, escalas: [{ ...ESCALA_VACIA }] })}>Aviso automatizado</Boton>
        <Boton variante="secundario" icono="enviar" onClick={() => setManual({ unidades: '', canales: ['correo'], asunto: 'Recordatorio de pago', cuerpo: 'Hola {{nombre}}, el Dpto {{unidad}} tiene una deuda vencida de {{deuda}}. Si ya pagaste, no tomes en cuenta este mensaje.', adjunta_pdf: false })}>Aviso manual</Boton>
      </div>

      <Seccion titulo="Avisos automatizados">
        {avs.error ? (
          <ErrorCarga error={avs.error} onReintentar={avs.recargar} />
        ) : !avs.datos ? (
          <Esqueleto className="h-24 w-full" />
        ) : listaAvs.length === 0 ? (
          <Vacio icono="correo" titulo="Sin avisos programados" texto="Programa avisos por escalas: uno suave para deudas pequeñas y otro firme para las grandes." compacto />
        ) : (
          <ul className="divide-y divide-borde">
            {listaAvs.map((a) => (
              <li key={a.id} className="flex flex-wrap items-center gap-3 py-2.5">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium text-tinta">{a.nombre}</p>
                  <p className="truncate text-xs text-texto-apoyo">
                    {FRECUENCIA[a.frecuencia]}{a.frecuencia !== 'diaria' && ` · día ${a.dia}`} · {a.escalas.length} tramo(s) por {a.tipo_escala === 'monto' ? 'deuda' : 'recibos'} · {a.canales.join(' y ')}
                    {a.ultima_ejecucion && ` · último ${formatearFecha(a.ultima_ejecucion)}`}
                  </p>
                </div>
                <Insignia estado={a.activo ? 'pendiente' : 'anulado'} tono={a.activo ? 'curso' : 'hecho'} texto={a.activo ? 'Activo' : 'Pausado'} />
                <Boton tamano="sm" variante="secundario" onClick={() => ejecutar(a)}>Enviar ahora</Boton>
                <Boton tamano="sm" variante="fantasma" onClick={() => setForm(aForm(a))}>Editar</Boton>
                <Boton tamano="sm" variante="fantasma" className="!text-alerta" onClick={() => borrar(a)}>Eliminar</Boton>
              </li>
            ))}
          </ul>
        )}
      </Seccion>

      <Seccion titulo="Evidencia de envíos" padding="p-0">
        <Tabla
          etiqueta="Envíos de avisos"
          densa
          filas={lista(env.datos)}
          cargando={env.cargando}
          error={env.error}
          onReintentar={env.recargar}
          columnas={[
            { clave: 'creado_en', titulo: 'Fecha', movil: 'sub', render: (v) => formatearFechaHora(v.creado_en) },
            { clave: 'unidad', titulo: 'Unidad', movil: 'titulo', render: (v) => `Dpto ${v.unidad}` },
            { clave: 'aviso', titulo: 'Aviso', prioridad: 2 },
            { clave: 'asunto', titulo: 'Asunto', prioridad: 3 },
            { clave: 'canal', titulo: 'Canal' },
            { clave: 'deuda_cts', titulo: 'Deuda', alinear: 'der', movil: 'valor', render: (v) => formatearSoles(v.deuda_cts) },
            {
              clave: 'estado', titulo: 'Estado', movil: 'valor2',
              render: (v) => (v.estado === 'encolado'
                ? <Insignia estado={v.estado_bandeja === 'enviado' ? 'pagado' : 'pendiente'} texto={v.estado_bandeja === 'enviado' ? 'Enviado' : v.estado_bandeja === 'simulado' ? 'Simulado' : 'En bandeja'} />
                : <Insignia estado="vencido" texto={v.estado === 'sin_contacto' ? 'Sin contacto' : 'Error'} />),
            },
          ]}
          vacio={<Vacio icono="bandeja" titulo="Sin envíos" texto="Aquí queda la evidencia de cada aviso enviado." compacto />}
        />
      </Seccion>

      <Modal abierto={!!form} onCerrar={() => setForm(null)} titulo={form?.id ? 'Editar aviso automatizado' : 'Nuevo aviso automatizado'} ancho="max-w-2xl"
        pie={<><Boton variante="fantasma" onClick={() => setForm(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardar}>Guardar</Boton></>}>
        {form && (
          <div className="flex flex-col gap-3">
            <Campo etiqueta="Nombre" valor={form.nombre} onCambio={(v) => setForm({ ...form, nombre: v })} />
            <div className="grid grid-cols-3 gap-3">
              <Campo etiqueta="Frecuencia" tipo="select" valor={form.frecuencia} onCambio={(v) => setForm({ ...form, frecuencia: v })}
                opciones={Object.entries(FRECUENCIA).map(([valor, etiqueta]) => ({ valor, etiqueta }))} />
              <Campo etiqueta={form.frecuencia === 'semanal' ? 'Día (1 lun … 7 dom)' : 'Día del mes'} tipo="numero" valor={form.dia} disabled={form.frecuencia === 'diaria'} onCambio={(v) => setForm({ ...form, dia: v })} />
              <Campo etiqueta="Escala por" tipo="select" valor={form.tipo_escala} onCambio={(v) => setForm({ ...form, tipo_escala: v })}
                opciones={[{ valor: 'monto', etiqueta: 'Deuda vencida' }, { valor: 'recibos', etiqueta: 'Recibos vencidos' }]} />
            </div>
            {canalesChips(form.canales, (c) => setForm({ ...form, canales: c }))}
            <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.adjunta_pdf} onChange={(e) => setForm({ ...form, adjunta_pdf: e.target.checked })} /> Adjuntar el PDF de los recibos vencidos (correo)</label>
            <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.activo} onChange={(e) => setForm({ ...form, activo: e.target.checked })} /> Activo</label>
            <p className="text-xs text-texto-apoyo">Variables: {'{{nombre}}'}, {'{{unidad}}'}, {'{{deuda}}'}, {'{{recibos}}'}, {'{{edificio}}'}. Deja «hasta» vacío en el último tramo para que no tenga tope.</p>
            {form.escalas.map((x, i) => (
              <div key={i} className="flex flex-col gap-2 rounded-control border border-borde p-3">
                <div className="grid grid-cols-2 gap-3">
                  {form.tipo_escala === 'monto' ? (
                    <>
                      <Campo etiqueta="Desde" tipo="dinero" valor={x.desde_cts} onCambio={(v) => setEscala(i, { desde_cts: v })} />
                      <Campo etiqueta="Hasta" tipo="dinero" valor={x.hasta_cts} onCambio={(v) => setEscala(i, { hasta_cts: v })} />
                    </>
                  ) : (
                    <>
                      <Campo etiqueta="Desde (recibos)" tipo="numero" valor={x.desde_n ?? ''} onCambio={(v) => setEscala(i, { desde_n: v })} />
                      <Campo etiqueta="Hasta (recibos)" tipo="numero" valor={x.hasta_n ?? ''} onCambio={(v) => setEscala(i, { hasta_n: v })} />
                    </>
                  )}
                </div>
                <Campo etiqueta="Asunto" valor={x.asunto} onCambio={(v) => setEscala(i, { asunto: v })} />
                <Campo etiqueta="Mensaje" tipo="textarea" valor={x.cuerpo} onCambio={(v) => setEscala(i, { cuerpo: v })} />
                {form.escalas.length > 1 && (
                  <Boton tamano="sm" variante="fantasma" className="self-start !text-alerta" onClick={() => setForm({ ...form, escalas: form.escalas.filter((_, j) => j !== i) })}>Quitar tramo</Boton>
                )}
              </div>
            ))}
            <Boton variante="secundario" icono="mas_signo" className="self-start" onClick={() => setForm({ ...form, escalas: [...form.escalas, { ...ESCALA_VACIA }] })}>Agregar tramo</Boton>
          </div>
        )}
      </Modal>

      <Modal abierto={!!manual} onCerrar={() => setManual(null)} titulo="Aviso manual" ancho="max-w-lg"
        pie={<><Boton variante="fantasma" onClick={() => setManual(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={enviarManual}>Enviar ahora</Boton></>}>
        {manual && (
          <div className="flex flex-col gap-3">
            <Campo etiqueta="Unidades" valor={manual.unidades} onCambio={(v) => setManual({ ...manual, unidades: v })} ayuda="Códigos separados por coma (402, 503). Vacío = todas las unidades con deuda vencida." />
            {canalesChips(manual.canales, (c) => setManual({ ...manual, canales: c }))}
            <Campo etiqueta="Asunto" valor={manual.asunto} onCambio={(v) => setManual({ ...manual, asunto: v })} />
            <Campo etiqueta="Mensaje" tipo="textarea" valor={manual.cuerpo} onCambio={(v) => setManual({ ...manual, cuerpo: v })} />
            <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={manual.adjunta_pdf} onChange={(e) => setManual({ ...manual, adjunta_pdf: e.target.checked })} /> Adjuntar el PDF de los recibos vencidos (correo)</label>
          </div>
        )}
      </Modal>
    </>
  );
}
