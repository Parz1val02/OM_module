# OM_module
RF simulated 4G/5G ran and core based on the docker_open5gs project: https://github.com/herlesupreeth/docker_open5gs

## Get Pre-built Docker images
Pull base open5gs image:
```bash
docker pull ghcr.io/herlesupreeth/docker_open5gs:master
docker tag ghcr.io/herlesupreeth/docker_open5gs:master docker_open5gs
```
For srsRAN components:
```bash
docker pull ghcr.io/herlesupreeth/docker_srslte:master
docker tag ghcr.io/herlesupreeth/docker_srslte:master docker_srslte

docker pull ghcr.io/herlesupreeth/docker_srsran:master
docker tag ghcr.io/herlesupreeth/docker_srsran:master docker_srsran
```
For ueransim components:
```bash
docker pull ghcr.io/herlesupreeth/docker_ueransim:master
docker tag ghcr.io/herlesupreeth/docker_ueransim:master docker_ueransim
```

## Deployments

### 4G core and RAN deployment
docker compose -f 4G_core.yaml up -d
#### 4G srsRAN (eNB + UE)
docker compose -f ran.yaml --profile ran-4g-srs up -d<br/>
docker container attach srsenb_zmq<br/>
docker container attach srsue_zmq<br/>
### 5G core and RAN deployment
docker compose -f 5G_core.yaml up -d
> Option 1 with srsran
#### 5G srsRAN (gNB + UE)
docker compose -f ran.yaml --profile ran-5g-srs up -d<br/>
docker container attach srsgnb_zmq<br/>
docker container attach srsue_5g_zmq<br/>
> Option 2 with ueransim
#### 5G UERANSIM (gNB + UE)
docker compose -f ran.yaml --profile ran-5g-ueransim up -d<br/>
docker container attach nr_gnb<br/>
docker container attach nr_ue<br/>

### O&M services
#### Observability stack deployment
docker compose -f services.yaml up --build -d

### Traffic Generation
Before generating traffic, verify the TUN interface exists and has an IP:
> 4G srsLTE and 5G srsRAN
```bash
docker exec srsue_zmq ip addr show tun_srsue
docker exec srsue_5g_zmq ip addr show tun_srsue
```
> UERANSIM
```bash
docker exec nr_ue ip addr show uesimtun0
```
Verify connectivity to the UPF gateway:
```bash
docker exec <ue_container> ping -I <interface> -c 3 192.168.100.1
```
Continuous traffic:
```bash
docker exec <ue_container> ping -I <interface> -i 0.2 -s 1400 8.8.8.8
```

## Access UIs
### Provisioning of UE information in open5gs ui as follows:
Open (http://localhost:9999) in a web browser. Login with following credentials
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
Open (http://localhost:3000) in a web browser. Login with following credentials
```
Username : open5gs
Password : open5gs
```
#### Prometheus
Open (http://localhost:9090) in a web browser

#### Loki
Available as a datasource in Grafana. Direct API at http://localhost:3100.

#### Tempo
Available as a datasource in Grafana. Direct API at http://localhost:3200.
