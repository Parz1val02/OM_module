#!/usr/bin/bash
# traffic.sh
# Detecta qué contenedores UE están corriendo y ejecuta ping continuo
# en los que tienen interfaz de datos activa (tun_srsue o uesimtun0)

set -euo pipefail

GW="8.8.8.8"
PING_INTERVAL="0.2"
PING_SIZE="1400"

# UEs srsRAN — interfaz tun_srsue
SRS_UES=("srsue_zmq" "srsue_5g_zmq")

# UEs UERANSIM — interfaz uesimtun0
UERANSIM_UES=("nr_ue" "nr_ue2" "nr_ue3")

started=0

ping_ue() {
    local container="$1"
    local iface="$2"
    echo "  ▶ Tráfico en $container ($iface)..."
    docker exec -d "$container" ping -I "$iface" -i "$PING_INTERVAL" -s "$PING_SIZE" "$GW" 2>/dev/null
    echo "    ✓ Ping iniciado en $container"
}

echo "▶ Iniciando tráfico en UEs activos..."

# srsRAN UEs
for ue in "${SRS_UES[@]}"; do
    status=$(docker inspect --format='{{.State.Status}}' "$ue" 2>/dev/null || echo "not_found")
    if [ "$status" = "running" ]; then
        # Verificar que la interfaz tun_srsue existe
        if docker exec "$ue" ip link show tun_srsue &>/dev/null; then
            ping_ue "$ue" "tun_srsue"
            started=$((started + 1))
        else
            echo "  ⚠ $ue está running pero tun_srsue no existe aún (¿attach completado?)"
        fi
    fi
done

# UERANSIM UEs
for ue in "${UERANSIM_UES[@]}"; do
    status=$(docker inspect --format='{{.State.Status}}' "$ue" 2>/dev/null || echo "not_found")
    if [ "$status" = "running" ]; then
        # Verificar que la interfaz uesimtun0 existe
        if docker exec "$ue" ip link show uesimtun0 &>/dev/null; then
            ping_ue "$ue" "uesimtun0"
            started=$((started + 1))
        else
            echo "  ⚠ $ue está running pero uesimtun0 no existe aún (¿PDU session establecida?)"
        fi
    fi
done

if [ $started -eq 0 ]; then
    echo "  ✗ No se encontraron UEs activos con interfaz de datos lista"
    echo "    Verifica que los contenedores UE están corriendo y el attach/registro completó"
    exit 1
fi

echo "✅ Tráfico iniciado en $started UE(s)"
echo "   Para detener: docker exec <container> pkill ping"
