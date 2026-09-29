# EDISYS · atajos del motor (F0). Se corre desde motor/.
COMPOSE := docker compose

.PHONY: descargar benchmark benchmark-todos resultados abajo logs prep

prep:          ## Prepara un EC2 de 3 GB recién creado (docker, swap, sysctl) — con sudo
	./scripts/ec2-preparar.sh

descargar:     ## Baja los 3 GGUF candidatos + e5-small a models/
	./scripts/descargar-modelos.sh models

benchmark:     ## Corre el benchmark de UN modelo: make benchmark MODELO=qwen25-1.5b
	./scripts/benchmark.sh $(or $(MODELO),qwen25-1.5b)

benchmark-todos:  ## Corre el benchmark de los 3 candidatos, uno por uno
	./scripts/benchmark.sh todos

resultados:    ## Muestra el resumen acumulado
	@test -s resultados/RESUMEN.md && cat resultados/RESUMEN.md || echo "Aún no hay resultados: make benchmark"

abajo:         ## Apaga llama-server
	$(COMPOSE) down

logs:          ## Logs de llama-server
	$(COMPOSE) logs -f --tail=100 llama
