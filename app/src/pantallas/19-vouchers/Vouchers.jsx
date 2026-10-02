import { useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFecha } from '../../lib/fechas.js';
import { useEid } from '../../layout/Sesion.jsx';
import { useCarga } from '../../lib/useCarga.js';
import Encabezado, { Contenido, Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, Chip, ErrorCarga, Esqueleto, Modal, Vacio, useDialog, useToast } from '../../ui/index.js';

const FORM_CUENTA = { banco: '', numero: '', moneda: 'PEN' };

/** Bloque A3 · Vouchers multicuenta: cuentas bancarias y cobro aplicado a varios recibos. */
export default function Vouchers() {
  const eid = useEid();
  const { toast } = useToast();
  const { dialog, dialogEl } = useDialog();
  const [vista, setVista] = useState('voucher');

  const cuentas = useCarga(() => api.get(`/edificios/${eid}/cuentas-bancarias`), [eid]);
  const listaCuentas = lista(cuentas.datos);

  const [formCuenta, setFormCuenta] = useState(null);
  const [ocupado, setOcupado] = useState(false);

  // Registro de voucher.
  const [codigo, setCodigo] = useState('');
  const [unidad, setUnidad] = useState(null);
  const [cargos, setCargos] = useState([]);
  const [sel, setSel] = useState({}); // recibo_id → true
  const [cuentaID, setCuentaID] = useState('');
  const [op, setOp] = useState('');
  const [fecha, setFecha] = useState('');
  const [buscando, setBuscando] = useState(false);

  const buscar = async () => {
    setBuscando(true);
    try {
      const r = await api.get(`/edificios/${eid}/unidades`, { buscar: codigo });
      const u = lista(r)[0];
      if (!u) {
        await dialog.alert({ title: 'Sin unidad', text: `No encontré la unidad «${codigo}».` });
        setUnidad(null);
        setCargos([]);
        return;
      }
      const c = await api.get(`/edificios/${eid}/unidades/${u.id}/cuenta`);
      setUnidad(u);
      setCargos(lista(c.cargos).filter((x) => x.saldo_cts > 0));
      setSel({});
    } catch (err) {
      await dialog.alert({ title: 'No se pudo buscar', text: err.message });
    } finally {
      setBuscando(false);
    }
  };

  const totalSel = cargos.filter((c) => sel[c.recibo_id]).reduce((s, c) => s + c.saldo_cts, 0);
  const marcarTodo = () => {
    const todos = {};
    cargos.forEach((c) => (todos[c.recibo_id] = true));
    setSel(todos);
  };

  const registrar = async () => {
    const aplicaciones = cargos.filter((c) => sel[c.recibo_id]).map((c) => ({ recibo_id: c.recibo_id, monto_cts: c.saldo_cts }));
    if (!unidad || aplicaciones.length === 0) {
      await dialog.alert({ title: 'Falta información', text: 'Busca una unidad y marca al menos un recibo.' });
      return;
    }
    if (!op.trim()) {
      await dialog.alert({ title: 'Falta el código', text: 'Escribe el código de operación del voucher.' });
      return;
    }
    setOcupado(true);
    try {
      const r = await api.post(`/edificios/${eid}/unidades/${unidad.id}/vouchers`, {
        cuenta_bancaria_id: cuentaID ? Number(cuentaID) : 0,
        medio: 'transferencia',
        codigo_operacion: op.trim(),
        fecha,
        aplicaciones,
      });
      toast(`Voucher registrado (${r.estado === 'validado' ? 'cobrado' : 'por validar'}).`, { tipo: 'exito' });
      setSel({});
      setOp('');
      await buscar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo registrar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  const guardarCuenta = async (e) => {
    e.preventDefault();
    setOcupado(true);
    try {
      await api.post(`/edificios/${eid}/cuentas-bancarias`, formCuenta);
      toast('Cuenta bancaria guardada.', { tipo: 'exito' });
      setFormCuenta(null);
      await cuentas.recargar();
    } catch (err) {
      await dialog.alert({ title: 'No se pudo guardar', text: err.message });
    } finally {
      setOcupado(false);
    }
  };

  return (
    <>
      {dialogEl}
      <Encabezado titulo="Vouchers y cuentas bancarias" subtitulo="Cobra un voucher aplicándolo a varios recibos" ayuda="Registra el voucher con su cuenta de origen y aplícalo a uno o varios recibos de la unidad. Queda cobrado o por validar según tu rol." />
      <Contenido>
        <div className="flex flex-wrap items-center gap-2">
          <Chip activo={vista === 'voucher'} icono="recibo" onClick={() => setVista('voucher')}>
            Registrar voucher
          </Chip>
          <Chip activo={vista === 'cuentas'} icono="edificio" onClick={() => setVista('cuentas')} contador={listaCuentas.length}>
            Cuentas bancarias
          </Chip>
        </div>

        {vista === 'cuentas' ? (
          <Seccion titulo="Cuentas bancarias" extra={<Boton icono="mas_signo" onClick={() => setFormCuenta({ ...FORM_CUENTA })}>Nueva cuenta</Boton>}>
            {cuentas.error ? (
              <ErrorCarga error={cuentas.error} onReintentar={cuentas.recargar} />
            ) : !cuentas.datos ? (
              <Esqueleto className="h-40 w-full" />
            ) : listaCuentas.length === 0 ? (
              <Vacio icono="edificio" titulo="Sin cuentas" texto="Registra las cuentas donde el edificio recibe los pagos." />
            ) : (
              <ul className="divide-y divide-borde">
                {listaCuentas.map((c) => (
                  <li key={c.id} className="flex items-center gap-3 py-2.5">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-tinta">{c.banco}</p>
                      <p className="truncate text-xs text-texto-apoyo">{c.numero || 'Sin número'} · {c.moneda}</p>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </Seccion>
        ) : (
          <>
            <Seccion titulo="Buscar unidad">
              <div className="flex flex-wrap items-end gap-3">
                <Campo etiqueta="Código de unidad" valor={codigo} onCambio={setCodigo} className="max-w-[180px]" />
                <Boton cargando={buscando} onClick={buscar}>Buscar deuda</Boton>
                {unidad && <span className="text-sm text-texto-apoyo">Dpto {unidad.codigo} · deuda {formatearSoles(cargos.reduce((s, c) => s + c.saldo_cts, 0))}</span>}
              </div>
            </Seccion>

            {unidad && (
              <Seccion titulo="Recibos con saldo" extra={cargos.length > 0 && <Boton variante="fantasma" tamano="sm" onClick={marcarTodo}>Marcar todos</Boton>}>
                {cargos.length === 0 ? (
                  <p className="text-sm text-texto-apoyo">Esta unidad no tiene recibos con saldo.</p>
                ) : (
                  <ul className="divide-y divide-borde">
                    {cargos.map((c) => (
                      <li key={c.recibo_id} className="flex items-center gap-3 py-2">
                        <input type="checkbox" className="h-4 w-4" checked={!!sel[c.recibo_id]} onChange={(e) => setSel({ ...sel, [c.recibo_id]: e.target.checked })} aria-label={`Aplicar a ${c.numero}`} />
                        <span className="min-w-0 flex-1 text-sm">
                          {c.periodo} · {c.numero || 'recibo'} <span className="text-texto-apoyo">{formatearFecha(c.vence)}</span>
                        </span>
                        <span className="text-sm font-semibold tabular-nums">{formatearSoles(c.saldo_cts)}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </Seccion>
            )}

            {unidad && cargos.length > 0 && (
              <Seccion titulo="Datos del voucher">
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                  <Campo etiqueta="Cuenta" tipo="select" valor={cuentaID} onCambio={setCuentaID} opciones={[{ valor: '', etiqueta: 'Sin cuenta' }, ...listaCuentas.map((c) => ({ valor: String(c.id), etiqueta: `${c.banco} ${c.numero}` }))]} />
                  <Campo etiqueta="Código de operación" valor={op} onCambio={setOp} />
                  <Campo etiqueta="Fecha" tipo="fecha" valor={fecha} onCambio={setFecha} />
                  <div className="flex flex-col justify-end">
                    <span className="text-xs text-texto-apoyo">Total a cobrar</span>
                    <span className="text-lg font-semibold tabular-nums">{formatearSoles(totalSel)}</span>
                  </div>
                </div>
                <div className="flex justify-end">
                  <Boton cargando={ocupado} onClick={registrar}>Registrar voucher</Boton>
                </div>
              </Seccion>
            )}
          </>
        )}
      </Contenido>

      <Modal abierto={!!formCuenta} onCerrar={() => setFormCuenta(null)} titulo="Nueva cuenta bancaria" ancho="max-w-md" pie={<><Boton variante="fantasma" onClick={() => setFormCuenta(null)}>Cancelar</Boton><Boton cargando={ocupado} onClick={guardarCuenta}>Guardar</Boton></>}>
        {formCuenta && (
          <form className="flex flex-col gap-3" onSubmit={guardarCuenta}>
            <Campo etiqueta="Banco" valor={formCuenta.banco} onCambio={(v) => setFormCuenta({ ...formCuenta, banco: v })} />
            <Campo etiqueta="Número" valor={formCuenta.numero} onCambio={(v) => setFormCuenta({ ...formCuenta, numero: v })} />
            <Campo etiqueta="Moneda" tipo="select" valor={formCuenta.moneda} onCambio={(v) => setFormCuenta({ ...formCuenta, moneda: v })} opciones={[{ valor: 'PEN', etiqueta: 'Soles' }, { valor: 'USD', etiqueta: 'Dólares' }]} />
          </form>
        )}
      </Modal>
    </>
  );
}
