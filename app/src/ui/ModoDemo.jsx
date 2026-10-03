import { useEffect, useState } from 'react';
import { ruta, navegar, paginaActual } from '../lib/nav.jsx';
import Icono from './Icono.jsx';

// Recorridos guiados. Navegan entre páginas Astro (el estado viaja en localStorage) y, en cada paso,
// enfocan el elemento con `data-tour` y explican con un globito. Hay tres recorridos:
// «demo» (qué hace cada módulo), «dia» (gestión de un día cualquiera) y «finmes» (cierre de mes).

const CLAVE = 'edisys.demo';
const A = '[data-tour="acciones"]';
const C = '[data-tour="contenido"]';
const M = '[data-tour="menu"]';

export const TOURS = {
  demo: {
    nombre: 'Qué hace cada módulo',
    corto: 'Modo demo',
    pasos: [
      { pagina: 'inicio', sel: M, titulo: 'Tu menú, por grupos', texto: 'A la izquierda están todas las secciones, agrupadas. En pantallas medianas el menú se pliega a solo iconos; pasa el ratón para ver el nombre.' },
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
    ],
  },
  dia: {
    nombre: 'Día cualquiera',
    corto: 'Día cualquiera',
    pasos: [
      { pagina: 'inicio', sel: C, titulo: 'Empieza el día', texto: 'Mira la cobranza del mes, el saldo y la lista de tareas de hoy. Cada tarjeta te lleva a su pantalla.' },
      { pagina: 'cobranzas', sel: A, titulo: '1 · Dinero sin identificar', texto: 'Lo primero: revisa si entró dinero sin recibo y ímputalo a su unidad, o regístralo como devolución.' },
      { pagina: 'recibos', sel: A, titulo: '2 · Vouchers del día', texto: 'Entra a Recibos y valida los pagos informados por los propietarios (los vouchers en revisión).' },
      { pagina: 'vouchers', sel: A, titulo: '3 · Cobro multicuenta', texto: 'Si el pago llega a varias cuotas, usa Vouchers: elige la unidad, marca los recibos y aplica el voucher.' },
      { pagina: 'proveedores', sel: A, titulo: '4 · Pagos del día', texto: 'Revisa las cuentas por pagar pendientes y registra el pago del proveedor. El egreso entra solo al balance.' },
      { pagina: 'mantenimiento', sel: C, titulo: '5 · Tickets del día', texto: 'Atiende las incidencias reportadas: valida, asigna y mueve la tarjeta por el tablero.' },
      { pagina: 'medidores', sel: C, titulo: '6 · Ronda de lecturas', texto: 'Si toca, entra a Medidores y toma las lecturas con foto; el consumo se calcula al instante.' },
      { pagina: 'documentos', sel: A, titulo: '7 · Comunicados', texto: 'Publica el anuncio del día (corte de agua, reunión) para que los vecinos queden avisados.' },
      { pagina: 'inicio', sel: C, titulo: '8 · Cierre del día', texto: 'Vuelve al resumen: lo que quedó pendiente en tareas es lo que mañana toca primero.' },
    ],
  },
  finmes: {
    nombre: 'Día de fin de mes',
    corto: 'Fin de mes',
    pasos: [
      { pagina: 'inicio', sel: C, titulo: 'Preparar el cierre', texto: 'Antes de emitir: mira morosidad, saldo y pendientes. Es tu foto de partida del mes.' },
      { pagina: 'unidades', sel: C, titulo: '1 · Verifica el padrón', texto: 'Confirma que no falten altas ni cambios de propietario, y que las participaciones sumen 100 %.' },
      { pagina: 'medidores', sel: C, titulo: '2 · Cierra la ronda', texto: 'Asegura que estén todas las lecturas del mes; sin lecturas no se puede emitir si el edificio cobra agua.' },
      { pagina: 'recibos', sel: A, titulo: '3 · Genera y emite', texto: 'Genera borradores, revísalos, emite el periodo y envíalos por correo. Un recibo emitido ya no se edita: se anula.' },
      { pagina: 'balance', sel: C, titulo: '4 · Revisa el balance', texto: 'Baja de rubro a concepto a documento. Verifica que todo tenga sustento y que el saldo cuadre.' },
      { pagina: 'conciliacion', sel: A, titulo: '5 · Cuadra con el banco', texto: 'Sube el extracto del banco y confirma las parejas. La diferencia debería llegar a cero.' },
      { pagina: 'fondos', sel: A, titulo: '6 · Trazabilidad', texto: 'Mira el saldo de cada fondo/servicio y, si hace falta, transfiere entre cuentas. El total no cambia.' },
      { pagina: 'informes', sel: C, titulo: '7 · Informe del mes', texto: 'Revisa ingresos/egresos, el flujo de 12 meses y los consumos. Este es el informe que ve la junta.' },
      { pagina: 'proveedores', sel: A, titulo: '8 · Cuentas por pagar', texto: 'Deja al día las cuentas por pagar del mes para no arrastrar deuda con proveedores.' },
      { pagina: 'documentos', sel: A, titulo: '9 · Acta de cierre', texto: 'Publica el acta y el informe del mes; los propietarios ven el balance sin pedirlo.' },
      { pagina: 'inicio', sel: C, titulo: 'Mes cerrado', texto: 'Vuelve al resumen: la cobranza del nuevo mes ya corre y la morosidad quedó visible para la cobranza.' },
    ],
  },
};

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

