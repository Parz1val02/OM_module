#!/usr/bin/bash
# wait_core.sh <generation>
# Espera que los NFs core principales estén en estado running
# Uso: bash scripts/wait_core.sh 4g

set -euo pipefail

GENERATION="${1:-4g}"
TIMEOUT=60
INTERVAL=3

if [ "$GENERATION" = "4g" ]; then
    NFS=("mme" "hss" "sgwc" "sgwu" "smf" "upf" "pcrf")
else
    NFS=("amf" "smf" "upf" "ausf" "udm" "udr" "pcf" "nrf" "scp" "bsf" "nssf")
fi

echo "  Esperando NFs core ${GENERATION^^}..."

elapsed=0
for nf in "${NFS[@]}"; do
    waited=0
    while true; do
        status=$(docker inspect --format='{{.State.Status}}' "$nf" 2>/dev/null || echo "not_found")
        if [ "$status" = "running" ]; then
            echo "    ✓ $nf running"
            break
        fi
        if [ $waited -ge $TIMEOUT ]; then
            echo "    ✗ Timeout esperando $nf (status: $status)"
            exit 1
        fi
        sleep $INTERVAL
        waited=$((waited + INTERVAL))
    done
done

echo "  Todos los NFs core ${GENERATION^^} están running"
