#!/usr/bin/bash
# run_e2.sh
# Levanta los 4 pares eNB+UE de E2 secuencialmente
# Cada eNB espera confirmación S1 antes de levantar su UE

set -euo pipefail

COMPOSE="docker compose"
RAN="ran.yaml"
SCRIPTS_DIR="$(dirname "$0")"

echo "  Par 1 — eNB1 + UE válido (IMSI 895)..."
$COMPOSE -f $RAN --profile ran-4g-srs up -d srsenb_zmq
bash "$SCRIPTS_DIR/wait_ran.sh" srsenb_zmq "eNB started"
$COMPOSE -f $RAN --profile ran-4g-srs up -d srsue_zmq
bash "$SCRIPTS_DIR/wait_ran.sh" srsue_zmq "Network attach successful"

echo "  Par 2 — eNB2 + UE bad_ki (IMSI 902)..."
$COMPOSE -f $RAN --profile ran-4g-e2 up -d srsenb_zmq2
bash "$SCRIPTS_DIR/wait_ran.sh" srsenb_zmq2 "eNB started"
$COMPOSE -f $RAN --profile ran-4g-e2 up -d srsue_zmq_bad_ki

echo "  Par 3 — eNB3 + UE bad_imsi (IMSI 901)..."
$COMPOSE -f $RAN --profile ran-4g-e2 up -d srsenb_zmq3
bash "$SCRIPTS_DIR/wait_ran.sh" srsenb_zmq3 "eNB started"
$COMPOSE -f $RAN --profile ran-4g-e2 up -d srsue_zmq_bad_imsi

echo "  Par 4 — eNB4 + UE bad_apn (IMSI 903)..."
$COMPOSE -f $RAN --profile ran-4g-e2 up -d srsenb_zmq4
bash "$SCRIPTS_DIR/wait_ran.sh" srsenb_zmq4 "eNB started"
$COMPOSE -f $RAN --profile ran-4g-e2 up -d srsue_zmq_bad_apn

echo "  ✓ 4 pares eNB+UE levantados"
