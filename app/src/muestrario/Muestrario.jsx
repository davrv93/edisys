import { useState } from 'react';
import { ToastProvider, useToast, Boton, Campo, Tabla, TarjetaKPI, NodoDesplegable, SubirFoto, SubirArchivo, Calendario, LeyendaCalendario, useDialog, Insignia, Vacio, ErrorCarga, SinPermiso, SelectorPeriodo, SelectorEdificio, Logo } from '../ui/index.js';
import { ESTADOS } from '../ui/estados.js';

/** Muestrario de los componentes compartidos (/app/ui, solo en desarrollo). */
export default function Muestrario() {
  return (
    <ToastProvider>
      <Contenido />
    </ToastProvider>
  );
}

function Contenido() {
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();
  const [cts, setCts] = useState(99000);
  const [periodo, setPeriodo] = useState('2026-09');
  const [foto, setFoto] = useState(null);
  const [archivo, setArchivo] = useState(null);
  const [abierto, setAbierto] = useState(false);
  const bloque = (t, c) => (
    <section className="flex flex-col gap-3 rounded-tarjeta border border-borde bg-superficie p-5">
      <h2 className="font-titulo text-xl font-semibold">{t}</h2>
      {c}
    </section>
  );
  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6 p-4 lg:p-8">
      {dialogEl}
      <Logo />
      {bloque(
        'Boton',
        <div className="flex flex-wrap gap-2">
          <Boton>Primario</Boton>
          <Boton variante="secundario">Secundario</Boton>
          <Boton variante="fantasma">Fantasma</Boton>
          <Boton variante="peligro">Peligro</Boton>
          <Boton cargando>Cargando</Boton>
          <Boton tamano="sm">Pequeño</Boton>
          <Boton tamano="lg">Grande</Boton>
        </div>,
      )}
      {bloque(
        'Dialog y Toast',
        <div className="flex flex-wrap gap-2">
          <Boton onClick={async () => toast(`confirm → ${await dialog.confirm({ title: '¿Emitir 24 recibos de setiembre?', text: 'Después de emitirlos ya no se editan; solo se anulan.' })}`)}>confirm</Boton>
          <Boton variante="peligro" onClick={async () => toast(`danger → ${await dialog.confirm({ title: '¿Anular?', danger: true })}`)}>confirm danger</Boton>
          <Boton variante="secundario" onClick={async () => toast(`prompt → ${await dialog.prompt({ title: 'Motivo', label: 'Motivo', required: true })}`)}>prompt</Boton>
          <Boton variante="secundario" onClick={() => dialog.alert({ icon: '⚠️', title: 'No se pudo guardar', text: 'Error de ejemplo.' })}>alert</Boton>
          <Boton variante="fantasma" onClick={() => toast('Error que se queda', { tipo: 'error' })}>toast error</Boton>
        </div>,
      )}
      {bloque(
        'Campo',
        <div className="grid gap-3 sm:grid-cols-3">
          <Campo etiqueta="Texto" valor="" ayuda="Texto de ayuda" />
          <Campo etiqueta="Dinero" tipo="dinero" valor={cts} onCambio={setCts} ayuda={`céntimos: ${cts}`} />
          <Campo etiqueta="Con error" valor="4000020" error="DNI debe tener 8 dígitos" />
        </div>,
      )}
      {bloque(
        'Insignia (estados.js)',
        <div className="flex flex-wrap gap-2">
          {Object.keys(ESTADOS).map((e) => (
            <Insignia key={e} estado={e} />
          ))}
        </div>,
      )}
      {bloque(
        'TarjetaKPI',
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <TarjetaKPI titulo="Ingresos cobrados" valor="S/ 19.460,00" nota="de S/ 22.400,00 emitidos" variacion={{ texto: '+12,0 % vs. agosto', buena: true }} />
          <TarjetaKPI tono="acento" titulo="Saldo del mes" valor="S/ 510,00" />
          <TarjetaKPI tono="alerta" titulo="Morosidad" valor="13,1 %" nota="3 unidades · S/ 2.940,00" />
          <TarjetaKPI cargando titulo="Cargando" />
        </div>,
      )}
      {bloque(
        'NodoDesplegable',
        <div role="tree" className="rounded-tarjeta border border-borde">
          <NodoDesplegable raiz nivel={0} abierto nodo={{ id: 'r', nombre: 'Edificio Demo · saldo del mes', total_cts: 51000, documentos: 66, tiene_hijos: true }} />
          <NodoDesplegable nivel={1} abierto={abierto} onAlternar={() => setAbierto(!abierto)} nodo={{ id: 'egr', nombre: 'Egresos', total_cts: 1895000, pct_padre: 100, documentos: 42, tiene_hijos: true }} />
          {abierto && <NodoDesplegable nivel={2} nodo={{ id: 'f', nombre: 'Fondo de contingencia', total_cts: 70000, pct_padre: 3.7, sin_sustento: true }} />}
          <NodoDesplegable nivel={1} error={new Error('x')} nodo={{ id: 'ing', nombre: 'Ingresos (error de carga)', total_cts: 1946000, tiene_hijos: true }} />
        </div>,
      )}
      {bloque(
        'Tabla (tarjetas en móvil)',
        <Tabla
          columnas={[
            { clave: 'u', titulo: 'Unidad', movil: 'titulo' },
            { clave: 'p', titulo: 'Propietario', movil: 'sub' },
            { clave: 't', titulo: 'Total', alinear: 'der', movil: 'valor' },
            { clave: 'e', titulo: 'Estado', render: (f) => <Insignia estado={f.e} /> },
          ]}
          filas={[
            { id: 1, u: 'Dpto 201', p: 'María Demo', t: 'S/ 990,00', e: 'pagado' },
            { id: 2, u: 'Dpto 402', p: 'Pedro Prueba', t: 'S/ 1.420,00', e: 'vencido' },
          ]}
          paginacion={{ pagina: 1, porPagina: 2, total: 24, onPagina: () => {} }}
        />,
      )}
      {bloque(
        'SubirFoto / SubirArchivo',
        <div className="grid gap-4 sm:grid-cols-2">
          <SubirFoto foto={foto} onFoto={setFoto} obligatoria />
          <SubirArchivo archivo={archivo} onArchivo={setArchivo} />
        </div>,
      )}
      {bloque(
        'Calendario',
        <div className="overflow-hidden rounded-tarjeta border border-borde">
          <Calendario
            dias={['2026-09-28', '2026-09-29', '2026-09-30', '2026-10-01', '2026-10-02', '2026-10-03', '2026-10-04']}
            hoy="2026-09-28"
            recursos={[{ id: 1, nombre: 'Parrilla 1', detalle: 'S/ 80 · 4 h' }]}
            eventos={[{ id: 1, recurso_id: 1, dia: '2026-10-03', desde: '12:00', hasta: '16:00', estado: 'pendiente_pago', titulo: 'Dpto 201', sub: 'Esperando pago' }]}
          />
          <LeyendaCalendario />
        </div>,
      )}
      {bloque(
        'Selectores',
        <div className="flex flex-wrap items-center gap-4">
          <SelectorPeriodo periodo={periodo} onCambio={setPeriodo} />
          <div className="w-60">
            <SelectorEdificio edificios={[{ id: 1, nombre: 'Edificio Demo', unidades: 24 }, { id: 2, nombre: 'Torre Prueba', unidades: 40 }]} actual={1} onCambio={() => {}} />
          </div>
        </div>,
      )}
      {bloque('Vacio / ErrorCarga / SinPermiso', <div className="grid gap-3 lg:grid-cols-3"><Vacio compacto titulo="Sin recibos" /><ErrorCarga compacto error={new Error('El servidor tuvo un problema.')} onReintentar={() => {}} /><SinPermiso permiso="recibos.emitir" /></div>)}
    </div>
  );
}
