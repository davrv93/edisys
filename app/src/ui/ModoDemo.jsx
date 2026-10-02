import { useEffect, useState } from 'react';
import { ruta, navegar, paginaActual } from '../lib/nav.jsx';
import Icono from './Icono.jsx';

// Modo demo: un recorrido guiado por las pantallas del alcance. Como cada pantalla es una
// página Astro, el estado del recorrido se guarda en localStorage y sobrevive a la navegación.

const CLAVE = 'edisys.demo';

export const PASOS_DEMO = [
  { pagina: 'inicio', titulo: 'Resumen del edificio', texto: 'Lo primero: ingresos, egresos, saldo y morosidad del mes, más las tareas de hoy.' },
  { pagina: 'balance', titulo: 'Balance por nodos', texto: 'De lo general a lo particular: edificio → ingresos/egresos → rubro → documento. Cada cifra con su sustento.' },
  { pagina: 'conciliacion', titulo: 'Conciliación bancaria', texto: 'Sube el extracto del banco y cruza los movimientos con los pagos y egresos de EDISYS.' },
  { pagina: 'recibos', titulo: 'Recibos y cobranza', texto: 'Emite los recibos del periodo, registra pagos y entrégalos al propietario.' },
  { pagina: 'vouchers', titulo: 'Vouchers y cuentas bancarias', texto: 'Registra un voucher con su cuenta de origen y aplícalo a uno o varios recibos de una unidad.' },
  { pagina: 'cobranzas', titulo: 'Cobranzas sin identificar', texto: 'El dinero que entra sin recibo: ímputalo a una unidad o regístralo como devolución.' },
  { pagina: 'proveedores', titulo: 'Proveedores y cuentas por pagar', texto: 'A quién le pagas y el control de sus comprobantes. Al pagar, el egreso entra solo al balance.' },
  { pagina: 'fondos', titulo: 'Trazabilidad de fondos', texto: 'Cada sol cobrado y cada egreso se asientan por servicio: mira el saldo de cada fondo.' },
  { pagina: 'informes', titulo: 'Informes económicos', texto: 'Resumen del mes, flujo de 12 meses y consumo de agua por departamento.' },
  { pagina: 'externos', titulo: 'Recibos e ingresos externos', texto: 'Lo que no es la cuota: alquileres a terceros e ingresos que no vienen de un recibo.' },
  { pagina: 'unidades', titulo: 'Unidades y propietarios', texto: 'El padrón del edificio, con importación desde Excel y la deuda inicial.' },
  { pagina: 'reservas', titulo: 'Reservas de áreas comunes', texto: 'Calendario sin doble reserva, bloqueo a morosos y cobro con voucher o al recibo.' },
  { pagina: 'medidores', titulo: 'Medidores', texto: 'El operario toma la foto del medidor; el consumo se calcula y se reparte solo.' },
  { pagina: 'mantenimiento', titulo: 'Mantenimiento', texto: 'Reportar → validar → presupuestar → aprobación de la junta, con evidencia en cada paso.' },
  { pagina: 'analitica', titulo: 'Analítica', texto: 'Cobranza, morosidad, consumos e incidencias a lo largo del tiempo.' },
  { pagina: 'roles', titulo: 'Roles y permisos', texto: 'Invita personas, define la junta y ajusta qué ve cada rol.' },
];

function leer() {
  try {
    const v = JSON.parse(window.localStorage.getItem(CLAVE));
    return v && typeof v === 'object' ? v : null;
  } catch {
    return null;
  }
}
function guardar(v) {
  try {
    if (v) window.localStorage.setItem(CLAVE, JSON.stringify(v));
    else window.localStorage.removeItem(CLAVE);
  } catch {
    /* sin almacenamiento */
  }
}

