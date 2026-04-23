#!/usr/bin/bash
# wait_ran.sh <container> <mensaje>
# Espera que aparezca un mensaje específico en los logs del contenedor
# Uso: bash scripts/wait_ran.sh srsgnb_zmq "gNB-N2 Setup Procedure completed"

set -euo pipefail

CONTAINER="${1}"
MESSAGE="${2}"
TIMEOUT=120
INTERVAL=2

echo "  Esperando '$MESSAGE' en $CONTAINER..."

elapsed=0
while true; do
    # Verificar que el contenedor existe y está running
    status=$(docker inspect --format='{{.State.Status}}' "$CONTAINER" 2>/dev/null || echo "not_found")
    if [ "$status" = "not_found" ]; then
        echo "    ✗ Contenedor $CONTAINER no encontrado"
        exit 1
    fi

    # Buscar el mensaje en los logs
    if docker logs "$CONTAINER" 2>&1 | grep -q "$MESSAGE"; then
        echo "    ✓ $CONTAINER: '$MESSAGE'"
        break
    fi

    if [ $elapsed -ge $TIMEOUT ]; then
        echo "    ✗ Timeout esperando '$MESSAGE' en $CONTAINER"
        echo "    Últimas líneas de log:"
        docker logs --tail 10 "$CONTAINER" 2>&1 | sed 's/^/      /'
        exit 1
    fi

    sleep $INTERVAL
    elapsed=$((elapsed + INTERVAL))
done
