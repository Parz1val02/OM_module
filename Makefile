# ─────────────────────────────────────────────────────────────────────────────
# Módulo O&M — Makefile para automatización de escenarios
# Uso: make <target>
# ─────────────────────────────────────────────────────────────────────────────

COMPOSE      := docker compose
CORE_4G      := 4G_core.yaml
CORE_5G      := 5G_core.yaml
RAN          := ran.yaml
SERVICES     := services.yaml
SCRIPTS_DIR  := scripts

.PHONY: help \
        services-up services-down \
        core-4g-up core-4g-down \
        core-5g-up core-5g-down \
        e1 e2 e3 e3-ueransim e4 \
        e1-down e2-down e3-down e3-ueransim-down e4-down \
        traffic down

# ── Ayuda ────────────────────────────────────────────────────────────────────

help:
	@echo ""
	@echo "Módulo O&M — Testbed 4G/5G"
	@echo "────────────────────────────────────────────────────"
	@echo "  Orden de arranque recomendado:"
	@echo "    1. make core-4g-up   o   make core-5g-up"
	@echo "    2. make services-up"
	@echo "    3. make e1 / e2 / e3 / e3-ueransim / e4"
	@echo "    4. make traffic"
	@echo ""
	@echo "  Core"
	@echo "    make core-4g-up/down      Core 4G (Open5GS EPC)"
	@echo "    make core-5g-up/down      Core 5G (Open5GS 5GC)"
	@echo ""
	@echo "  Servicios O&M"
	@echo "    make services-up          Stack observabilidad (levantar después del core)"
	@echo "    make services-down        Bajar stack observabilidad"
	@echo ""
	@echo "  Escenarios (solo RAN — core y servicios deben estar activos)"
	@echo "    make e1                   E1 — Flujo base 4G"
	@echo "    make e2                   E2 — Multi-eNB UEs mixtos 4G"
	@echo "    make e3                   E3 — Flujo base 5G (srsRAN)"
	@echo "    make e3-ueransim          E3 — Flujo base 5G (UERANSIM)"
	@echo "    make e4                   E4 — Multi-gNB Slicing 5G"
	@echo ""
	@echo "  Teardown por escenario (solo baja RAN)"
	@echo "    make e1-down ... e4-down"
	@echo ""
	@echo "  Utilidades"
	@echo "    make traffic              Ping en todos los UEs activos"
	@echo "    make down                 Bajar todo (RAN + core + servicios)"
	@echo ""

# ── Servicios O&M ─────────────────────────────────────────────────────────────

services-up:
	@echo "▶ Levantando stack de observabilidad..."
	$(COMPOSE) -f $(SERVICES) up --build -d
	@echo "✅ Servicios O&M activos"

services-down:
	@echo "▶ Bajando stack de observabilidad..."
	$(COMPOSE) -f $(SERVICES) down
	@echo "✅ Servicios O&M detenidos"

# ── Core 4G ──────────────────────────────────────────────────────────────────

core-4g-up:
	@echo "▶ Levantando core 4G (Open5GS EPC)..."
	$(COMPOSE) -f $(CORE_4G) up -d
	@bash $(SCRIPTS_DIR)/wait_core.sh 4g
	@echo "✅ Core 4G listo"

core-4g-down:
	@echo "▶ Bajando core 4G..."
	$(COMPOSE) -f $(CORE_4G) down
	@echo "✅ Core 4G detenido"

# ── Core 5G ──────────────────────────────────────────────────────────────────

core-5g-up:
	@echo "▶ Levantando core 5G (Open5GS 5GC)..."
	$(COMPOSE) -f $(CORE_5G) up -d
	@bash $(SCRIPTS_DIR)/wait_core.sh 5g
	@echo "✅ Core 5G listo"

core-5g-down:
	@echo "▶ Bajando core 5G..."
	$(COMPOSE) -f $(CORE_5G) down
	@echo "✅ Core 5G detenido"

# ── E1 — Flujo base 4G ───────────────────────────────────────────────────────

e1:
	@echo "▶ Levantando RAN E1 (srsRAN 4G)..."
	$(COMPOSE) -f $(RAN) --profile ran-4g-srs up -d srsenb_zmq
	@bash $(SCRIPTS_DIR)/wait_ran.sh srsenb_zmq "eNodeB started"
	$(COMPOSE) -f $(RAN) --profile ran-4g-srs up -d srsue_zmq
	@bash $(SCRIPTS_DIR)/wait_ran.sh srsue_zmq "Network attach successful"
	@echo "✅ E1 listo — UE attached y Bearer establecido"