/** Botón y recorrido guiado. Se monta en el armazón, así aparece en todas las pantallas. */
export default function ModoDemo() {
  const [estado, setEstado] = useState({ activo: false, paso: 0 });

  // Al montar en cada página: si el recorrido está activo, sincroniza el paso con la página actual.
  useEffect(() => {
    const s = leer();
    if (s?.activo) {
      const idx = PASOS_DEMO.findIndex((p) => p.pagina === paginaActual());
      const paso = idx >= 0 ? idx : s.paso || 0;
      const n = { activo: true, paso };
      guardar(n);
      setEstado(n);
    } else {
      setEstado({ activo: false, paso: 0 });
    }
  }, []);

  const empezar = () => {
    const idx = PASOS_DEMO.findIndex((p) => p.pagina === paginaActual());
    const paso = idx >= 0 ? idx : 0;
    const n = { activo: true, paso };
    guardar(n);
    setEstado(n);
    if (PASOS_DEMO[paso].pagina !== paginaActual()) navegar(ruta(PASOS_DEMO[paso].pagina));
  };

  const ir = (paso) => {
    const p = Math.max(0, Math.min(PASOS_DEMO.length - 1, paso));
    const n = { activo: true, paso: p };
    guardar(n);
    setEstado(n);
    navegar(ruta(PASOS_DEMO[p].pagina));
  };

  const salir = () => {
    guardar(null);
    setEstado({ activo: false, paso: 0 });
  };

  if (!estado.activo) {
    return (
      <button
        type="button"
        onClick={empezar}
        className="fixed bottom-20 right-3 z-40 inline-flex items-center gap-2 rounded-chip border border-acento-borde bg-acento px-3.5 py-2 text-sm font-semibold text-white shadow-flotante transition-colors duration-rapida hover:bg-acento-hover lg:bottom-6 lg:right-6"
        aria-label="Iniciar el recorrido de demostración"
      >
        <Icono nombre="robot" tam={16} />
        Modo demo
      </button>
    );
  }

  const paso = PASOS_DEMO[estado.paso] || PASOS_DEMO[0];
  const actual = estado.paso + 1;
  const total = PASOS_DEMO.length;
  return (
    <div className="fixed bottom-20 right-3 z-40 w-[min(92vw,350px)] rounded-tarjeta border border-acento-borde bg-superficie p-4 shadow-flotante lg:bottom-6 lg:right-6" role="dialog" aria-label="Recorrido de demostración">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-2 text-acento">
          <Icono nombre="robot" tam={16} />
          <span className="text-xs font-semibold uppercase tracking-wide">Modo demo · {actual} de {total}</span>
        </div>
        <button type="button" onClick={salir} aria-label="Salir del recorrido" className="-mr-1 -mt-1 inline-flex h-6 w-6 items-center justify-center rounded-chip text-texto-apoyo hover:text-tinta">
          <Icono nombre="cerrar" tam={14} />
        </button>
      </div>
      <p className="mt-2 font-titulo text-base font-semibold text-tinta">{paso.titulo}</p>
      <p className="mt-1 text-sm text-texto-suave">{paso.texto}</p>
      <div className="mt-3 h-1 w-full overflow-hidden rounded-chip bg-superficie-2" aria-hidden="true">
        <div className="h-full rounded-chip bg-acento transition-[width] duration-media" style={{ width: `${(actual / total) * 100}%` }} />
      </div>
      <div className="mt-3 flex items-center justify-between gap-2">
        <button type="button" onClick={() => ir(estado.paso - 1)} disabled={estado.paso === 0} className="inline-flex h-9 items-center rounded-control border border-borde-fuerte bg-superficie px-3 text-sm font-semibold text-tinta transition-colors duration-rapida hover:bg-fondo disabled:opacity-40">
          Anterior
        </button>
        {estado.paso < total - 1 ? (
          <button type="button" onClick={() => ir(estado.paso + 1)} className="inline-flex h-9 items-center rounded-control bg-acento px-4 text-sm font-semibold text-white transition-colors duration-rapida hover:bg-acento-hover">
            Siguiente
          </button>
        ) : (
          <button type="button" onClick={salir} className="inline-flex h-9 items-center gap-1.5 rounded-control bg-acento px-4 text-sm font-semibold text-white transition-colors duration-rapida hover:bg-acento-hover">
            <Icono nombre="check" tam={15} /> Terminar
          </button>
        )}
      </div>
    </div>
  );
}
