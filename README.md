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

### 4G core deployment
docker compose -f 4g_core_only.yaml up -d
#### srsRAN ZMQ eNB (RF simulated)
docker compose -f srsenb_zmq.yaml up -d && docker container attach srsenb_zmq
#### srsRAN ZMQ 4G UE (RF simulated)
docker compose -f srsue_zmq.yaml up -d && docker container attach srsue_zmq

### 5G core deployment
docker compose -f 5g_core_only.yaml up -d
#### srsRAN ZMQ gNB (RF simulated)
docker compose -f srsgnb_zmq.yaml up -d && docker container attach srsgnb_zmq
#### srsRAN ZMQ 5G UE (RF simulated)
docker compose -f srsue_5g_zmq.yaml up -d && docker container attach srsue_5g_zmq

## Provisioning of UE information

### Provisioning of UE information in open5gs as follows:

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
