-- 0014 · Motor conversacional (plan §4 y §5 F8): ajuste por edificio, banco de
-- GoldenSQL con auditoría, registro de consultas y permiso de administración.
-- Nada de datos personales en el motor (Ley 29733).
ALTER TABLE edificio ADD COLUMN IF NOT EXISTS config_json jsonb NOT NULL DEFAULT '{}';
-- motor_activado: 'off' (defecto) | 'solo_admin' | 'todos'
CREATE TABLE IF NOT EXISTS motor_golden_sql (
  id           bigserial PRIMARY KEY,
  edificio_id  bigint REFERENCES edificio(id),
  pregunta     text NOT NULL,
  sql          text NOT NULL,
  tablas       text[] NOT NULL DEFAULT '{}',
  fuente       text NOT NULL DEFAULT 'manual' CHECK (fuente IN ('manual','generada_aprobada')),
  creado_por   bigint REFERENCES usuario(id),
  verifico_en  timestamptz,
  veces_usada  int NOT NULL DEFAULT 0,
  desactivada  boolean NOT NULL DEFAULT false,
  creado_en    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_motor_golden_edificio ON motor_golden_sql (edificio_id);

CREATE TABLE IF NOT EXISTS motor_consulta (
  id           bigserial PRIMARY KEY,
  edificio_id  bigint REFERENCES edificio(id),
  golden_id    bigint REFERENCES motor_golden_sql(id),
  pregunta     text NOT NULL,
  sql          text NOT NULL DEFAULT '',
  filas        int NOT NULL DEFAULT 0,
  estado       text NOT NULL,
  duracion_ms  int NOT NULL DEFAULT 0,
  creado_en    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_motor_consulta_edificio ON motor_consulta (edificio_id, creado_en);

INSERT INTO permiso (codigo, modulo, descripcion, ajustable)
  VALUES ('motor.administrar', 'whatsapp', 'Probar el motor conversacional y sus golden SQL', false)
  ON CONFLICT (codigo) DO NOTHING;

INSERT INTO rol_permiso (rol, permiso) VALUES
  ('superadmin', 'motor.administrar'),
  ('administrador', 'motor.administrar')
  ON CONFLICT DO NOTHING;
