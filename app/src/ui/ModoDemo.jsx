import { useEffect, useState } from 'react';
import { ruta, navegar, paginaActual } from '../lib/nav.jsx';
import Icono from './Icono.jsx';

// Modo demo: recorrido guiado por el alcance. Navega entre las páginas Astro (el estado viaja en
// localStorage) y, en cada paso, enfoca el elemento con `data-tour` y explica con un globito.

const CLAVE = 'edisys.demo';

const A = '[data-tour="acciones"]';
const C = '[data-tour="contenido"]';

export const PASOS_DEMO = [
  { pagina: 'inicio', sel: '[data-tour="menu"]', titulo: 'Tu menú, por grupos', texto: 'A la izquierda están todas las secciones, agrupadas. En pantallas medianas el menú se pliega a solo iconos; pasa el ratón para ver el nombre.' },
  { pagina: 'inicio', sel: '[data-tour="edificio"]', titulo: 'Cambia de edificio', texto: 'Si administras más de uno, cámbialo aquí; EDISYS recuerda el último que elegiste.' },
  { pagina: 'inicio', sel: C, titulo: 'Resumen del edificio', texto: 'Ingresos, egresos, saldo y morosidad del mes, más las tareas de hoy. Cada cifra te lleva a su detalle.' },
  { pagina: 'balance', sel: C, titulo: 'Balance por nodos', texto: 'De lo general a lo particular: edificio → ingresos/egresos → rubro → documento. Cada cifra con su sustento.' },
  { pagina: 'conciliacion', sel: A, titulo: 'Conciliación bancaria', texto: 'Con este botón subes el extracto del banco y EDISYS cruza los movimientos con sus pagos y egresos.' },
  { pagina: 'recibos', sel: A, titulo: 'Recibos y cobranza', texto: 'Emitir los recibos del periodo, generar borradores, enviarlos y registrar pagos está aquí, arriba a la derecha.' },
  { pagina: 'vouchers', sel: A, titulo: 'Vouchers y cuentas bancarias', texto: 'Registra el voucher con su cuenta de origen y aplícalo a uno o varios recibos de la unidad.' },
  { pagina: 'cobranzas', sel: A, titulo: 'Cobranzas sin identificar', texto: 'Registra el dinero que entra sin recibo. Luego lo imputas a una unidad (del cargo más antiguo) o lo devuelves.' },
  { pagina: 'proveedores', sel: A, titulo: 'Proveedores y cuentas por pagar', texto: 'Da de alta al proveedor y registra sus comprobantes. Al pagar, el egreso entra solo al balance.' },
  { pagina: 'fondos', sel: A, titulo: 'Trazabilidad de fondos', texto: 'Cada sol cobrado y cada egreso se asientan por servicio. Aquí transfieres entre fondos o registras un movimiento.' },
  { pagina: 'informes', sel: C, titulo: 'Informes económicos', texto: 'Resumen del mes, flujo de 12 meses y consumo de agua por departamento.' },
  { pagina: 'externos', sel: A, titulo: 'Recibos e ingresos externos', texto: 'Lo que no es la cuota: alquileres a terceros e ingresos que no vienen de un recibo.' },
  { pagina: 'unidades', sel: C, titulo: 'Unidades y propietarios', texto: 'El padrón del edificio, con importación desde Excel y la deuda inicial mes a mes.' },
  { pagina: 'reservas', sel: C, titulo: 'Reservas de áreas comunes', texto: 'Calendario sin doble reserva, con bloqueo a morosos y cobro con voucher o al recibo.' },
  { pagina: 'medidores', sel: C, titulo: 'Medidores', texto: 'El operario toma la foto del medidor; el consumo se calcula y el área común se reparte sola.' },
  { pagina: 'mantenimiento', sel: C, titulo: 'Mantenimiento', texto: 'Reportar → validar → presupuestar → aprobación de la junta, con evidencia en cada paso.' },
  { pagina: 'analitica', sel: C, titulo: 'Analítica', texto: 'Cobranza, morosidad, consumos e incidencias a lo largo del tiempo.' },
  { pagina: 'documentos', sel: A, titulo: 'Documentos', texto: 'Publica actas, reglamentos y comunicados por categorías; los propietarios ven solo lo publicado.' },
  { pagina: 'roles', sel: C, titulo: 'Roles y permisos', texto: 'Invita personas, define la junta y ajusta qué ve cada rol. Fin del recorrido.' },
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

/** Mide el elemento anclado y sigue su posición (scroll/resize). */
function useRect(sel) {
  const [rect, setRect] = useState(null);
  useEffect(() => {
    if (!sel) {
      setRect(null);
      return undefined;
    }
    const medir = () => {
      const el = document.querySelector(sel);
      if (!el) {
        setRect(null);
        return;
      }
      const r = el.getBoundingClientRect();
      setRect({ top: r.top, left: r.left, width: r.width, height: r.height });
    };
    const el = document.querySelector(sel);
    if (el && el.scrollIntoView) el.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' });
    medir();
    const t1 = setTimeout(medir, 220);
    const t2 = setTimeout(medir, 500);
    window.addEventListener('resize', medir);
    window.addEventListener('scroll', medir, true);
    return () => {
      clearTimeout(t1);
      clearTimeout(t2);
      window.removeEventListener('resize', medir);
      window.removeEventListener('scroll', medir, true);
    };
  }, [sel]);
  return rect;
}

const BAL = 340;
const PAD = 6;

/** Botón y recorrido guiado. Se monta en el armazón, así aparece en todas las pantallas. */
export default function ModoDemo() {
  const [estado, setEstado] = useState({ activo: false, paso: 0 });
  const pasoActivo = PASOS_DEMO[estado.paso];
  const rect = useRect(estado.activo ? pasoActivo?.sel : null);

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
    guardar({ activo: true, paso: p });
    setEstado({ activo: true, paso: p });
    if (PASOS_DEMO[p].pagina !== paginaActual()) navegar(ruta(PASOS_DEMO[p].pagina));
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

  const paso = pasoActivo || PASOS_DEMO[0];
  const actual = estado.paso + 1;
  const total = PASOS_DEMO.length;

  // Posición del globito: si el objetivo es bajo, debajo (o encima); si es alto o ancho
  // (menú lateral, contenido), se ancla al lado con hueco o, si no, centrado. Nunca fuera de pantalla.
  const ALTO_BAL = 200;
  const centroX = Math.max(12, (window.innerWidth - BAL) / 2);
  const topMax = Math.max(12, window.innerHeight - ALTO_BAL - 12);
  let bal;
  if (rect && rect.height <= window.innerHeight * 0.55 && rect.width <= window.innerWidth * 0.9) {
    const below = rect.top + rect.height + 12;
    const colocArriba = below + ALTO_BAL > window.innerHeight && rect.top > ALTO_BAL;
    const top = Math.max(12, Math.min(colocArriba ? rect.top - 12 - ALTO_BAL : below, topMax));
    let left = rect.left + rect.width / 2 - BAL / 2;
    left = Math.max(12, Math.min(left, window.innerWidth - BAL - 12));
    bal = { top, left };
  } else if (rect) {
    const huecoDerecha = window.innerWidth - (rect.left + rect.width) >= BAL + 24;
    const left = huecoDerecha ? rect.left + rect.width + 12 : centroX;
    bal = { top: Math.max(12, Math.min(window.innerHeight / 2 - ALTO_BAL / 2, topMax)), left };
  } else {
    bal = { top: Math.max(12, window.innerHeight / 2 - ALTO_BAL / 2), left: centroX };
  }

  return (
    <>
      {rect ? (
        <div
          aria-hidden="true"
          className="pointer-events-none fixed z-40 rounded-control ring-2 ring-acento"
          style={{
            top: rect.top - PAD,
            left: rect.left - PAD,
            width: rect.width + PAD * 2,
            height: rect.height + PAD * 2,
            boxShadow: '0 0 0 9999px rgba(15,23,42,0.55)',
            transition: 'all 160ms ease-out',
          }}
        />
      ) : (
        <div className="fixed inset-0 z-40 bg-tinta/55" aria-hidden="true" />
      )}

      <div
        role="dialog"
        aria-label="Recorrido de demostración"
        className="fixed z-50 rounded-tarjeta border border-acento-borde bg-superficie p-4 shadow-flotante"
        style={{ top: bal.top, left: bal.left, width: BAL }}
      >
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
    </>
  );
}
