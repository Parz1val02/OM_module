#!/usr/bin/bash
# run_e4.sh
# Levanta los 3 gNBs de E4 secuencialmente y luego los 4 UEs válidos
# Los gNBs deben levantarse de a uno para evitar race conditions en NGAP

set -euo pipefail

COMPOSE="docker compose"
RAN="ran.yaml"
SCRIPTS_DIR="$(dirname "$0")"

echo "  Fase 1 — gNB srsRAN (referencia SST=1)..."
$COMPOSE -f $RAN --profile ran-5g-srs up -d srsgnb_zmq
bash "$SCRIPTS_DIR/wait_ran.sh" srsgnb_zmq "gNB started"

echo "  Fase 1 — gNB UERANSIM nr_gnb (SST=1)..."
$COMPOSE -f $RAN --profile ran-5g-ueransim up -d nr_gnb
bash "$SCRIPTS_DIR/wait_ran.sh" nr_gnb "NG Setup procedure is successful"

echo "  Fase 1 — gNB UERANSIM nr_gnb2 (SST=1+2)..."
$COMPOSE -f $RAN --profile ran-5g-e4 up -d nr_gnb2
bash "$SCRIPTS_DIR/wait_ran.sh" nr_gnb2 "NG Setup procedure is successful"

echo "  ✓ 3 gNBs registrados"

echo "  Fase 2 — UEs válidos..."
$COMPOSE -f $RAN --profile ran-5g-srs up -d srsue_5g_zmq
bash "$SCRIPTS_DIR/wait_ran.sh" srsue_5g_zmq "PDU Session establishment is successful"

$COMPOSE -f $RAN --profile ran-5g-ueransim up -d nr_ue
bash "$SCRIPTS_DIR/wait_ran.sh" nr_ue "PDU Session establishment is successful"

$COMPOSE -f $RAN --profile ran-5g-e4 up -d nr_ue2
bash "$SCRIPTS_DIR/wait_ran.sh" nr_ue2 "PDU Session establishment is successful"

$COMPOSE -f $RAN --profile ran-5g-e4 up -d nr_ue3
bash "$SCRIPTS_DIR/wait_ran.sh" nr_ue3 "PDU Session establishment is successful"

echo "  ✓ 4 UEs válidos levantados"
