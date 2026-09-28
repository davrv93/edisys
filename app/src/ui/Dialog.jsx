import { useCallback, useMemo, useRef, useState } from 'react';
import Modal from './Modal.jsx';
import Boton from './Boton.jsx';
import Campo from './Campo.jsx';

/**
 * Reemplaza alert/confirm/prompt nativos (§0.5). Devuelven promesa:
 *   prompt → string | null · confirm → boolean · alert → true
 *
 *   const { dialog, dialogEl } = useDialog();
 *   const ok = await dialog.confirm({ title: '¿Emitir 24 recibos?', text: '…', danger: true });
 *   return (<>{dialogEl} …</>);
 */
export function useDialog() {
  const [cola, setCola] = useState([]);
  const resolver = useRef(new Map());
  const n = useRef(0);

  const abrir = useCallback((tipo, op) => {
    const id = ++n.current;
    return new Promise((resolve) => {
      resolver.current.set(id, resolve);
      setCola((c) => [...c, { id, tipo, ...(typeof op === 'string' ? { title: op } : op) }]);
    });
  }, []);

  const cerrar = useCallback((id, valor) => {
    const r = resolver.current.get(id);
    resolver.current.delete(id);
    setCola((c) => c.filter((d) => d.id !== id));
    r?.(valor);
  }, []);

  const dialog = useMemo(
    () => ({
      alert: (op) => abrir('alert', op),
      confirm: (op) => abrir('confirm', op),
      prompt: (op) => abrir('prompt', op),
    }),
    [abrir],
  );

  const dialogEl = (
    <>
      {cola.map((d) => (
        <DialogoUno key={d.id} d={d} onFin={(v) => cerrar(d.id, v)} />
      ))}
    </>
  );
  return { dialog, dialogEl };
}

function DialogoUno({ d, onFin }) {
  const [valor, setValor] = useState(d.defaultValue ?? '');
  const [error, setError] = useState(null);
  const cancelar = () => onFin(d.tipo === 'alert' ? true : d.tipo === 'confirm' ? false : null);
  const aceptar = () => {
    if (d.tipo === 'prompt') {
      if (d.required && !String(valor).trim()) {
        setError('Este dato es obligatorio.');
        return;
      }
      onFin(String(valor));
    } else onFin(true);
  };
  const pie =
    d.tipo === 'alert' ? (
      <Boton onClick={aceptar} data-autofoco>
        {d.okText || 'Entendido'}
      </Boton>
    ) : (
      <>
        <Boton variante="secundario" onClick={cancelar}>
          {d.cancelText || 'Cancelar'}
        </Boton>
        <Boton variante={d.danger ? 'peligro' : 'primario'} onClick={aceptar} data-autofoco={d.tipo === 'confirm' ? true : undefined}>
          {d.okText || (d.tipo === 'confirm' ? 'Confirmar' : 'Aceptar')}
        </Boton>
      </>
    );
  return (
    <Modal abierto onCerrar={cancelar} titulo={d.title || (d.tipo === 'alert' ? 'Aviso' : 'Confirma')} pie={pie}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          aceptar();
        }}
        className="flex flex-col gap-3"
      >
        {d.icon && <div className="text-3xl" aria-hidden="true">{d.icon}</div>}
        {d.text && <p className="text-base text-texto-suave whitespace-pre-line">{d.text}</p>}
        {d.tipo === 'prompt' && (
          <Campo
            etiqueta={d.label || 'Valor'}
            tipo={d.type === 'number' ? 'numero' : d.type === 'textarea' ? 'textarea' : 'texto'}
            valor={valor}
            onCambio={(v) => {
              setValor(v);
              setError(null);
            }}
            placeholder={d.placeholder}
            error={error}
            data-autofoco
          />
        )}
      </form>
    </Modal>
  );
}