const BAL = 350;
const PAD = 6;
const ALTO_BAL = 210;

/** Botón(es) y recorrido guiado. Se monta en el armazón, así aparece en todas las pantallas. */
export default function ModoDemo() {
  const [estado, setEstado] = useState({ activo: false, tour: 'demo', paso: 0 });
  const [menu, setMenu] = useState(false);
  const pasos = TOURS[estado.tour]?.pasos || TOURS.demo.pasos;
  const pasoActivo = pasos[estado.paso];
  const rect = useRect(estado.activo ? pasoActivo?.sel : null);

  useEffect(() => {
    const s = leer();
    if (s?.activo && TOURS[s.tour]) {
      const idx = TOURS[s.tour].pasos.findIndex((p) => p.pagina === paginaActual());
      const paso = idx >= 0 ? idx : s.paso || 0;
      const n = { activo: true, tour: s.tour, paso };
      guardar(n);
      setEstado(n);
    } else {
      setEstado({ activo: false, tour: 'demo', paso: 0 });
    }
  }, []);

  const empezar = (tour) => {
    setMenu(false);
    const p = TOURS[tour].pasos;
    const idx = p.findIndex((x) => x.pagina === paginaActual());
    const paso = idx >= 0 ? idx : 0;
    guardar({ activo: true, tour, paso });
    setEstado({ activo: true, tour, paso });
    if (p[paso].pagina !== paginaActual()) navegar(ruta(p[paso].pagina));
  };

  const ir = (paso) => {
    const p = Math.max(0, Math.min(pasos.length - 1, paso));
    guardar({ activo: true, tour: estado.tour, paso: p });
    setEstado({ activo: true, tour: estado.tour, paso: p });
    if (pasos[p].pagina !== paginaActual()) navegar(ruta(pasos[p].pagina));
  };

  const salir = () => {
    guardar(null);
    setEstado({ activo: false, tour: 'demo', paso: 0 });
  };

  if (!estado.activo) {
    return (
      <div className="fixed bottom-20 right-3 z-40 flex flex-col items-end gap-2 lg:bottom-6 lg:right-6">
        {menu && (
          <div className="w-[min(92vw,320px)] rounded-tarjeta border border-acento-borde bg-superficie p-2 shadow-flotante" role="menu">
            <p className="px-2 pb-1 pt-1 text-xs font-semibold uppercase tracking-wide text-texto-apoyo">Elige el tour</p>
            {['dia', 'finmes'].map((t) => (
              <button key={t} type="button" onClick={() => empezar(t)} className="flex w-full flex-col items-start gap-0.5 rounded-control px-2 py-2 text-left transition-colors duration-rapida hover:bg-fondo">
                <span className="text-sm font-semibold text-tinta">{TOURS[t].nombre}</span>
                <span className="text-xs text-texto-apoyo">{t === 'dia' ? 'La rutina diaria, pantalla por pantalla.' : 'El cierre del mes, paso a paso.'}</span>
              </button>
            ))}
            <button type="button" onClick={() => empezar('demo')} className="flex w-full flex-col items-start gap-0.5 rounded-control px-2 py-2 text-left transition-colors duration-rapida hover:bg-fondo">
              <span className="text-sm font-semibold text-tinta">{TOURS.demo.nombre}</span>
              <span className="text-xs text-texto-apoyo">Recorrido por todos los módulos.</span>
            </button>
          </div>
        )}
        <button
          type="button"
          onClick={() => setMenu((v) => !v)}
          aria-expanded={menu}
          className="inline-flex items-center gap-2 rounded-chip border border-acento-borde bg-acento px-3.5 py-2 text-sm font-semibold text-white shadow-flotante transition-colors duration-rapida hover:bg-acento-hover"
        >
          <Icono nombre="robot" tam={16} />
          Tour
        </button>
      </div>
    );
  }

  const paso = pasoActivo || pasos[0];
  const actual = estado.paso + 1;
  const total = pasos.length;

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
        aria-label="Recorrido guiado"
        className="fixed z-50 rounded-tarjeta border border-acento-borde bg-superficie p-4 shadow-flotante"
        style={{ top: bal.top, left: bal.left, width: BAL }}
      >
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 text-acento">
            <Icono nombre="robot" tam={16} />
            <span className="text-xs font-semibold uppercase tracking-wide">{TOURS[estado.tour].nombre} · {actual} de {total}</span>
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
