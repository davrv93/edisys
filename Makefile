# EDISYS · atajos. Todo corre con docker compose (proyecto «edisys»).
COMPOSE := docker compose
TEST_DB ?= postgres://edisys:edisys@localhost:4754/edisys_test?sslmode=disable

.PHONY: up down logs seed seed-demo test test-unit ps smoke build api

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
