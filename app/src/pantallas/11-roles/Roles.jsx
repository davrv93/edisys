import { useEffect, useState } from 'react';
import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearSoles } from '../../lib/dinero.js';
import { formatearFechaHora } from '../../lib/fechas.js';
import { NOMBRE_ROL } from '../../lib/permisos.js';
import { useQuery } from '../../lib/nav.jsx';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton, Campo, ErrorCarga, Esqueleto, Icono, Insignia, MenuAcciones, Modal, Tabla, useDialog, useToast } from '../../ui/index.js';

const ROLES_EDIFICIO = ['administrador', 'junta', 'propietario', 'inquilino', 'operario', 'tecnico'];

// Matriz del lienzo (11-roles-permisos): se usa si el API no la trae. Solo lectura en el MVP.
const MATRIZ_BASE = [
  { modulo: 'Configuración del edificio', niveles: { administrador: 'total', junta: 'ver' } },
  { modulo: 'Unidades e importación', niveles: { administrador: 'total', junta: 'ver', propietario: 'propio' } },
  { modulo: 'Recibos y cobranza', niveles: { administrador: 'total', junta: 'ver', propietario: 'propio', inquilino: 'si_habilita' } },
  { modulo: 'Balance por nodos', niveles: { administrador: 'total', junta: 'ver', propietario: 'ver' } },
  { modulo: 'Reservas', niveles: { administrador: 'total', junta: 'ver', propietario: 'accion:Reservar', inquilino: 'accion:Reservar' } },
  { modulo: 'Lectura de medidores', niveles: { administrador: 'total', junta: 'ver', propietario: 'propio', operario: 'accion:Registrar' } },
  { modulo: 'Incidencias y mantenimiento', niveles: { administrador: 'total', junta: 'ver', propietario: 'accion:Reportar', inquilino: 'accion:Reportar', operario: 'accion:Reportar', tecnico: 'accion:Actualizar' } },
  { modulo: 'Aprobación de gastos', niveles: { administrador: 'accion:Proponer', junta: 'aprobar', propietario: 'ver' } },
  { modulo: 'Roles y permisos', niveles: { administrador: 'total' }, bloqueado: true },
];

const NIVELES = {
  total: { icono: 'hecho', clase: 'text-acento', texto: 'Total' },
  aprobar: { icono: 'junta', clase: 'text-tinta', texto: 'Aprobar' },
  ver: { icono: 'ver', clase: 'text-texto-suave', texto: 'Ver' },
  propio: { icono: 'usuario', clase: 'text-texto-suave', texto: 'Su unidad' },
  si_habilita: { icono: 'parcial', clase: 'text-texto-suave', texto: 'Si lo habilita' },
  ajustable: { icono: 'ajustes', clase: 'text-aviso', texto: 'Ajustable' },
};

/** Nivel de permiso como icono (v2): check, raya, ojo…; la acción concreta lleva su verbo. Texto accesible siempre. */
function Nivel({ valor, conTexto = false }) {
  if (!valor)
    return (
      <span className="inline-flex text-texto-apoyo" title="Sin acceso">
        <Icono nombre="menos" tam={16} />
        <span className="sr-only">Sin acceso</span>
      </span>
    );
  const [tipo, texto] = valor.split(':');
  if (tipo === 'accion')
    return (
      <span className="inline-flex items-center gap-1 whitespace-nowrap rounded-chip border border-acento-borde bg-acento-suave px-2 py-0.5 text-xs font-semibold text-acento">
        <Icono nombre="check" tam={12} grosor={2.25} />
        {texto}
      </span>
    );
  const n = NIVELES[tipo] || { icono: 'info', clase: 'text-texto-suave', texto: valor };
  return (
    <span className={`inline-flex items-center gap-1.5 text-xs font-semibold ${n.clase}`} title={n.texto}>
      <Icono nombre={n.icono} tam={16} grosor={tipo === 'total' ? 2.25 : 1.75} />
      <span className={conTexto ? '' : 'sr-only'}>{n.texto}</span>
    </span>
  );
}

