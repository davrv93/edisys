import { useEffect } from 'react';
import { useToast } from './ui/index.js';

/** Registra el service worker y avisa cuando hay una versión nueva (§2.3). */
export default function Pwa() {
  const { toast } = useToast();
  useEffect(() => {
    if (import.meta.env.DEV || !('serviceWorker' in navigator)) return;
    let cancelado = false;
    import('virtual:pwa-register').then(({ registerSW }) => {
      if (cancelado) return;
      const actualizar = registerSW({
        onNeedRefresh() {
          toast('Hay una versión nueva de EDISYS.', { fijo: true, accion: { texto: 'Toca para actualizar', onClick: () => actualizar(true) } });
        },
      });
    });
    return () => {
      cancelado = true;
    };
  }, [toast]);
  return null;
}
