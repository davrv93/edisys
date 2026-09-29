import { useState } from 'react';
import { api } from '../../lib/api.js';
import { formatearSoles } from '../../lib/dinero.js';
import { Boton, Campo, Modal, useDialog, useToast } from '../../ui/index.js';

/**
 * Corregir una lectura ya tomada (lecturas.corregir). El API recalcula en cascada el mes siguiente,
 * rehace los repartos y, si los recibos ya se emitieron, deja notas de cargo/abono para el próximo recibo.
 */
export default function CorregirLectura({ eid, medidor, onCorregida }) {
  const [abierto, setAbierto] = useState(false);
  const [valor, setValor] = useState('');
  const [motivo, setMotivo] = useState('');
  const [errores, setErrores] = useState({});
  const [enviando, setEnviando] = useState(false);
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();

  const abrir = () => {
    setValor(String(medidor.lectura_actual ?? ''));
    setMotivo('');
    setErrores({});
    setAbierto(true);
  };

  const enviar = async (confirmarNegativo = false) => {
    setEnviando(true);
    setErrores({});
    try {
      const res = await api.put(`/edificios/${eid}/lecturas/${medidor.lectura_id}`, {
        valor: String(valor).replace(',', '.'),
        motivo,
        confirmar_negativo: confirmarNegativo,
      });
      setAbierto(false);
      const ajustes = res?.ajustes?.length || 0;
      const cascada = res?.siguiente ? ` El consumo de ${res.siguiente.periodo} se recalculó.` : '';
      toast(`Lectura de ${medidor.unidad} corregida.${cascada}${ajustes ? ` ${ajustes} ajustes irán al siguiente recibo.` : ''}`, { tipo: 'exito' });
      onCorregida?.(res);
    } catch (err) {
      if (err.codigo === 'CONSUMO_NEGATIVO' && !confirmarNegativo) {
        const ok = await dialog.confirm({ title: 'El consumo sale negativo', text: `${err.message}`, okText: 'Sí, cambió el medidor' });
        if (ok) return enviar(true);
      } else {
        setErrores(err.campos || {});
        if (!err.campos || !Object.keys(err.campos).length) await dialog.alert({ title: 'No se pudo corregir', text: err.message });
      }
    } finally {
      setEnviando(false);
    }
  };

  return (
    <>
      {dialogEl}
      <Boton variante="fantasma" tamano="sm" icono="documento" onClick={abrir} aria-label={`Corregir la lectura de ${medidor.unidad}`}>
        Corregir
      </Boton>
      <Modal
        abierto={abierto}
        onCerrar={() => setAbierto(false)}
        titulo={`Corregir lectura · ${medidor.unidad}`}
        pie={
          <>
            <Boton variante="secundario" onClick={() => setAbierto(false)}>
              Cancelar
            </Boton>
            <Boton cargando={enviando} disabled={!valor || !motivo.trim()} onClick={() => enviar(false)}>
              Guardar corrección
            </Boton>
          </>
        }
      >
        <div className="flex flex-col gap-4">
          <p className="text-sm text-texto-apoyo">
            Anterior {medidor.lectura_anterior} m³ · actual {medidor.lectura_actual} m³. Si el mes siguiente ya tiene lectura, su consumo se recalcula; si los recibos ya se
            emitieron, la diferencia va como nota de cargo o abono al próximo recibo{medidor.total_cts ? ` (hoy ${formatearSoles(medidor.total_cts)})` : ''}.
          </p>
          <Campo etiqueta="Lectura corregida (m³)" tipo="numero" valor={valor} onCambio={setValor} error={errores.valor} />
          <Campo etiqueta="Motivo" tipo="textarea" valor={motivo} onCambio={setMotivo} error={errores.motivo} ayuda="Queda en la auditoría." />
        </div>
      </Modal>
    </>
  );
}
