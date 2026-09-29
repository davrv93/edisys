-- Aprendizaje del chatbot: cada intercambio, el feedback 👍/👎, las golden
-- (pregunta → intención aprendida del edificio) y el uso del LLM por día.
-- Las cifras siempre salen de la base; lo que se aprende es a clasificar.
CREATE TABLE chatbot_mensaje (
  id          bigserial PRIMARY KEY,
  edificio_id bigint NOT NULL REFERENCES edificio(id),
  telefono    text NOT NULL DEFAULT '',
  texto       text NOT NULL,
  respuesta   text NOT NULL DEFAULT '',
  intencion   text NOT NULL DEFAULT 'no_entendi',
  origen      text NOT NULL DEFAULT 'reglas' CHECK (origen IN ('reglas','golden','motor','llm')),
  creado_en   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX chatbot_mensaje_edificio_idx ON chatbot_mensaje (edificio_id, creado_en DESC);

CREATE TABLE chatbot_feedback (
  mensaje_id  bigint PRIMARY KEY REFERENCES chatbot_mensaje(id) ON DELETE CASCADE,
  edificio_id bigint NOT NULL REFERENCES edificio(id),
  valor       text NOT NULL CHECK (valor IN ('util','mal')),
  creado_en   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE chatbot_golden (
  id              bigserial PRIMARY KEY,
  edificio_id     bigint NOT NULL REFERENCES edificio(id),
  pregunta        text NOT NULL,
  norma           text NOT NULL,
  intencion       text NOT NULL,
  confirmaciones  int NOT NULL DEFAULT 0,
  activa          boolean NOT NULL DEFAULT true,
  creado_en       timestamptz NOT NULL DEFAULT now(),
  actualizado_en  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (edificio_id, norma)
);

CREATE TABLE chatbot_llm_uso (
  dia         date NOT NULL,
  edificio_id bigint NOT NULL REFERENCES edificio(id),
  modelo      text NOT NULL,
  llamadas    int NOT NULL DEFAULT 0,
  fallos      int NOT NULL DEFAULT 0,
  PRIMARY KEY (dia, edificio_id, modelo)
);