/** 11 · Roles y permisos: usuarios (invitar, cambiar rol, desactivar), matriz por rol (solo lectura en el MVP) y junta. */
export default function Roles() {
  const eid = useEid();
  const s = useSesion();
  const [q, setQuery] = useQuery();
  const { dialog, dialogEl } = useDialog();
  const { toast } = useToast();
  const tab = ['usuarios', 'permisos', 'junta'].includes(q.get('tab')) ? q.get('tab') : 'usuarios';
  const usuarios = useCarga(() => api.get(`/edificios/${eid}/usuarios`, { pagina: 1, por_pagina: 200 }), [eid]);
  const roles = useCarga(() => api.get('/roles'), []);
  const junta = useCarga(() => api.get(`/edificios/${eid}/junta`), [eid], { activo: tab === 'junta' });
  const [invitar, setInvitar] = useState(false);
  const [editar, setEditar] = useState(null);

  const filas = lista(usuarios.datos);
  // El API manda los roles con «codigo» (no «id»): se normaliza para contar y como clave.
  const listaRoles = (roles.datos?.roles || ROLES_EDIFICIO.map((r) => ({ id: r, nombre: NOMBRE_ROL[r] }))).map((r) => ({ ...r, id: r.id ?? r.codigo }));
  const conteo = (rol) => filas.filter((u) => u.rol === rol && u.estado !== 'inactivo').length;
  const matriz = roles.datos?.modulos || MATRIZ_BASE;

  const cambiarActivo = async (u) => {
    const activar = u.estado === 'inactivo';
    const ok = await dialog.confirm({
      title: activar ? `¿Reactivar a ${u.nombre}?` : `¿Desactivar a ${u.nombre}?`,
      text: activar ? 'Podrá volver a entrar con su clave.' : 'Se cierran sus sesiones y ya no podrá entrar. Su historial se conserva.',
      danger: !activar,
      okText: activar ? 'Reactivar' : 'Desactivar',
    });
    if (!ok) return;
    try {
      await api.patch(`/edificios/${eid}/usuarios/${u.id}`, { activo: activar });
      toast(activar ? 'Usuario reactivado.' : 'Usuario desactivado.', { tipo: 'exito' });
      usuarios.recargar();
    } catch (err) {
      await dialog.alert({ title: err.codigo === 'ULTIMO_ADMIN' ? 'No puedes dejar el edificio sin administrador' : 'No se pudo cambiar', text: err.message });
    }
  };

  const columnas = [
    { clave: 'nombre', titulo: 'Nombre', movil: 'titulo' },
    { clave: 'correo', titulo: 'Correo o unidad', render: (u) => [u.unidad, u.correo].filter(Boolean).join(' · ') || '—', movil: 'sub' },
    { clave: 'rol', titulo: 'Rol', render: (u) => `${NOMBRE_ROL[u.rol] || u.rol}${u.presidente ? ' · presidente' : ''}` },
    { clave: 'ultimo_ingreso', prioridad: 2, titulo: 'Último ingreso', render: (u) => (u.ultimo_ingreso ? formatearFechaHora(u.ultimo_ingreso) : 'Nunca') },
    { clave: 'estado', titulo: 'Estado', movil: 'valor', render: (u) => <Insignia estado={u.estado || 'activo'} /> },
    {
      clave: 'acciones',
      titulo: '',
      alinear: 'der',
      render: (u) =>
        String(u.id) === String(s.usuario.id) ? (
          <span className="text-xs text-texto-apoyo">Tú</span>
        ) : (
          <span className="flex justify-end" onClick={(e) => e.stopPropagation()}>
            <MenuAcciones
              etiqueta={`Acciones de ${u.nombre}`}
              variante="fantasma"
              items={[
                { etiqueta: 'Cambiar rol', icono: 'roles', onClick: () => setEditar(u) },
                u.estado === 'inactivo' ? { etiqueta: 'Reactivar', icono: 'hecho', onClick: () => cambiarActivo(u) } : { etiqueta: 'Desactivar', icono: 'inactivo', peligro: true, onClick: () => cambiarActivo(u) },
              ]}
            />
          </span>
        ),
    },
  ];

  return (
    <>
      {dialogEl}
      <Encabezado
        titulo="Roles y permisos"
        acciones={
          <Boton icono="mas_signo" onClick={() => setInvitar(true)}>
            Invitar persona
          </Boton>
        }
      >
        <div role="tablist" className="flex gap-1 overflow-x-auto px-4 lg:px-8">
          {[
            ['usuarios', 'Usuarios'],
            ['permisos', 'Permisos por rol'],
            ['junta', 'Junta directiva'],
          ].map(([id, t]) => (
            <button key={id} type="button" role="tab" aria-selected={tab === id} onClick={() => setQuery({ tab: id === 'usuarios' ? null : id })} className={`h-11 whitespace-nowrap border-b-2 px-3 text-sm font-semibold transition-colors duration-rapida ${tab === id ? 'border-acento text-acento' : 'border-transparent text-texto-suave hover:text-tinta'}`}>
              {t}
            </button>
          ))}
        </div>
      </Encabezado>
      <Contenido>
        <ul className="flex flex-wrap gap-1.5" aria-label="Personas por rol">
          {listaRoles.map((r) => {
            const n = usuarios.datos ? conteo(r.id) : r.personas;
            return (
              <li key={r.id} className="inline-flex items-center gap-2 rounded-chip border border-borde bg-superficie px-3 py-1 text-sm">
                <span className="font-semibold">{r.nombre || NOMBRE_ROL[r.id]}</span>
                <span className="tabular-nums text-texto-apoyo">
                  {n ?? '…'} <span className="sr-only">{n === 1 ? 'persona' : 'personas'}</span>
                </span>
              </li>
            );
          })}
        </ul>

        {tab === 'usuarios' && (
          <div className="overflow-hidden rounded-tarjeta border border-borde bg-superficie">
            <Tabla etiqueta="Usuarios del edificio" columnas={columnas} filas={filas} cargando={usuarios.cargando} error={usuarios.error} onReintentar={usuarios.recargar} claseFila={(u) => (u.estado === 'inactivo' ? 'opacity-60' : '')} />
          </div>
        )}

        {tab === 'permisos' && (
          <>
            <p className="flex items-start gap-2 rounded-tarjeta border border-borde bg-superficie p-4 text-sm text-texto-suave">
              <Icono nombre="candado" tam={18} className="mt-0.5 shrink-0" />
              En esta versión los permisos de cada rol son fijos. La matriz editable llega en la etapa 2; los permisos peligrosos (emitir recibos, administrar roles) nunca se podrán ajustar.
            </p>
            {roles.error && <ErrorCarga error={roles.error} onReintentar={roles.recargar} compacto />}
            {/* Escritorio: matriz */}
            <div className="hidden max-h-[calc(100dvh-260px)] overflow-auto rounded-tarjeta border border-borde bg-superficie md:block">
              <table className="w-full border-separate border-spacing-0 text-sm">
                <thead>
                  <tr className="text-left text-xs text-texto-apoyo">
                    <th className="sticky left-0 top-0 z-20 w-[220px] border-b border-borde bg-fondo px-4 py-2.5 font-semibold">Módulo</th>
                    {ROLES_EDIFICIO.map((r) => (
                      <th key={r} scope="col" className="sticky top-0 z-10 border-b border-borde bg-fondo px-2 py-2.5 text-center font-semibold">
                        {NOMBRE_ROL[r]}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {matriz.map((m) => (
                    <tr key={m.modulo} className="transition-colors duration-rapida hover:bg-fondo">
                      <th scope="row" className="sticky left-0 z-10 border-b border-superficie-2 bg-superficie px-4 py-2.5 text-left font-semibold">
                        <span className="flex items-center gap-2">
                          {m.modulo}
                          {m.bloqueado && <Icono nombre="candado" tam={14} className="text-texto-apoyo" titulo="Fijo" />}
                        </span>
                      </th>
                      {ROLES_EDIFICIO.map((r) => (
                        <td key={r} className="border-b border-superficie-2 px-2 py-2.5 text-center">
                          <Nivel valor={m.niveles?.[r]} />
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {/* Móvil: un acordeón por rol */}
            <div className="flex flex-col gap-2 md:hidden">
              {ROLES_EDIFICIO.map((r) => (
                <details key={r} className="rounded-tarjeta border border-borde bg-superficie">
                  <summary className="flex min-h-[52px] cursor-pointer items-center justify-between px-4 text-base font-semibold">{NOMBRE_ROL[r]}</summary>
                  <ul className="flex flex-col gap-2 border-t border-borde p-4">
                    {matriz.map((m) => (
                      <li key={m.modulo} className="flex items-center justify-between gap-2 text-sm">
                        <span>{m.modulo}</span>
                        <Nivel valor={m.niveles?.[r]} conTexto />
                      </li>
                    ))}
                  </ul>
                </details>
              ))}
            </div>
            <div className="flex flex-wrap gap-x-5 gap-y-2 text-xs text-texto-suave">
              <span className="flex items-center gap-2">
                <Nivel valor="total" conTexto /> · crear, editar y borrar
              </span>
              <span className="flex items-center gap-2">
                <Nivel valor="aprobar" conTexto /> · voto trazable
              </span>
              <span className="flex items-center gap-2">
                <Nivel valor="accion:Acción" /> solo esa acción
              </span>
              <span className="flex items-center gap-2">
                <Nivel valor="ver" conTexto /> · lectura
              </span>
              <span className="flex items-center gap-2">
                <Nivel valor="propio" conTexto /> · solo lo propio
              </span>
              <span className="flex items-center gap-2">
                <Nivel valor="si_habilita" conTexto /> · si la administración lo habilita
              </span>
              <span className="flex items-center gap-2">
                <Nivel valor="" /> sin acceso
              </span>
            </div>
          </>
        )}

        {tab === 'junta' && (
          <section className="flex max-w-xl flex-col gap-3 rounded-tarjeta border border-borde bg-superficie p-tarjeta">
            {junta.error ? (
              <ErrorCarga error={junta.error} onReintentar={junta.recargar} compacto />
            ) : !junta.datos ? (
              <Esqueleto className="h-40 w-full" />
            ) : (
              <>
                <h2 className="text-base font-semibold">Miembros de la junta</h2>
                <span className="text-sm text-texto-apoyo">
                  Regla: {junta.datos.modo === 'presidente' ? 'basta el voto del presidente' : `mayoría (${Math.floor((junta.datos.miembros?.length || 0) / 2) + 1} de ${junta.datos.miembros?.length || 0})`} sobre {formatearSoles(junta.datos.umbral_cts)}. Por debajo, aprueba la administración.
                </span>
                <ul className="flex flex-col">
                  {(junta.datos.miembros || []).map((m) => (
                    <li key={m.id} className="flex items-center justify-between border-t border-superficie-2 py-3 text-sm">
                      <span>{m.nombre}</span>
                      {m.presidente && <Insignia estado="aprobado" texto="Presidente" />}
                    </li>
                  ))}
                </ul>
              </>
            )}
          </section>
        )}
      </Contenido>

      <Invitar abierto={invitar} onCerrar={() => setInvitar(false)} eid={eid} onListo={() => usuarios.recargar()} />
      <CambiarRol usuario={editar} onCerrar={() => setEditar(null)} eid={eid} dialog={dialog} onListo={() => (setEditar(null), toast('Rol actualizado. Surte efecto en menos de un minuto.', { tipo: 'exito' }), usuarios.recargar())} />
    </>
  );
}

function Invitar({ abierto, onCerrar, eid, onListo }) {
  const [correo, setCorreo] = useState('');
  const [rol, setRol] = useState('propietario');
  const [error, setError] = useState(null);
  const [enviando, setEnviando] = useState(false);
  const [enlace, setEnlace] = useState(null);
  const [copiado, setCopiado] = useState(false);
  const cerrar = () => {
    setCorreo('');
    setEnlace(null);
    setError(null);
    setCopiado(false);
    onCerrar();
  };
  const enviar = async () => {
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(correo.trim())) {
      setError('Escribe un correo válido.');
      return;
    }
    setEnviando(true);
    try {
      const r = await api.post(`/edificios/${eid}/usuarios/invitar`, { correo: correo.trim(), rol });
      setEnlace(r?.enlace_invitacion || '');
      onListo();
    } catch (err) {
      setError(err.campos?.correo || err.message);
    } finally {
      setEnviando(false);
    }
  };
  const copiar = async () => {
    try {
      await navigator.clipboard.writeText(enlace);
      setCopiado(true);
    } catch {
      setCopiado(false);
    }
  };
  return (
    <Modal
      abierto={abierto}
      onCerrar={cerrar}
      titulo="Invitar persona"
      pie={
        enlace ? (
          <Boton onClick={cerrar}>Listo</Boton>
        ) : (
          <>
            <Boton variante="secundario" onClick={cerrar}>
              Cancelar
            </Boton>
            <Boton onClick={enviar} cargando={enviando}>
              Crear invitación
            </Boton>
          </>
        )
      }
    >
      {enlace ? (
        <div className="flex flex-col gap-3">
          <p className="text-base text-texto-suave">Invitación creada. Comparte este enlace con la persona (por ahora no se envía solo):</p>
          <input readOnly value={enlace} className="h-11 w-full rounded-control border border-borde-fuerte bg-fondo px-3 text-sm" onFocus={(e) => e.target.select()} aria-label="Enlace de invitación" />
          <Boton variante="secundario" icono={copiado ? 'check' : 'documento'} onClick={copiar}>
            {copiado ? 'Copiado' : 'Copiar enlace'}
          </Boton>
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          <Campo etiqueta="Correo" tipo="correo" valor={correo} onCambio={(v) => (setCorreo(v), setError(null))} error={error} autoComplete="off" />
          <Campo etiqueta="Rol en este edificio" tipo="select" valor={rol} onCambio={setRol} opciones={ROLES_EDIFICIO.map((r) => ({ valor: r, etiqueta: NOMBRE_ROL[r] }))} />
          <p className="text-xs text-texto-apoyo">La persona fija su propia clave con el enlace. La administración nunca ve ni fija claves.</p>
        </div>
      )}
    </Modal>
  );
}

function CambiarRol({ usuario, onCerrar, eid, dialog, onListo }) {
  const [rol, setRol] = useState('');
  const [enviando, setEnviando] = useState(false);
  const actual = usuario?.rol;
  useEffect(() => setRol(''), [usuario]);
  const guardar = async () => {
    setEnviando(true);
    try {
      await api.patch(`/edificios/${eid}/usuarios/${usuario.id}`, { rol: rol || actual });
      onListo();
    } catch (err) {
      await dialog.alert({ title: err.codigo === 'ULTIMO_ADMIN' ? 'No puedes dejar el edificio sin administrador' : 'No se pudo cambiar el rol', text: err.message });
    } finally {
      setEnviando(false);
    }
  };
  return (
    <Modal
      abierto={!!usuario}
      onCerrar={onCerrar}
      titulo={usuario ? `Rol de ${usuario.nombre}` : ''}
      pie={
        <>
          <Boton variante="secundario" onClick={onCerrar}>
            Cancelar
          </Boton>
          <Boton onClick={guardar} cargando={enviando} disabled={!rol || rol === actual}>
            Guardar
          </Boton>
        </>
      }
    >
      <Campo etiqueta="Rol" tipo="select" valor={rol || actual} onCambio={setRol} opciones={ROLES_EDIFICIO.map((r) => ({ valor: r, etiqueta: NOMBRE_ROL[r] }))} />
    </Modal>
  );
}
