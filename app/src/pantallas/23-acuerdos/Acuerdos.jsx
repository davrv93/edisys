import { useMemo, useState } from 'react';
import { api, lista, subir } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha, nombrePeriodo } from '../../lib/fechas.js';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, Insignia, Modal, SubirArchivo, Tabla, Vacio, useDialog, useToast } from '../../ui/index.js';

// Estado visible del acuerdo → insignia (el API deriva «cumplido» y «vencido» de las cuotas).
const ESTADO = {
  activo: { estado: 'pendiente', tono: 'curso', texto: 'Activo' },
  vencido: { estado: 'vencido', texto: 'Cuotas vencidas' },
  cumplido: { estado: 'pagado', texto: 'Cumplido' },
  anulado: { estado: 'anulado', texto: 'Anulado' },
};
const ESTADO_CUOTA = { pagado: 'pagado', parcial: 'parcial', vencido: 'vencido', pendiente: 'pendiente' };

/** Divide como el API (resto de a un céntimo en las primeras) para la vista previa. */
function previaCuotas(monto, n) {
  if (!(monto > 0) || !(n > 0)) return [];
  const base = Math.floor(monto / n);
  const resto = monto % n;
  return Array.from({ length: n }, (_, i) => base + (i < resto ? 1 : 0));
}

/** Bloque D1 · Acuerdos de pago (financiamiento de deuda vencida en cuotas). */
export default function Acuerdos() {
  const eid = useEid();
  const s = useSesion();
  const gestiona = s.tiene('acuerdos.gestionar');
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [filtro, setFiltro] = useState('activo');
  const [nuevo, setNuevo] = useState(false);
  const [detalle, setDetalle] = useState(null); // id

  const acs = useCarga(() => api.get(`/edificios/${eid}/acuerdos`, { estado: filtro === 'todos' ? '' : filtro }), [eid, filtro]);
  const filas = lista(acs.datos);

  const columnas = [
    { clave: 'numero', titulo: 'Acuerdo', movil: 'titulo', render: (a) => `${a.numero} · Dpto ${a.unidad}` },
    { clave: 'fecha', titulo: 'Fecha', movil: 'sub', render: (a) => formatearFecha(a.fecha) },
    { clave: 'aceptado_por', titulo: 'Deudor', prioridad: 2 },
    { clave: 'monto_acordado_cts', titulo: 'Acordado', alinear: 'der', movil: 'valor', render: (a) => formatearSoles(a.monto_acordado_cts) },
    { clave: 'n_cuotas', titulo: 'Cuotas', alinear: 'der', prioridad: 2 },
    { clave: 'avance_cts', titulo: 'Avance', alinear: 'der', prioridad: 2, render: (a) => formatearSoles(a.avance_cts) },
    { clave: 'estado_visible', titulo: 'Estado', movil: 'valor2', render: (a) => <Insignia {...(ESTADO[a.estado_visible] || ESTADO.activo)} /> },
  ];

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Acuerdos de pago"
        subtitulo="Financia la deuda vencida en cuotas"
        ayuda="El acuerdo toma recibos vencidos, aplica recargo o descuento y divide el monto en cuotas mensuales. Mientras las cuotas estén al día la unidad no figura como morosa y puede reservar. Sube el documento firmado por el presidente de la junta y el deudor."
        acciones={gestiona && <Boton icono="mas_signo" onClick={() => setNuevo(true)}>Nuevo acuerdo</Boton>}
      />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          {[['activo', 'Activos'], ['anulado', 'Anulados'], ['todos', 'Todos']].map(([v, t]) => (
            <Chip key={v} activo={filtro === v} onClick={() => setFiltro(v)}>{t}</Chip>
          ))}
        </div>
        <Seccion titulo="Acuerdos" padding="p-0">
          <Tabla
            etiqueta="Acuerdos de pago"
            columnas={columnas}
            filas={filas}
            cargando={acs.cargando}
            error={acs.error}
            onReintentar={acs.recargar}
            onFila={(a) => setDetalle(a.id)}
            vacio={<Vacio icono="recibo" titulo="Sin acuerdos" texto="Cuando un propietario negocie su deuda, regístralo aquí." />}
          />
        </Seccion>
      </Contenido>

      {nuevo && (
        <NuevoAcuerdo
          onCerrar={() => setNuevo(false)}
          onCreado={async (a) => {
            setNuevo(false);
            toast(`Acuerdo ${a.numero} registrado.`, { tipo: 'exito' });
            await acs.recargar();
            setDetalle(a.id);
          }}
          dialog={dialog}
        />
      )}
      {detalle && <DetalleAcuerdo id={detalle} gestiona={gestiona} dialog={dialog} toast={toast} onCerrar={() => setDetalle(null)} onCambio={acs.recargar} />}
    </>
  );
}

