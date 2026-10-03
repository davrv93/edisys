import { $, component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import {
  routeLoader$,
  useLocation,
  type DocumentHead,
  type RequestHandler,
} from "@builder.io/qwik-city";
import { Particulas } from "~/components/particulas/particulas";
import {
  DESTINO_POR_DEFECTO,
  ERROR_SIN_CONEXION,
  RUTA_LOGIN_API,
  destinoDesdeUrl,
  destinoSeguro,
  errorDesdeRespuesta,
  validar,
  type ErrorLogin,
} from "~/lib/login";
import { CLAVE_SLUG, colorValido, cssDeTokens, rutaMarca, slugDe, type MarcaPublica } from "~/lib/marca"; // marca: I2

const CLAVE_RECORDAR = "edisys.login.correo";

interface ResultadoServidor {
  correo?: string;
  error?: ErrorLogin;
}

/* ------------------------------------------------------------------ *
 * Respaldo sin JavaScript (§01: «funciona sin JavaScript para el envío»).
 * Con JS, el navegador llama al API directamente y este handler no corre.
 * Sin JS, el <form method="post"> llega aquí: se reenvía al API por la red
 * interna y se copian sus Set-Cookie (HttpOnly) a la respuesta.
 * ------------------------------------------------------------------ */
type Jarra = Parameters<RequestHandler>[0]["cookie"];

/** Copia un Set-Cookie del API a la respuesta con cookie.set, conservando sus atributos. */
function copiarCookie(cookie: Jarra, linea: string) {
  const [par, ...attrs] = linea.split(";").map((x) => x.trim());
  const i = par.indexOf("=");
  if (i <= 0) return;
  const opts: Record<string, unknown> = {};
  for (const a of attrs) {
    const [k, ...v] = a.split("=");
    const val = v.join("=");
    switch (k.toLowerCase()) {
      case "path": opts.path = val; break;
      case "domain": opts.domain = val; break;
      case "max-age": opts.maxAge = Number(val); break;
      case "expires": opts.expires = new Date(val); break;
      case "httponly": opts.httpOnly = true; break;
      case "secure": opts.secure = true; break;
      case "samesite": opts.sameSite = val.toLowerCase(); break;
    }
  }
  cookie.set(par.slice(0, i), decodeURIComponent(par.slice(i + 1)), opts);
}

export const onPost: RequestHandler = async ({ request, cookie, redirect, sharedMap, url, clientConn }) => {
  const form = await request.formData();
  const correo = String(form.get("correo") ?? "").trim();
  const clave = String(form.get("clave") ?? "");
  const nextForm = String(form.get("next") ?? "");
  const destino = nextForm ? destinoSeguro(nextForm) : destinoDesdeUrl(url);

  const invalido = validar(correo, clave);
  if (invalido) {
    sharedMap.set("login", { correo, error: invalido } satisfies ResultadoServidor);
    return;
  }

  const api = process.env.EDISYS_API_INTERNO ?? "http://api:8080";
  let res: Response;
  try {
    res = await fetch(`${api}${RUTA_LOGIN_API}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
        "X-EDISYS": "1",
        ...(clientConn.ip ? { "X-Forwarded-For": clientConn.ip } : {}),
      },
      body: JSON.stringify({ correo, clave }),
    });
  } catch {
    sharedMap.set("login", { correo, error: ERROR_SIN_CONEXION } satisfies ResultadoServidor);
    return;
  }

  if (res.ok) {
    // headers.append("Set-Cookie") dos veces se colapsa en una sola cabecera y el
    // navegador pierde edisys_at; cookie.set de Qwik City emite una por cookie.
    for (const c of res.headers.getSetCookie()) copiarCookie(cookie, c);
    throw redirect(303, destino);
  }
  let cuerpo: unknown = null;
  try {
    cuerpo = await res.json();
  } catch {
    /* cuerpo vacío o no JSON */
  }
  sharedMap.set("login", {
    correo,
    error: errorDesdeRespuesta(res.status, cuerpo, res.headers),
  } satisfies ResultadoServidor);
};

export const useResultadoServidor = routeLoader$<ResultadoServidor>(
  ({ sharedMap }) => (sharedMap.get("login") as ResultadoServidor | undefined) ?? {},
);

/* ------------------------------------------------------------------ */

const IconoOjo = component$((props: { abierto: boolean }) => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" />
    <circle cx="12" cy="12" r="3" />
    {!props.abierto && <path d="M3 3l18 18" />}
  </svg>
));

const Spinner = () => (
  <svg class="h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden="true">
    <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity="0.3" stroke-width="3" />
    <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" stroke-width="3" stroke-linecap="round" />
  </svg>
);

export default component$(() => {
  const loc = useLocation();
  const servidor = useResultadoServidor();
  const destino = destinoDesdeUrl(loc.url);

  const correo = useSignal(servidor.value.correo ?? "");
  const clave = useSignal("");
  const verClave = useSignal(false);
  const recordar = useSignal(true);
  const enviando = useSignal(false);
  const error = useSignal<ErrorLogin | null>(servidor.value.error ?? null);
  const nota = useSignal<string | null>(null);
  const bloqueadoHasta = useSignal(0);
  const restanteMin = useSignal(0);
  const correoRef = useSignal<HTMLInputElement>();
  const claveRef = useSignal<HTMLInputElement>();
  // marca: I2 · nombre, lema, logo y fondo de la administradora (null = EDISYS).
  const marca = useSignal<MarcaPublica | null>(null);

  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    let guardado: string | null = null;
    try {
      guardado = localStorage.getItem(CLAVE_SLUG);
    } catch {
      /* almacenamiento bloqueado */
    }
    const slug = slugDe(new URL(window.location.href), guardado);
    if (!slug) return;
    try {
      const res = await fetch(rutaMarca(slug), { headers: { Accept: "application/json" } });
      if (!res.ok) return;
      const m = (await res.json()) as MarcaPublica;
      if (!m.personalizada) return;
      const css = cssDeTokens(m.tokens);
      if (css) {
        const el = document.createElement("style");
        el.id = "edisys-marca";
        el.textContent = css;
        document.head.appendChild(el);
      }
      marca.value = m;
      document.title = `Ingresar · ${m.nombre}`;
    } catch {
      /* sin red: el login sigue con EDISYS */
    }
  });

  // Correo recordado en este dispositivo (nunca la clave).
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    try {
      const guardado = localStorage.getItem(CLAVE_RECORDAR);
      if (guardado && !correo.value) {
        correo.value = guardado;
        claveRef.value?.focus();
      }
    } catch {
      /* almacenamiento bloqueado: no pasa nada */
    }
  });

  // Cuenta regresiva del bloqueo por intentos (429).
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(({ track, cleanup }) => {
    const hasta = track(() => bloqueadoHasta.value);
    if (!hasta) return;
    const tic = () => {
      const ms = hasta - Date.now();
      restanteMin.value = ms > 0 ? Math.ceil(ms / 60000) : 0;
      if (ms <= 0) {
        bloqueadoHasta.value = 0;
        if (error.value?.esperaMin) error.value = null;
      }
    };
    tic();
    const id = setInterval(tic, 5000);
    cleanup(() => clearInterval(id));
  });

  const enviar = $(async () => {
    if (enviando.value || bloqueadoHasta.value > Date.now()) return;
    nota.value = null;
    const invalido = validar(correo.value, clave.value);
    if (invalido) {
      error.value = invalido;
      (invalido.campos?.correo ? correoRef.value : claveRef.value)?.focus();
      return;
    }
    enviando.value = true;
    error.value = null;
    try {
      const res = await fetch(RUTA_LOGIN_API, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", Accept: "application/json", "X-EDISYS": "1" },
        body: JSON.stringify({ correo: correo.value.trim(), clave: clave.value }),
      });
      if (res.ok) {
        try {
          if (recordar.value) localStorage.setItem(CLAVE_RECORDAR, correo.value.trim());
          else localStorage.removeItem(CLAVE_RECORDAR);
        } catch {
          /* sin almacenamiento */
        }
        // La cookie HttpOnly ya la puso el API; el botón sigue en «cargando» hasta salir.
        window.location.assign(destino || DESTINO_POR_DEFECTO);
        return;
      }
      let cuerpo: unknown = null;
      try {
        cuerpo = await res.json();
      } catch {
        /* sin cuerpo JSON */
      }
      const e = errorDesdeRespuesta(res.status, cuerpo, res.headers);
      error.value = e;
      if (e.esperaMin) bloqueadoHasta.value = Date.now() + e.esperaMin * 60000;
      if (e.campos?.correo) correoRef.value?.focus();
      else if (e.campos?.clave || res.status === 401) claveRef.value?.select();
    } catch {
      error.value = ERROR_SIN_CONEXION;
    }
    enviando.value = false;
  });

  const avisarOlvido = $(() => {
    error.value = null;
    nota.value =
      "Pide a la administración de tu edificio que te reenvíe la invitación; con ese enlace fijas una clave nueva.";
  });
  const avisarWhatsApp = $(() => {
    error.value = null;
    nota.value =
      "El ingreso con código por WhatsApp llega en la siguiente etapa. Por ahora entra con tu correo y contraseña.";
  });

  const errCorreo = error.value?.campos?.correo;
  const errClave = error.value?.campos?.clave;
  const bloqueado = bloqueadoHasta.value > 0 && restanteMin.value > 0;
  const deshabilitado = enviando.value || bloqueado;

  const campo =
    "h-11 w-full rounded-control border bg-superficie px-3.5 text-[15px] text-tinta placeholder:text-texto-apoyo transition-colors duration-rapida " +
    "focus:outline-none focus:ring-1 disabled:bg-fondo disabled:text-texto-apoyo";
  const campoOk = "border-borde-fuerte focus:border-acento focus:ring-acento";
  const campoMal = "border-alerta focus:border-alerta focus:ring-alerta";

  return (
    <div
      class="relative flex min-h-screen items-center justify-center overflow-hidden bg-tinta px-4 py-6 font-sans"
      style={colorValido(marca.value?.color_fondo_login) ? { backgroundColor: marca.value!.color_fondo_login } : undefined}
    >
      <div aria-hidden="true" class="pointer-events-none absolute -left-24 -top-24 h-72 w-72 rounded-full bg-acento-oscuro opacity-10 blur-3xl" />
      <div aria-hidden="true" class="pointer-events-none absolute -bottom-32 -right-16 h-80 w-80 rounded-full bg-acento opacity-20 blur-3xl" />
      <Particulas cantidad={30} />

      <main class="relative z-10 flex w-full justify-center">
        <form
          method="post"
          action={loc.url.pathname + loc.url.search}
          noValidate
          preventdefault:submit
          onSubmit$={enviar}
          aria-busy={enviando.value}
          class={["flex w-full max-w-[344px] flex-col gap-4 rounded-tarjeta border border-borde bg-superficie px-5 py-6 shadow-flotante transition-opacity duration-media animate-aparecer sm:px-6", enviando.value ? "opacity-80" : ""]}
        >
          <input type="hidden" name="next" value={destino} />

          <div class="flex flex-col items-center gap-3 text-center">
            {marca.value ? (
              // marca: I2 · logo o nombre de la administradora
              <div class="flex flex-col items-center gap-1.5">
                {marca.value.logo_url ? (
                  <img src={marca.value.logo_url} alt={marca.value.nombre} width={160} height={48} class="max-h-12 w-auto max-w-[200px] object-contain" />
                ) : (
                  <span class="text-xl font-semibold tracking-tight text-tinta">{marca.value.nombre}</span>
                )}
                {marca.value.lema && <span class="text-xs text-texto-apoyo">{marca.value.lema}</span>}
              </div>
            ) : (
              <a href="/" class="flex items-center gap-2.5 rounded-lg" aria-label="EDISYS, ir a la página principal">
                <svg width="34" height="34" viewBox="0 0 32 32" aria-hidden="true">
                  <rect width="32" height="32" rx="8" class="fill-acento" />
                  <path
                    d="M8 24h16M10.5 19h11M13 14h6M16 9v0.01"
                    stroke="#FFFFFF"
                    stroke-width="2.4"
                    stroke-linecap="round"
                    fill="none"
                  />
                </svg>
                <span class="text-xl font-semibold tracking-[0.08em] text-tinta">EDISYS</span>
              </a>
            )}
            <h1 class="text-sm font-normal text-texto-suave">Accede a tu edificio</h1>
          </div>

          {error.value && (
            <div
              role="alert"
              class="rounded-control border border-alerta-borde bg-alerta-suave p-4 text-sm leading-normal text-alerta-texto animate-desplegar"
            >
              <p class="font-semibold">
                {bloqueado
                  ? `Demasiados intentos. Podrás volver a intentar en ${restanteMin.value} ${restanteMin.value === 1 ? "minuto" : "minutos"}.`
                  : error.value.mensaje}
              </p>
              {error.value.idPeticion && (
                <p class="mt-1 text-alerta">
                  Código para soporte: <span class="font-mono tabular-nums">{error.value.idPeticion}</span>
                </p>
              )}
            </div>
          )}
          {nota.value && (
            <div
              role="status"
              class="rounded-control border border-acento-borde bg-acento-suave p-4 text-sm leading-normal text-acento-hover animate-desplegar"
            >
              {nota.value}
            </div>
          )}

          <fieldset disabled={enviando.value} class="flex flex-col gap-3">
            <div class="flex flex-col gap-1.5">
              <label for="correo" class="sr-only">
                Correo o DNI
              </label>
              <input
                ref={correoRef}
                id="correo"
                name="correo"
                type="text"
                inputMode="email"
                autoComplete="username"
                autoCapitalize="none"
                spellcheck={false}
                autoFocus
                required
                bind:value={correo}
                aria-invalid={errCorreo ? "true" : undefined}
                aria-describedby={errCorreo ? "correo-error" : undefined}
                class={[campo, errCorreo ? campoMal : campoOk]}
                placeholder="Correo o DNI"
              />
              {errCorreo && (
                <p id="correo-error" class="text-sm text-alerta">
                  {errCorreo}
                </p>
              )}
            </div>

            <div class="flex flex-col gap-1.5">
              <label for="clave" class="sr-only">
                Contraseña
              </label>
              <div class="relative">
                <input
                  ref={claveRef}
                  id="clave"
                  name="clave"
                  type={verClave.value ? "text" : "password"}
                  autoComplete="current-password"
                  required
                  bind:value={clave}
                  aria-invalid={errClave ? "true" : undefined}
                  aria-describedby={errClave ? "clave-error" : undefined}
                  class={[campo, "pr-12", errClave ? campoMal : campoOk]}
                  placeholder="Contraseña"
                />
                <button
                  type="button"
                  onClick$={() => (verClave.value = !verClave.value)}
                  aria-pressed={verClave.value}
                  aria-controls="clave"
                  aria-label={verClave.value ? "Ocultar contraseña" : "Mostrar contraseña"}
                  class="absolute inset-y-0 right-0 flex w-11 items-center justify-center rounded-r-lg text-texto-suave hover:text-tinta focus-visible:outline focus-visible:outline-2 focus-visible:outline-acento"
                >
                  <IconoOjo abierto={!verClave.value} />
                </button>
              </div>
              {errClave && (
                <p id="clave-error" class="text-sm text-alerta">
                  {errClave}
                </p>
              )}
            </div>

            <div class="flex items-center justify-between gap-3">
              <label class="flex min-h-8 items-center gap-2 text-sm text-texto-suave">
                <input
                  type="checkbox"
                  name="recordar"
                  bind:checked={recordar}
                  class="h-4 w-4 rounded border-borde-fuerte accent-[var(--color-acento)]"
                />
                <span>Recordarme</span>
              </label>
              <button
                type="button"
                onClick$={avisarOlvido}
                class="text-sm text-acento underline-offset-2 hover:text-acento-hover hover:underline"
              >
                ¿Olvidaste tu clave?
              </button>
            </div>
          </fieldset>

          <button
            type="submit"
            disabled={deshabilitado}
            class="flex h-11 items-center justify-center gap-2 rounded-control bg-acento text-[15px] font-semibold text-white transition-[background-color,transform] duration-rapida hover:bg-acento-hover enabled:active:scale-98 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-acento disabled:cursor-not-allowed disabled:opacity-70"
          >
            {enviando.value && <Spinner />}
            {enviando.value ? "Ingresando…" : "Ingresar"}
          </button>

          <button
            type="button"
            onClick$={avisarWhatsApp}
            disabled={enviando.value}
            class="self-center text-sm font-medium text-acento underline-offset-2 hover:text-acento-hover hover:underline disabled:opacity-60"
          >
            Recibir código por WhatsApp
          </button>

          <p class="text-center text-xs text-texto-apoyo">
            ¿No tienes cuenta? <a href="/#contacto">Pide tu acceso</a>
          </p>
        </form>
      </main>
    </div>
  );
});

export const head: DocumentHead = {
  title: "Ingresar · EDISYS",
  meta: [
    {
      name: "description",
      content: "Ingresa a EDISYS: recibos, balance, reservas y mantenimiento de tu edificio.",
    },
  ],
};
