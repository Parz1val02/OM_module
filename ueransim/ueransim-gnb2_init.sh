#!/bin/bash
export IP_ADDR=$(awk 'END{print $1}' /etc/hosts)

cp /mnt/ueransim/${COMPONENT_NAME}.yaml /UERANSIM/config/${COMPONENT_NAME}.yaml

sed -i 's|MNC|'$MNC'|g' /UERANSIM/config/${COMPONENT_NAME}.yaml
sed -i 's|MCC|'$MCC'|g' /UERANSIM/config/${COMPONENT_NAME}.yaml
sed -i 's|TAC|'$TAC'|g' /UERANSIM/config/${COMPONENT_NAME}.yaml
sed -i 's|NR_GNB2_IP|'$NR_GNB2_IP'|g' /UERANSIM/config/${COMPONENT_NAME}.yaml
sed -i 's|AMF_IP|'$AMF_IP'|g' /UERANSIM/config/${COMPONENT_NAME}.yaml

./nr-gnb -c ../config/${COMPONENT_NAME}.yaml &
exec bash $@