function NuevoAcuerdo({ onCerrar, onCreado, dialog }) {
  const eid = useEid();
  const [unidad, setUnidad] = useState('');
  const [marcados, setMarcados] = useState(null); // Set de recibo_id; null = todos
  const [form, setForm] = useState({ recargo_cts: 0, descuento_cts: 0, n_cuotas: '6', primera_cuota: '', aceptado_por: '', comentario: '' });
  const [ocupado, setOcupado] = useState(false);

  // Solo se ofrecen unidades con deuda (cuentas por cobrar).
  const cxc = useCarga(() => api.get(`/edificios/${eid}/cuentas-por-cobrar`), [eid]);
  const opciones = [{ valor: '', etiqueta: 'Elige la unidad…' }, ...lista(cxc.datos).filter((u) => !u.acuerdo || u.deuda_vencida_cts > 0).map((u) => ({ valor: String(u.unidad_id), etiqueta: `Dpto ${u.unidad} · ${formatearSoles(u.deuda_cts)}` }))];
  const pr = useCarga(() => (unidad ? api.get(`/edificios/${eid}/acuerdos/propuesta`, { unidad_id: unidad }) : Promise.resolve(null)), [eid, unidad]);
  const recibos = lista(pr.datos, 'recibos');
  const elegidos = recibos.filter((r) => !marcados || marcados.has(r.recibo_id));
  const saldo = elegidos.reduce((a, r) => a + r.saldo_cts, 0);
  const mora = elegidos.reduce((a, r) => a + r.mora_cts, 0);
  const monto = saldo + (form.recargo_cts || 0) - (form.descuento_cts || 0);
  const cuotas = useMemo(() => previaCuotas(monto, Number(form.n_cuotas)), [monto, form.n_cuotas]);

  const alternar = (id) => {
    const base = marcados || new Set(recibos.map((r) => r.recibo_id));
    const n = new Set(base);
    if (n.has(id)) n.delete(id);
    else n.add(id);
    setMarcados(n);
  };

  const guardar = async () => {
    setOcupado(true);
    try {
      const a = await api.post(`/edificios/${eid}/acuerdos`, {
        unidad_id: Number(unidad),
        recibo_ids: elegidos.map((r) => r.recibo_id),
        recargo_cts: form.recargo_cts || 0,
        descuento_cts: form.descuento_cts || 0,
        n_cuotas: Number(form.n_cuotas),
        primera_cuota: form.primera_cuota,
        aceptado_por: form.aceptado_por,
        comentario: form.comentario,
      });
      await onCreado(a);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar el acuerdo', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  return (
    <Modal
      abierto
      onCerrar={onCerrar}
      titulo="Nuevo acuerdo de pago"
      ancho="max-w-2xl"
      pie={<><Boton variante="fantasma" onClick={onCerrar}>Cancelar</Boton><Boton cargando={ocupado} disabled={!unidad || !elegidos.length || monto <= 0} onClick={guardar}>Registrar acuerdo</Boton></>}
    >
      <div className="flex flex-col gap-4">
        <Campo etiqueta="Unidad" tipo="select" valor={unidad} onCambio={(v) => { setUnidad(v); setMarcados(null); }} opciones={opciones} />
        {unidad && (
          pr.cargando && !pr.datos ? (
            <p className="text-sm text-texto-apoyo">Cargando recibos vencidos…</p>
          ) : recibos.length === 0 ? (
            <Vacio icono="pagado" titulo="Sin recibos vencidos" texto="La unidad no tiene recibos vencidos libres (o ya están en otro acuerdo)." compacto />
          ) : (
            <div className="overflow-hidden rounded-control border border-borde">
              <table className="w-full text-sm">
                <thead className="bg-fondo text-left text-xs text-texto-apoyo">
                  <tr>
                    <th className="px-3 py-2" aria-label="Incluir" />
                    <th className="px-3 py-2">Recibo</th>
                    <th className="px-3 py-2 text-right">Total</th>
                    <th className="px-3 py-2 text-right">Saldo</th>
                    <th className="px-3 py-2 text-right">Mora calculada</th>
                  </tr>
                </thead>
                <tbody>
                  {recibos.map((r) => (
                    <tr key={r.recibo_id} className="border-t border-borde">
                      <td className="px-3 py-1.5">
                        <input type="checkbox" checked={!marcados || marcados.has(r.recibo_id)} onChange={() => alternar(r.recibo_id)} aria-label={`Incluir ${r.numero}`} />
                      </td>
                      <td className="px-3 py-1.5">{nombrePeriodo(r.periodo)} <span className="text-xs text-texto-apoyo">{r.numero}</span></td>
                      <td className="px-3 py-1.5 text-right tabular-nums">{formatearSoles(r.total_cts)}</td>
                      <td className="px-3 py-1.5 text-right tabular-nums">{formatearSoles(r.saldo_cts)}</td>
                      <td className="px-3 py-1.5 text-right tabular-nums">{formatearSoles(r.mora_cts)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        )}
        <div className="grid grid-cols-2 gap-3">
          <Campo etiqueta="Recargo" tipo="dinero" valor={form.recargo_cts} onCambio={(v) => setForm({ ...form, recargo_cts: v })}
            ayuda={mora > 0 ? `Mora calculada: ${formatearSoles(mora)}` : undefined} />
          <Campo etiqueta="Descuento" tipo="dinero" valor={form.descuento_cts} onCambio={(v) => setForm({ ...form, descuento_cts: v })} />
          <Campo etiqueta="Número de cuotas" tipo="numero" valor={form.n_cuotas} onCambio={(v) => setForm({ ...form, n_cuotas: v })} />
          <Campo etiqueta="Primera cuota" tipo="fecha" valor={form.primera_cuota} onCambio={(v) => setForm({ ...form, primera_cuota: v })} ayuda="Por defecto, en un mes." />
        </div>
        <Campo etiqueta="Acepta (deudor)" valor={form.aceptado_por} onCambio={(v) => setForm({ ...form, aceptado_por: v })} />
        <Campo etiqueta="Comentario" tipo="textarea" valor={form.comentario} onCambio={(v) => setForm({ ...form, comentario: v })} />
        <div className="rounded-control bg-superficie-2 p-3 text-sm">
          <p className="font-semibold text-tinta">Monto acordado: {formatearSoles(Math.max(monto, 0))}</p>
          <p className="text-texto-apoyo">
            Saldo {formatearSoles(saldo)} + recargo {formatearSoles(form.recargo_cts || 0)} − descuento {formatearSoles(form.descuento_cts || 0)}
            {cuotas.length > 0 && ` · ${cuotas.length} cuota(s) de ${formatearSoles(cuotas[cuotas.length - 1])}${cuotas[0] !== cuotas[cuotas.length - 1] ? ` a ${formatearSoles(cuotas[0])}` : ''}`}
          </p>
        </div>
      </div>
    </Modal>
  );
}

function DetalleAcuerdo({ id, gestiona, dialog, toast, onCerrar, onCambio }) {
  const eid = useEid();
  const ac = useCarga(() => api.get(`/edificios/${eid}/acuerdos/${id}`), [eid, id]);
  const a = ac.datos;
  const [archivo, setArchivo] = useState(null);
  const [ocupado, setOcupado] = useState(false);

  const anular = async () => {
    const motivo = await dialog.prompt({ title: `Anular ${a.numero}`, label: 'Motivo', text: 'Se quitan el recargo y el descuento y la deuda vuelve a contar como vencida.' });
    if (!motivo) return;
    try {
      await api.post(`/edificios/${eid}/acuerdos/${id}/anular`, { motivo });
      toast('Acuerdo anulado.', { tipo: 'exito' });
      await Promise.all([ac.recargar(), onCambio()]);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo anular', text: err.message });
    }
  };

  const subirDoc = async () => {
    setOcupado(true);
    try {
      const fd = new FormData();
      fd.set('archivo', archivo);
      await subir(`/edificios/${eid}/acuerdos/${id}/documento`, fd);
      toast('Documento firmado guardado.', { tipo: 'exito' });
      setArchivo(null);
      await ac.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo subir', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  return (
    <Modal
      abierto
      onCerrar={onCerrar}
      titulo={a ? `Acuerdo ${a.numero} · Dpto ${a.unidad}` : 'Acuerdo de pago'}
      ancho="max-w-2xl"
      pie={<>
        {gestiona && a?.estado === 'activo' && <Boton variante="fantasma" className="!text-alerta" onClick={anular}>Anular</Boton>}
        <Boton variante="secundario" onClick={onCerrar}>Cerrar</Boton>
      </>}
    >
      {!a ? (
        <p className="text-sm text-texto-apoyo">{ac.error ? ac.error.message : 'Cargando…'}</p>
      ) : (
        <div className="flex flex-col gap-4 text-sm">
          <div className="flex flex-wrap items-center gap-2">
            <Insignia {...(ESTADO[a.estado_visible] || ESTADO.activo)} />
            <span className="text-texto-apoyo">Firmado el {formatearFecha(a.fecha)} · acepta {a.aceptado_por}</span>
          </div>
          <dl className="grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-3">
            <div><dt className="text-xs text-texto-apoyo">Saldo</dt><dd>{formatearSoles(a.saldo_cts)}</dd></div>
            <div><dt className="text-xs text-texto-apoyo">Mora calculada</dt><dd>{formatearSoles(a.mora_cts)}</dd></div>
            <div><dt className="text-xs text-texto-apoyo">Recargo / descuento</dt><dd>{formatearSoles(a.recargo_cts)} / {formatearSoles(a.descuento_cts)}</dd></div>
            <div><dt className="text-xs text-texto-apoyo">Acordado</dt><dd className="font-semibold">{formatearSoles(a.monto_acordado_cts)}</dd></div>
            <div><dt className="text-xs text-texto-apoyo">Avance</dt><dd>{formatearSoles(a.avance_cts)}</dd></div>
            <div><dt className="text-xs text-texto-apoyo">Pendiente</dt><dd>{formatearSoles(a.pendiente_cts)}</dd></div>
          </dl>
          {a.comentario && <p className="text-texto-apoyo">{a.comentario}</p>}
          {a.anulado_motivo && <p className="text-alerta">Anulado: {a.anulado_motivo}</p>}

          <Seccion titulo="Cuotas" padding="p-0">
            <Tabla
              etiqueta="Cuotas"
              densa
              claveFila="numero"
              filas={a.cuotas || []}
              columnas={[
                { clave: 'numero', titulo: 'N.º', movil: 'titulo', render: (c) => `Cuota ${c.numero}` },
                { clave: 'vence', titulo: 'Vence', movil: 'sub', render: (c) => formatearFecha(c.vence) },
                { clave: 'monto_cts', titulo: 'Monto', alinear: 'der', movil: 'valor', render: (c) => formatearSoles(c.monto_cts) },
                { clave: 'pagado_cts', titulo: 'Pagado', alinear: 'der', render: (c) => formatearSoles(c.pagado_cts) },
                { clave: 'estado', titulo: 'Estado', movil: 'valor2', render: (c) => <Insignia estado={ESTADO_CUOTA[c.estado] || 'pendiente'} /> },
              ]}
            />
          </Seccion>

          <Seccion titulo="Recibos refinanciados" padding="p-0">
            <ul className="divide-y divide-borde px-4">
              {(a.recibos || []).map((r) => (
                <li key={r.recibo_id} className="flex items-center gap-3 py-2">
                  <span className="flex-1">{nombrePeriodo(r.periodo)} <span className="text-xs text-texto-apoyo">{r.numero}</span></span>
                  <span className="tabular-nums text-texto-apoyo">al firmar {formatearSoles(r.saldo_firma_cts)}</span>
                  <span className="tabular-nums">saldo {formatearSoles(r.saldo_cts)}</span>
                </li>
              ))}
            </ul>
          </Seccion>

          <Seccion titulo="Documento firmado">
            {a.documento_url ? (
              <Boton variante="secundario" icono="documento" href={a.documento_url}>Ver documento firmado</Boton>
            ) : (
              <p className="text-texto-apoyo">Aún no se sube el documento firmado por el presidente de la junta y el deudor.</p>
            )}
            {gestiona && a.estado === 'activo' && (
              <div className="mt-3 flex flex-col gap-2">
                <SubirArchivo etiqueta={a.documento_url ? 'Reemplazar documento' : 'Adjuntar documento firmado'} archivo={archivo} onArchivo={setArchivo} />
                {archivo && <Boton cargando={ocupado} onClick={subirDoc} className="self-start">Guardar documento</Boton>}
              </div>
            )}
          </Seccion>
        </div>
      )}
    </Modal>
  );
}
