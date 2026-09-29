# EDISYS · atajos. Todo corre con docker compose (proyecto «edisys»).
COMPOSE := docker compose
TEST_DB ?= postgres://edisys:edisys@localhost:4754/edisys_test?sslmode=disable

.PHONY: up down logs seed seed-demo test test-unit ps smoke build api backup restore respaldos validar-ubl motor motor-on motor-off humo-motor validar-deploy deploy-local peso

# Tope recomendado del dist en bytes: base 528950 (4e1acab) +30 %.
PESO_TOPE ?= 688000

up:            ## Construye y levanta todo (edge en http://localhost:4700)
	$(COMPOSE) up -d --build

build:
	$(COMPOSE) build

api:           ## Reconstruye el API, aplica migraciones y recrea solo migrate + api (el resto sigue arriba)
	$(COMPOSE) build migrate
	$(COMPOSE) up -d --no-deps --force-recreate migrate
	@until [ "$$(docker inspect -f '{{.State.Status}}' edisys_migrate)" = exited ]; do sleep 1; done
	@test "$$(docker inspect -f '{{.State.ExitCode}}' edisys_migrate)" = 0 || (docker logs --tail 20 edisys_migrate; exit 1)
	$(COMPOSE) up -d --no-deps --force-recreate api

down:          ## Apaga (los volúmenes se conservan)
	$(COMPOSE) down

logs:          ## Logs de todos los servicios
	$(COMPOSE) logs -f --tail=100

ps:
	$(COMPOSE) ps

seed:          ## Borra y vuelve a sembrar el Edificio Demo
	$(COMPOSE) run --rm -e SEMBRAR=siempre migrate preparar

seed-demo:     ## Semilla con 2 lecturas de setiembre pendientes (para tomarlas en vivo)
	$(COMPOSE) run --rm -e SEMBRAR=siempre -e SEMBRAR_PENDIENTES=2 migrate preparar

test-unit:     ## Pruebas puras (sin base)
	cd api && go test ./internal/reparto/ ./internal/mantenimiento/ ./internal/chatbot/

test:          ## Todas las pruebas; las de integración usan la base edisys_test del compose
	$(COMPOSE) up -d postgres
	cd api && TEST_DATABASE_URL="$(TEST_DB)" go test -count=1 ./...

smoke:         ## Pruebas de humo con curl contra el stack levantado
	./scripts/smoke.sh

backup:        ## Respaldo ahora (base + archivos) en el volumen edisys_respaldos
	$(COMPOSE) run --rm --no-deps backup respaldar.sh

respaldos:     ## Lista los respaldos
	$(COMPOSE) run --rm --no-deps --entrypoint sh backup -c 'ls -1 /respaldos'

restore:       ## Restaura FECHA=AAAAMMDD-HHMM (o «ultimo») en una base temporal y compara el conteo de filas
	$(COMPOSE) run --rm --no-deps backup restaurar.sh $(or $(FECHA),ultimo)

validar-ubl:   ## Valida boleta/factura/nota de crédito contra los XSD de UBL 2.1 y verifica la firma con xmlsec1
	./scripts/validar-ubl.sh

motor:         ## Baja los modelos del motor (~3 GB) y activa el servicio (descomenta MOTOR_URL)
	./motor/scripts/descargar-modelos.sh motor/models
	@sed -i.bak 's/^  # MOTOR_URL:/  MOTOR_URL:/' docker-compose.yml && rm -f docker-compose.yml.bak
	@echo "Listo: make up levantará motor+llama. Motor ON: make motor-on · OFF: make motor-off"

motor-on:      ## Enciende el motor para todos los edificios (motor_activado=todos)
	docker compose --profile motor up -d motor
	docker compose exec postgres psql -U edisys -d edisys -c "UPDATE edificio SET config_json = COALESCE(config_json,'{}'::jsonb) || '{\"motor_activado\":\"todos\"}'::jsonb"

motor-off:     ## Apaga el motor (vuelve al chatbot por reglas)
	docker compose exec postgres psql -U edisys -d edisys -c "UPDATE edificio SET config_json = COALESCE(config_json,'{}'::jsonb) || '{\"motor_activado\":\"off\"}'::jsonb"
	docker compose --profile motor stop motor llama

humo-motor:    ## Humo del motor: salud del motor y pregunta que las reglas no entienden
	./scripts/humo-motor.sh

validar-deploy: ## Pre-vuelo del despliegue sin remoto: sintaxis, compose, workflows y aviso de lo que falta (EC2)
	bash -n scripts/desplegar.sh && echo "desplegar.sh: sintaxis OK"
	$(COMPOSE) config -q && echo "compose: config OK"
	python3 -c "import yaml; [yaml.safe_load(open(f)) for f in ['.github/workflows/ci.yml', '.github/workflows/deploy.yml']]; print('workflows: YAML OK')"
	test -z "$$(git remote)" && echo "sin remoto ni EC2: la vía real es make deploy-local; el EC2 espera EC2_HOST/EC2_USER/EC2_SSH_KEY" || git remote -v

deploy-local:   ## Despliegue local: construye, respalda, migra (sin sembrar) y levanta con espera de salud
	$(COMPOSE) build
	$(COMPOSE) run --rm --no-deps backup respaldar.sh "predespliegue-$$(date +%Y%m%d-%H%M)"
	SEMBRAR=no $(COMPOSE) run --rm migrate preparar
	$(COMPOSE) up -d --remove-orphans
	for i in $$(seq 1 40); do curl -fs http://localhost:4700/api/health >/dev/null && break; sleep 3; done
	curl -fs http://localhost:4700/api/health && echo "deploy local: OK"

peso:           ## Construye la app y verifica que el dist no pase el tope recomendado
	cd app && npm run build
	test $$(find app/dist -type f -exec cat {} + | wc -c) -le $(PESO_TOPE) && echo "peso OK: dentro de $(PESO_TOPE) bytes"
