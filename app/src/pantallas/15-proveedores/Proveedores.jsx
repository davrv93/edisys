import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Insignia, Modal, Vacio, Icono, useDialog, useToast } from '../../ui/index.js';

const FORM_PROV = { razon_social: '', ruc: '', contacto: '', telefono: '', correo: '', banco: '', cuenta: '' };
const FORM_CPP = { proveedor_id: '', rubro_id: '', descripcion: '', comprobante_tipo: 'recibo', comprobante_numero: '', fecha_emision: '', fecha_vencimiento: '', monto_cts: null };
const FORM_PAGO = { monto_cts: null, fecha: '', modalidad: 'transferencia', numero_operacion: '', comentario: '' };

const TIPO_COMPROBANTE = [
  { valor: 'recibo', etiqueta: 'Recibo' },
  { valor: 'factura', etiqueta: 'Factura' },
  { valor: 'boleta', etiqueta: 'Boleta' },
  { valor: 'otro', etiqueta: 'Otro' },
];
const MODALIDAD = [
  { valor: 'transferencia', etiqueta: 'Transferencia' },
  { valor: 'efectivo', etiqueta: 'Efectivo' },
  { valor: 'yape', etiqueta: 'Yape' },
  { valor: 'plin', etiqueta: 'Plin' },
  { valor: 'deposito', etiqueta: 'Depósito' },
  { valor: 'cheque', etiqueta: 'Cheque' },
];

const estadoCpp = (c) => (c.estado === 'pagado' ? { estado: 'pagado' } : c.estado === 'parcial' ? { estado: 'pendiente', tono: 'aviso', texto: 'Parcial' } : { estado: 'pendiente' });