e1-down:
	@echo "▶ Bajando RAN E1..."
	$(COMPOSE) -f $(RAN) --profile ran-4g-srs down
	@echo "✅ RAN E1 detenido"

# ── E2 — Multi-eNB UEs mixtos 4G ─────────────────────────────────────────────

e2:
	@echo "▶ Levantando RAN E2 (multi-eNB + UEs mixtos 4G)..."
	@bash $(SCRIPTS_DIR)/run_e2.sh
	@echo "✅ E2 listo — eNB+UE activo + 3 pares ENB+UE inválidos"

e2-down:
	@echo "▶ Bajando RAN E2..."
	$(COMPOSE) -f $(RAN) --profile ran-4g-srs down
	$(COMPOSE) -f $(RAN) --profile ran-4g-e2 down
	@echo "✅ RAN E2 detenido"

# ── E3 — Flujo base 5G (srsRAN) ──────────────────────────────────────────────

e3:
	@echo "▶ Levantando RAN E3 (srsRAN Project 5G)..."
	$(COMPOSE) -f $(RAN) --profile ran-5g-srs up -d srsgnb_zmq
	@bash $(SCRIPTS_DIR)/wait_ran.sh srsgnb_zmq "gNB started"
	$(COMPOSE) -f $(RAN) --profile ran-5g-srs up -d srsue_5g_zmq
	@bash $(SCRIPTS_DIR)/wait_ran.sh srsue_5g_zmq "PDU Session Establishment successful"
	@echo "✅ E3 listo — UE registrado y PDU session establecida"

e3-down:
	@echo "▶ Bajando RAN E3..."
	$(COMPOSE) -f $(RAN) --profile ran-5g-srs down
	@echo "✅ RAN E3 detenido"

# ── E3-UERANSIM — Flujo base 5G (UERANSIM) ───────────────────────────────────

e3-ueransim:
	@echo "▶ Levantando RAN E3 con UERANSIM..."
	$(COMPOSE) -f $(RAN) --profile ran-5g-ueransim up -d nr_gnb
	@bash $(SCRIPTS_DIR)/wait_ran.sh nr_gnb "NG Setup procedure is successful"
	$(COMPOSE) -f $(RAN) --profile ran-5g-ueransim up -d nr_ue
	@bash $(SCRIPTS_DIR)/wait_ran.sh nr_ue "PDU Session establishment is successful"
	@echo "✅ E3-UERANSIM listo"

e3-ueransim-down:
	@echo "▶ Bajando RAN E3-UERANSIM..."
	$(COMPOSE) -f $(RAN) --profile ran-5g-ueransim down
	@echo "✅ RAN E3-UERANSIM detenido"

# ── E4 — Multi-gNB Slicing 5G ────────────────────────────────────────────────

e4:
	@echo "▶ Levantando RAN E4 (multi-gNB slicing 5G)..."
	@bash $(SCRIPTS_DIR)/run_e4.sh
	@echo "✅ E4 listo — 3 gNBs + 4 UEs válidos + 4 UEs inválidos activos"

e4-down:
	@echo "▶ Bajando RAN E4..."
	$(COMPOSE) -f $(RAN) --profile ran-5g-e4 down
	$(COMPOSE) -f $(RAN) --profile ran-5g-ueransim down
	$(COMPOSE) -f $(RAN) --profile ran-5g-srs down
	@echo "✅ RAN E4 detenido"

# ── Tráfico ──────────────────────────────────────────────────────────────────

traffic:
	@bash $(SCRIPTS_DIR)/traffic.sh

# ── Bajar todo ───────────────────────────────────────────────────────────────

down:
	@echo "▶ Bajando todo el testbed..."
	-$(COMPOSE) -f $(RAN) --profile ran-4g-srs down
	-$(COMPOSE) -f $(RAN) --profile ran-4g-e2 down
	-$(COMPOSE) -f $(RAN) --profile ran-5g-srs down
	-$(COMPOSE) -f $(RAN) --profile ran-5g-ueransim down
	-$(COMPOSE) -f $(RAN) --profile ran-5g-e4 down
	-$(COMPOSE) -f $(SERVICES) down
	-$(COMPOSE) -f $(CORE_4G) down
	-$(COMPOSE) -f $(CORE_5G) down
	@echo "✅ Testbed detenido completamente"
