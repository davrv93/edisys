# EDISYS · atajos. Todo corre con docker compose (proyecto «edisys»).
COMPOSE := docker compose
TEST_DB ?= postgres://edisys:edisys@localhost:4754/edisys_test?sslmode=disable

.PHONY: up down logs seed seed-demo test test-unit ps smoke build api backup restore respaldos validar-ubl validar-deploy

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

validar-deploy: ## Pre-vuelo del despliegue sin remoto: sintaxis, compose, workflows y aviso de lo que falta (EC2)
	bash -n scripts/desplegar.sh && echo "desplegar.sh: sintaxis OK"
	$(COMPOSE) config -q && echo "compose: config OK"
	python3 -c "import yaml; [yaml.safe_load(open(f)) for f in ['.github/workflows/ci.yml', '.github/workflows/deploy.yml']]; print('workflows: YAML OK')"
	test -z "$$(git remote)" && echo "sin remoto ni EC2: el deploy real espera EC2_HOST/EC2_USER/EC2_SSH_KEY y /opt/edisys con su .env" || git remote -v