/** Bloque B · Proveedores y cuentas por pagar: catálogo del edificio y el ciclo devengado → pagado. */
export default function Proveedores() {
  const eid = useEid();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [vista, setVista] = useState('cuentas');
  const [buscar, setBuscar] = useState('');
  const [estado, setEstado] = useState('');

  const prov = useCarga(() => api.get(`/edificios/${eid}/proveedores`, { buscar }), [eid, buscar]);
  const cpp = useCarga(() => api.get(`/edificios/${eid}/cuentas-por-pagar`, { estado }), [eid, estado]);
  const rubros = useCarga(() => api.get(`/edificios/${eid}/rubros`), [eid]);

  const [formProv, setFormProv] = useState(null); // {id?, ...campos}
  const [formCpp, setFormCpp] = useState(null);
  const [detalle, setDetalle] = useState(null); // {id, cuenta, pagos}
  const [formPago, setFormPago] = useState(null);
  const [ocupado, setOcupado] = useState(false);

  const listaProv = lista(prov.datos);
  const listaCpp = lista(cpp.datos);
  const listaRubros = lista(rubros.datos);
  const porPagar = cpp.datos?.por_pagar_cts ?? 0;

  const guardarProveedor = async (e) => {
    e.preventDefault();
    setOcupado(true);
    try {
      const { id, ...campos } = formProv;
      if (id) await api.put(`/edificios/${eid}/proveedores/${id}`, campos);
      else await api.post(`/edificios/${eid}/proveedores`, campos);
      toast(id ? 'Proveedor actualizado.' : 'Proveedor creado.', { tipo: 'exito' });
      setFormProv(null);
      await prov.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo guardar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const desactivar = async (p) => {
    const ok = await dialog.confirm({ title: `¿Desactivar a ${p.razon_social}?`, text: 'Dejará de aparecer en el catálogo, pero sus cuentas se conservan.', danger: true });
    if (!ok) return;
    try {
      await api.del(`/edificios/${eid}/proveedores/${p.id}`);
      toast('Proveedor desactivado.', { tipo: 'exito' });
      await prov.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo desactivar', text: err.message });
    }
  };

  const guardarCpp = async (e) => {
    e.preventDefault();
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/cuentas-por-pagar`, {
        ...formCpp,
        proveedor_id: Number(formCpp.proveedor_id),
        rubro_id: Number(formCpp.rubro_id),
        monto_cts: formCpp.monto_cts || 0,
      });
      toast('Cuenta por pagar registrada.', { tipo: 'exito' });
      setFormCpp(null);
      await cpp.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const abrirDetalle = async (c) => {
    try {
      const d = await api.get(`/edificios/${eid}/cuentas-por-pagar/${c.id}`);
      setDetalle(d);
    } catch (err) {
      await dialog.alert({ title: 'No se pudo abrir', text: err.message });
    }
  };

  const pagar = async (e) => {
    e.preventDefault();
    setOcupado(true);
    try {
      const r = await api.post(`/edificios/${eid}/cuentas-por-pagar/${detalle.cuenta.id}/pagos`, { ...formPago, monto_cts: formPago.monto_cts || 0 });
      toast(`Pago registrado. Saldo ${formatearSoles(r.saldo_cts)}.`, { tipo: 'exito' });
      setFormPago(null);
      setDetalle(null);
      await cpp.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo pagar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const acciones = (
    <>
      {vista === 'proveedores' ? (
        <Boton icono="mas_signo" onClick={() => setFormProv({ ...FORM_PROV })}>
          Nuevo proveedor
        </Boton>
      ) : (
        <Boton icono="mas_signo" onClick={() => setFormCpp({ ...FORM_CPP, proveedor_id: listaProv[0] ? String(listaProv[0].id) : '', rubro_id: listaRubros[0] ? String(listaRubros[0].id) : '' })}>
          Nueva cuenta
        </Boton>
      )}
      <Boton variante="secundario" onClick={() => setVista(vista === 'cuentas' ? 'proveedores' : 'cuentas')}>
        {vista === 'cuentas' ? 'Ver proveedores' : 'Ver cuentas'}
      </Boton>
    </>
  );

  return (
    <>
      {dialogEl}
      <Encabezado titulo="Proveedores y cuentas por pagar" subtitulo={vista === 'cuentas' ? `Por pagar: ${formatearSoles(porPagar)}` : 'Catálogo de proveedores del edificio'} ayuda="Registra a quién le pagas y controla el pago de sus comprobantes. Al pagar, el egreso entra solo al balance." acciones={acciones} />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'cuentas'} icono="recibo" onClick={() => setVista('cuentas')} contador={listaCpp.length}>
            Cuentas por pagar
          </Chip>
          <Chip activo={vista === 'proveedores'} icono="edificio" onClick={() => setVista('proveedores')} contador={listaProv.length}>
            Proveedores
          </Chip>
        </div>

        {vista === 'proveedores' ? (
          <Seccion titulo="Proveedores">
            <Campo tipo="buscar" valor={buscar} onCambio={setBuscar} placeholder="Buscar por razón social o RUC" ocultarEtiqueta aria-label="Buscar proveedor" />
            {prov.error ? (
              <ErrorCarga error={prov.error} onReintentar={prov.recargar} />
            ) : !prov.datos ? (
              <Esqueleto className="h-40 w-full" />
            ) : listaProv.length === 0 ? (
              <Vacio icono="edificio" titulo="Sin proveedores" texto="Registra a quién le pagas: administración, vigilancia, limpieza, agua, luz…">
                <Boton icono="mas_signo" onClick={() => setFormProv({ ...FORM_PROV })}>
                  Nuevo proveedor
                </Boton>
              </Vacio>
            ) : (
              <ul className="divide-y divide-borde">
                {listaProv.map((p) => (
                  <li key={p.id} className="flex items-center gap-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-tinta">
                        {p.razon_social} {!p.activo && <span className="text-texto-apoyo">· inactivo</span>}
                      </p>
                      <p className="truncate text-xs text-texto-apoyo">
                        {p.ruc ? `RUC ${p.ruc}` : 'Sin RUC'}
                        {p.contacto ? ` · ${p.contacto}` : ''}
                        {p.telefono ? ` · ${p.telefono}` : ''}
                      </p>
                    </div>
                    <span className="hidden shrink-0 text-sm tabular-nums text-texto-suave sm:block">{formatearSoles(p.por_pagar_cts)}</span>
                    <Boton tamano="sm" variante="fantasma" onClick={() => setFormProv({ id: p.id, razon_social: p.razon_social, ruc: p.ruc, contacto: p.contacto, telefono: p.telefono, correo: p.correo, banco: p.banco, cuenta: p.cuenta })}>
                      Editar
                    </Boton>
                    {p.activo && (
                      <Boton tamano="sm" variante="fantasma" className="!text-alerta" onClick={() => desactivar(p)}>
                        Desactivar
                      </Boton>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        ) : (
          <Seccion titulo="Cuentas por pagar">
            <div className="flex flex-wrap items-center gap-2">
              {[
                { id: '', etiqueta: 'Todas' },
                { id: 'pendiente', etiqueta: 'Pendientes' },
                { id: 'parcial', etiqueta: 'Parciales' },
                { id: 'pagado', etiqueta: 'Pagadas' },
              ].map((f) => (
                <Chip key={f.id} activo={estado === f.id} onClick={() => setEstado(f.id)}>
                  {f.etiqueta}
                </Chip>
              ))}
            </div>
            {cpp.error ? (
              <ErrorCarga error={cpp.error} onReintentar={cpp.recargar} />
            ) : !cpp.datos ? (
              <Esqueleto className="h-40 w-full" />
            ) : listaCpp.length === 0 ? (
              <Vacio icono="recibo" titulo="Sin cuentas por pagar" texto="Registra el comprobante de un proveedor para controlar su pago.">
                <Boton icono="mas_signo" onClick={() => setFormCpp({ ...FORM_CPP, proveedor_id: listaProv[0] ? String(listaProv[0].id) : '', rubro_id: listaRubros[0] ? String(listaRubros[0].id) : '' })}>
                  Nueva cuenta
                </Boton>
              </Vacio>
            ) : (
              <ul className="divide-y divide-borde">
                {listaCpp.map((c) => {
                  const ins = estadoCpp(c);
                  return (
                    <li key={c.id} className="flex items-center gap-3 py-2.5">
                      <button type="button" onClick={() => abrirDetalle(c)} className="min-w-0 flex-1 text-left">
                        <p className="truncate text-sm font-medium text-tinta">
                          {c.proveedor} · {c.descripcion || c.comprobante_tipo} {c.comprobante_numero ? `· ${c.comprobante_numero}` : ''}
                        </p>
                        <p className="mt-0.5 flex items-center gap-2 text-xs text-texto-apoyo">
                          <span>{formatearFecha(c.fecha_vencimiento)}</span>
                          {c.vencida && <span className="font-semibold text-alerta">· vencida</span>}
                        </p>
                      </button>
                      <div className="shrink-0 text-right">
                        <p className="text-sm font-semibold tabular-nums text-tinta">{formatearSoles(c.saldo_cts)}</p>
                        <p className="text-xs tabular-nums text-texto-apoyo">de {formatearSoles(c.monto_cts)}</p>
                      </div>
                      <Insignia estado={ins.estado} tono={ins.tono} texto={ins.texto} />
                    </li>
                  );
                })}
              </ul>
            )}
          </Seccion>
        )}
      </Contenido>

      {/* Alta / edición de proveedor */}
      <Modal abierto={!!formProv} onCerrar={() => setFormProv(null)} titulo={formProv?.id ? 'Editar proveedor' : 'Nuevo proveedor'} ancho="max-w-lg" pie={<><Boton variante="fantasma" onClick={() => setFormProv(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarProveedor}>Guardar</Boton></>}>
        {formProv && (
          <form className="flex flex-col gap-3" onSubmit={guardarProveedor}>
            <Campo etiqueta="Razón social" valor={formProv.razon_social} onCambio={(v) => setFormProv({ ...formProv, razon_social: v })} />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="RUC" valor={formProv.ruc} onCambio={(v) => setFormProv({ ...formProv, ruc: v })} ayuda="11 dígitos" />
              <Campo etiqueta="Contacto" valor={formProv.contacto} onCambio={(v) => setFormProv({ ...formProv, contacto: v })} />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Teléfono" tipo="telefono" valor={formProv.telefono} onCambio={(v) => setFormProv({ ...formProv, telefono: v })} />
              <Campo etiqueta="Correo" tipo="correo" valor={formProv.correo} onCambio={(v) => setFormProv({ ...formProv, correo: v })} />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Banco" valor={formProv.banco} onCambio={(v) => setFormProv({ ...formProv, banco: v })} />
              <Campo etiqueta="Cuenta" valor={formProv.cuenta} onCambio={(v) => setFormProv({ ...formProv, cuenta: v })} />
            </div>
          </form>
        )}
      </Modal>

      {/* Alta de cuenta por pagar */}
      <Modal abierto={!!formCpp} onCerrar={() => setFormCpp(null)} titulo="Nueva cuenta por pagar" ancho="max-w-lg" pie={<><Boton variante="fantasma" onClick={() => setFormCpp(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarCpp}>Registrar</Boton></>}>
        {formCpp && (
          <form className="flex flex-col gap-3" onSubmit={guardarCpp}>
            <Campo
              etiqueta="Proveedor"
              tipo="select"
              valor={formCpp.proveedor_id}
              onCambio={(v) => setFormCpp({ ...formCpp, proveedor_id: v })}
              opciones={listaProv.map((p) => ({ valor: String(p.id), etiqueta: p.razon_social }))}
            />
            <Campo
              etiqueta="Rubro del gasto"
              tipo="select"
              valor={formCpp.rubro_id}
              onCambio={(v) => setFormCpp({ ...formCpp, rubro_id: v })}
              opciones={listaRubros.map((r) => ({ valor: String(r.id), etiqueta: r.nombre }))}
              ayuda="Con este rubro entra el egreso en el balance al pagar."
            />
            <Campo etiqueta="Descripción" valor={formCpp.descripcion} onCambio={(v) => setFormCpp({ ...formCpp, descripcion: v })} />
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Comprobante" tipo="select" valor={formCpp.comprobante_tipo} onCambio={(v) => setFormCpp({ ...formCpp, comprobante_tipo: v })} opciones={TIPO_COMPROBANTE} />
              <Campo etiqueta="Número" valor={formCpp.comprobante_numero} onCambio={(v) => setFormCpp({ ...formCpp, comprobante_numero: v })} />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Campo etiqueta="Emisión" tipo="fecha" valor={formCpp.fecha_emision} onCambio={(v) => setFormCpp({ ...formCpp, fecha_emision: v })} />
              <Campo etiqueta="Vencimiento" tipo="fecha" valor={formCpp.fecha_vencimiento} onCambio={(v) => setFormCpp({ ...formCpp, fecha_vencimiento: v })} />
            </div>
            <Campo etiqueta="Monto" tipo="dinero" valor={formCpp.monto_cts} onCambio={(v) => setFormCpp({ ...formCpp, monto_cts: v })} />
          </form>
        )}
      </Modal>

      {/* Detalle: timeline y pagos */}
      <Modal abierto={!!detalle} onCerrar={() => setDetalle(null)} titulo="Cuenta por pagar" ancho="max-w-xl" lateral>
        {detalle && (
          <div className="flex flex-col gap-4">
            <div>
              <p className="text-sm font-semibold text-tinta">{detalle.cuenta.proveedor}</p>
              <p className="text-xs text-texto-apoyo">
                {detalle.cuenta.descripcion || detalle.cuenta.comprobante_tipo} {detalle.cuenta.comprobante_numero ? `· ${detalle.cuenta.comprobante_numero}` : ''}
              </p>
            </div>
            <div className="grid grid-cols-3 gap-3 rounded-tarjeta border border-borde p-3">
              <div>
                <p className="text-xs text-texto-apoyo">Monto</p>
                <p className="text-sm font-semibold tabular-nums">{formatearSoles(detalle.cuenta.monto_cts)}</p>
              </div>
              <div>
                <p className="text-xs text-texto-apoyo">Pagado</p>
                <p className="text-sm font-semibold tabular-nums">{formatearSoles(detalle.cuenta.pagado_cts)}</p>
              </div>
              <div>
                <p className="text-xs text-texto-apoyo">Saldo</p>
                <p className="text-sm font-semibold tabular-nums text-acento-hover">{formatearSoles(detalle.cuenta.saldo_cts)}</p>
              </div>
            </div>
            <div>
              <p className="mb-1 text-sm font-semibold">Pagos</p>
              {detalle.pagos.length === 0 ? (
                <p className="text-xs text-texto-apoyo">Sin pagos registrados.</p>
              ) : (
                <ul className="divide-y divide-borde">
                  {detalle.pagos.map((p) => (
                    <li key={p.id} className="flex items-center justify-between gap-3 py-2 text-sm">
                      <span className="min-w-0 truncate">
                        {formatearFecha(p.fecha)} · {p.modalidad}
                        {p.numero_operacion ? ` · Op. ${p.numero_operacion}` : ''}
                      </span>
                      <span className="shrink-0 tabular-nums">{formatearSoles(p.monto_cts)}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            {detalle.cuenta.estado !== 'pagado' && detalle.cuenta.estado !== 'anulado' && (
              formPago ? (
                <form className="flex flex-col gap-3" onSubmit={pagar}>
                  <div className="grid grid-cols-2 gap-3">
                    <Campo etiqueta={`Monto (saldo ${formatearSoles(detalle.cuenta.saldo_cts)})`} tipo="dinero" valor={formPago.monto_cts} onCambio={(v) => setFormPago({ ...formPago, monto_cts: v })} />
                    <Campo etiqueta="Fecha" tipo="fecha" valor={formPago.fecha} onCambio={(v) => setFormPago({ ...formPago, fecha: v })} />
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <Campo etiqueta="Modalidad" tipo="select" valor={formPago.modalidad} onCambio={(v) => setFormPago({ ...formPago, modalidad: v })} opciones={MODALIDAD} />
                    <Campo etiqueta="N.º de operación" valor={formPago.numero_operacion} onCambio={(v) => setFormPago({ ...formPago, numero_operacion: v })} />
                  </div>
                  <Campo etiqueta="Comentario" valor={formPago.comentario} onCambio={(v) => setFormPago({ ...formPago, comentario: v })} />
                  <div className="flex justify-end gap-2">
                    <Boton variante="fantasma" onClick={() => setFormPago(null)}>Cancelar</Boton>
                    <Boton cargando={ocupado} onClick={pagar} icono="check">
                      Registrar pago
                    </Boton>
                  </div>
                </form>
              ) : (
                <Boton icono="mas_signo" onClick={() => setFormPago({ ...FORM_PAGO, monto_cts: detalle.cuenta.saldo_cts })}>
                  Registrar pago
                </Boton>
              )
            )}
          </div>
        )}
      </Modal>
    </>
  );
}
