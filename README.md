# OM_module
RF simulated 4G/5G ran and core based on the docker_open5gs project: https://github.com/herlesupreeth/docker_open5gs

## Get Pre-built Docker images
Pull base open5gs image:
```
docker pull ghcr.io/herlesupreeth/docker_open5gs:master
docker tag ghcr.io/herlesupreeth/docker_open5gs:master docker_open5gs
```
For srsRAN components:
```
docker pull ghcr.io/herlesupreeth/docker_srslte:master
docker tag ghcr.io/herlesupreeth/docker_srslte:master docker_srslte

docker pull ghcr.io/herlesupreeth/docker_srsran:master
docker tag ghcr.io/herlesupreeth/docker_srsran:master docker_srsran
```
For ueransim components:
```
docker pull ghcr.io/herlesupreeth/docker_ueransim:master
docker tag ghcr.io/herlesupreeth/docker_ueransim:master docker_ueransim
```

## Deployments

### 4G core deployment
docker compose -f 4g_core_only.yaml up -d
#### srsRAN ZMQ eNB (RF simulated)
docker compose -f srsenb_zmq.yaml up -d && docker container attach srsenb_zmq
#### srsRAN ZMQ 4G UE (RF simulated)
docker compose -f srsue_4g_zmq.yaml up -d && docker container attach srsue_zmq

### 5G core deployment
docker compose -f 5g_core_only.yaml up -d
> Option 1 with srsran
#### srsRAN ZMQ gNB (RF simulated)
docker compose -f srsgnb_zmq.yaml up -d && docker container attach srsgnb_zmq
#### srsRAN ZMQ 5G UE (RF simulated)
docker compose -f srsue_5g_zmq.yaml up -d && docker container attach srsue_5g_zmq
> Option 2 with ueransim
#### UERANSIM gNB (RF simulated)
docker compose -f nr-gnb.yaml up -d && docker container attach nr_gnb
#### UERANSIM NR-UE (RF simulated)
docker compose -f nr-ue.yaml up -d && docker container attach nr_ue

### O&M services
#### Observability stack deployment
docker compose -f services.yaml up --build -d

## Access UIs
### Provisioning of UE information in open5gs ui as follows:
Open (http://<DOCKER_HOST_IP>:9999) in a web browser, where <DOCKER_HOST_IP> is the IP of the machine/VM running the open5gs containers. Login with following credentials
```
Username : admin
Password : 1423
```
UE information defined in .env file
```
IMSI=001011234567895
KI=8baf473f2f8fd09487cccbd7097c6862
OP=11111111111111111111111111111111
```
### Access Grafana and Prometheus
#### Grafana
Open (http://<DOCKER_HOST_IP>:3000) in a web browser, where <DOCKER_HOST_IP> is the IP of the machine/VM running the open5gs containers. Login with following credentials
```
Username : open5gs
Password : open5gs
```
#### Prometheus
Open (http://<DOCKER_HOST_IP>:9090) in a web browser, where <DOCKER_HOST_IP> is the IP of the machine/VM running the open5gs containers.
