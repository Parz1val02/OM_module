# OM_module — 4G/5G Testbed with Full Observability
 
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go|68](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://golang.org/)
[![Open5GS](https://img.shields.io/badge/Open5GS-latest-green.svg)](https://open5gs.org/)
[![srsRAN](https://img.shields.io/badge/srsRAN-4G%20%7C%20Project-0082C9.svg)](https://www.srsran.com/)
[![UERANSIM](https://img.shields.io/badge/UERANSIM-v3.2.6-FF6B35.svg)](https://github.com/aligungr/UERANSIM)
[![Prometheus](https://img.shields.io/badge/Prometheus-latest-E6522C.svg)](https://prometheus.io/)
[![Grafana](https://img.shields.io/badge/Grafana-11.3-F46800.svg)](https://grafana.com/)
[![Loki](https://img.shields.io/badge/Loki-3.0-F5A623.svg)](https://grafana.com/oss/loki/)
[![Tempo](https://img.shields.io/badge/Tempo-latest-7B61FF.svg)](https://grafana.com/oss/tempo/)
[![Docker](https://img.shields.io/badge/Docker-Compose%20v2-2496ED.svg)](https://docs.docker.com/compose/)
 
---
 
## Overview
 
This project is a containerized 4G/5G mobile network testbed developed as a PUCP thesis prototype. It extends the [docker_open5gs](https://github.com/herlesupreeth/docker_open5gs) project with a custom **Operations & Maintenance (O&M) module** designed to improve the learning experience in mobile network labs.
 
The testbed runs Open5GS as the 4G/5G core and srsRAN/UERANSIM as the radio access network simulator. On top of the network stack, the O&M module provides **full observability**: per-container resource metrics, structured log aggregation, and distributed traces that correlate signaling events across network functions — S1AP, NGAP, GTPv2, PFCP, Diameter, and 5G SBI.
 
Four test scenarios (E1–E4) cover both 4G and 5G with normal attach/registration flows and controlled fault injection (wrong Ki, invalid APN/DNN, bad IMSI/SUPI, wrong DNN/SD), allowing students to observe how the core responds to authentication and session errors in real time.
 
---
 
## Architecture
 
```
┌─────────────────────────────────────────────────────────────────┐
│  RAN Layer                                                      │
│  srsRAN (eNB/gNB + UE)  ·  UERANSIM (gNB + UE)                  │
└──────────────┬──────────────────────────┬───────────────────────┘
               │ S1AP / NGAP / GTPv1-U    │
┌──────────────▼──────────────────────────▼───────────────────────┐
│  Core Layer                                                     │
│  Open5GS EPC (MME · HSS · SGWC/U · SMF · UPF · PCRF)            │
│  Open5GS 5GC (AMF · NRF · AUSF · UDM · UDR · PCF · NSSF ·       │
│               BSF · SCP · SMF · UPF)                            │
│  E4 slice extension (SMF2 · UPF2 — SST=1 SD=000002)             │
└──────────────┬──────────────────────────────────────────────────┘
               │ Docker bridge capture (SCTP · UDP)
               │ GTPv2 · PFCP · Diameter · SBI
┌──────────────▼──────────────────────────────────────────────────┐
│  O&M Module (Go)                                                │
│  Container discovery · tshark capture · Prometheus exporter     │
│  OTLP span emission · REST API                                  │
└──────────┬───────────────┬──────────────────┬───────────────────┘
           │ scrape        │ OTLP/HTTP        │ logs
    ┌──────▼──────┐  ┌─────▼──────┐  ┌───────▼──────┐
    │ Prometheus  │  │   Tempo    │  │     Loki     │
    └──────┬──────┘  └────────────┘  └──────────────┘
           │ PromQL / LogQL / TraceQL
    ┌──────▼──────┐
    │   Grafana   │  ← 14 pre-built dashboards
    └─────────────┘
```
 
### Component Overview
 
| Component | Role | Compose file |
|---|---|---|
| Open5GS EPC | 4G core: MME, HSS, SGWC, SGWU, SMF, UPF, PCRF | `4G_core.yaml` |
| Open5GS 5GC | 5G core: AMF, NRF, AUSF, UDM, UDR, PCF, NSSF, BSF, SCP, SMF, UPF | `5G_core.yaml` |
| SMF2 + UPF2 | E4 slice extension: SST=1 SD=000002, DNN=private, subnet 192.168.200.0/24 | `5G_core_e4.yaml` |
| srsRAN / UERANSIM | RAN simulation (eNB/gNB + UE, ZMQ transport) | `ran.yaml` |
| O&M Module | Packet capture, metrics exporter, REST API, OTLP tracing | `services.yaml` |
| Prometheus | Metrics collection & storage | `services.yaml` |
| Grafana | Dashboards & visualization | `services.yaml` |
| Loki + Promtail | Log aggregation & structured log shipping | `services.yaml` |
| Tempo | Distributed tracing backend | `services.yaml` |
| json-exporter | Prometheus adapter for Open5GS REST API metrics (UE/session counts) | `services.yaml` |
 
---
 
## Prerequisites
 
- **Docker Engine** ≥ 24
- **Docker Compose** v2 (`docker compose`)
- **GNU Make**
- **Linux host** — packet capture requires access to Docker bridge interfaces (`NET_ADMIN`, `NET_RAW`); the O&M container runs with `network_mode: host`
- **8 GB RAM** minimum recommended (more for E4 with multiple gNBs)
> `tshark` (Wireshark CLI) is **included inside the O&M container image** — you do not need to install it on the host.
 
---
 
## Setup — Pull Docker Images
 
Pull the base images before first use:
 
```bash
# Open5GS core image
docker pull ghcr.io/herlesupreeth/docker_open5gs:master
docker tag ghcr.io/herlesupreeth/docker_open5gs:master docker_open5gs
 
# srsRAN LTE (eNB + UE for 4G)
docker pull ghcr.io/herlesupreeth/docker_srslte:master
docker tag ghcr.io/herlesupreeth/docker_srslte:master docker_srslte
 
# srsRAN Project (gNB + UE for 5G)
docker pull ghcr.io/herlesupreeth/docker_srsran:master
docker tag ghcr.io/herlesupreeth/docker_srsran:master docker_srsran
 
# UERANSIM (alternative gNB + UE for 5G)
docker pull ghcr.io/herlesupreeth/docker_ueransim:master
docker tag ghcr.io/herlesupreeth/docker_ueransim:master docker_ueransim
```
 
The O&M module image (`docker_om_module`) is built locally from `./om-module`. Docker Compose will use a cached image if one already exists with that name. To force a rebuild (e.g. after modifying the Go source):
 
```bash
docker compose -f services.yaml up --build -d
```
 
---
 
## Quick Start
 
### Recommended startup order
 
```bash
# Step 1 — Start the core (choose one)
make core-4g-up       # 4G core (Open5GS EPC: MME, HSS, SGWC/U, SMF, UPF, PCRF)
make core-5g-up       # 5G core (Open5GS 5GC: AMF, NRF, AUSF, UDM, UDR, PCF, ...)
 
# Step 2 — Start the observability + O&M stack (after core is up)
make services-up
 
# Step 3 — Provision subscriber data (MongoDB must be running)
bash scripts/mongo_insert.sh
 
# Step 4 — Launch a scenario (choose one)
make e1               # E1 — Basic 4G attach
make e3               # E3 — Basic 5G registration (srsRAN)
make e4               # E4 — Multi-gNB slicing (automatically starts smf2+upf2)
 
# Step 5 — Generate traffic
make traffic
```
 
> **E4 note:** `make e4` automatically brings up `smf2` and `upf2` via `5G_core_e4.yaml` before launching the RAN. The base core (`make core-5g-up`) must already be running. No manual compose step is needed.
 
Run `make help` to see all available targets.
 
---
 
## Test Scenarios
 
The four scenarios are designed in two parallel pairs for direct 4G↔5G comparison:
 
- **E1 ↔ E3** — baseline complete flow: same sequence of events (attach/registration → bearer/PDU session → traffic → detach/deregistration), different architecture
- **E2 ↔ E4** — multi-RAN node + fault injection: same fault categories (identity, authentication, session), different core and slicing
| Scenario | Generation | RAN | Description | Makefile |
|---|---|---|---|---|
| E1 | 4G | srsRAN LTE | 1 eNB + 1 valid UE — full EPS Attach → Bearer → Traffic → Detach flow | `make e1` |
| E2 | 4G | srsRAN LTE | 4 independent eNB+UE pairs — 1 valid + 3 fault-injected (wrong Ki, bad IMSI, wrong APN) | `make e2` |
| E3 | 5G | srsRAN Project (default) or UERANSIM | 1 gNB + 1 valid UE — full 5G Registration → PDU Session → Traffic → Deregistration flow | `make e3` / `make e3-ueransim` |
| E4 | 5G | srsRAN Project + UERANSIM | 3 gNBs + network slicing (SST=1 SD=000001 / SST=1 SD=000002) + dedicated SMF+UPF per slice + 4 valid UEs + 4 fault-injected UEs | `make e4` |
 
The Makefile waits for readiness at each step before proceeding (handled by `scripts/wait_ran.sh`).
 
### E2 — UE distribution (4G fault injection)
 
| Container | eNB | IMSI | Fault mechanism | Expected failure |
|---|---|---|---|---|
| `srsue_zmq` | eNB1 | 895 | None (valid) | ✅ Attach successful |
| `srsue_zmq_bad_ki` | eNB2 | 902 | Wrong Ki in `.conf` (DB entry correct) | ❌ `Authentication failure (MAC failure)` — `OGS_NAS_EMM_CAUSE[20]` |
| `srsue_zmq_bad_imsi` | eNB3 | 901 | **IMSI not in MongoDB** | ❌ `Attach reject` — `OGS_NAS_EMM_CAUSE[8]` (IMSI unknown in HLR) |
| `srsue_zmq_bad_apn` | eNB4 | 903 | Wrong APN in `.conf` (DB entry correct) | ⚠️ Attach succeeds, PDN rejected — `Invalid APN` (ESM layer) |
 
> **Key pedagogical contrast:** `bad_ki` fails *during* authentication (subscriber found in DB, key derivation fails); `bad_imsi` fails *before* authentication (HSS rejects the identity lookup); `bad_apn` fails *after* attach (session layer, not authentication).
 
> **ZMQ constraint:** srsRAN 4G ZMQ uses point-to-point REQ/REPLY sockets — one eNB can only serve one srsUE simultaneously. E2 therefore uses 4 independent eNB+UE pairs rather than multiple UEs per eNB.
 
### E4 — UE distribution (5G slicing + fault injection)
 
E4 implements **true network slicing with user plane isolation**: two independent SMF+UPF pairs, each serving a distinct slice with a separate UE IP subnet.
 
| Slice | S-NSSAI | DNN | SMF | UPF | UE subnet |
|---|---|---|---|---|---|
| Slice 1 | SST=1 SD=000001 | internet | `smf` | `upf` | 192.168.100.0/24 |
| Slice 2 | SST=1 SD=000002 | private | `smf2` | `upf2` | 192.168.200.0/24 |
 
| Container | gNB | IMSI | Slice | Expected result |
|---|---|---|---|---|
| `srsue_5g_zmq` | srsgnb_zmq | 895 | SST=1 SD=000001 (internet) | ✅ Registration + PDU → 192.168.100.x |
| `nr_ue` | gNB1 (UERANSIM) | 896 | SST=1 SD=000001 (internet) | ✅ Registration + PDU → 192.168.100.x |
| `nr_ue2` | gNB2 (UERANSIM) | 898 | SST=1 SD=000001 (internet) | ✅ Registration + PDU → 192.168.100.x |
| `nr_ue3` | gNB2 (UERANSIM) | 899 | SST=1 SD=000002 (private) | ✅ Registration + PDU → 192.168.200.x via smf2/upf2 |
| `nr_ue_bad_supi` | gNB1 | 905 | SST=1 SD=000001 | ❌ **SUPI not in MongoDB** → `Cannot find SUCI [404]` → Reject [7] |
| `nr_ue_bad_ki` | gNB1 | 906 | SST=1 SD=000001 | ❌ Wrong K in `.yaml` (DB correct) → `Auth failure MAC` → Reject [111] |
| `nr_ue_bad_dnn` | gNB1 | 908 | SST=1 SD=000001 | ⚠️ Registration succeeds, `DNN_NOT_SUPPORTED_OR_NOT_SUBSCRIBED` |
| `nr_ue_bad_sst` | gNB2 | 909 | SST=1 SD=000003 (non-existent) | ❌ `Cannot find Requested NSSAI [SST:1 SD:0x3]` → Reject [62] |
 
> **Slicing isolation proof:** `nr_ue3` always receives an IP in `192.168.200.x`, while all SST=1 SD=000001 UEs receive `192.168.100.x`. Traffic from each slice never crosses the other's UPF — observable via `ogstun` (slice 1) and `ogstun3` (slice 2) interface counters.
 
> **bad_sst fault mechanism:** The UE requests SST=1 SD=000003 which is not declared in the AMF's `plmn_support`. The AMF rejects at the NSSAI check before authentication begins (cause 62: Requested NSSAI not subscribed).
 
> **UERANSIM stability note:** In long-running E4 sessions with multiple UERANSIM instances, spontaneous disconnections that block reconnection have been observed. Run E4 within bounded time windows.

### Access Grafana

Open (http://localhost:3000) in a web browser. Login with following credentials

```
Username : open5gs
Password : open5gs
```
 
### Teardown
 
```bash
make e1-down          # Stop only the RAN for E1 (core + services stay up)
make e2-down          # Stop RAN for E2
make e3-down          # Stop RAN for E3 (srsRAN)
make e3-ueransim-down # Stop RAN for E3 (UERANSIM)
make e4-down          # Stop all RAN profiles + smf2/upf2 for E4
make down             # Stop everything (RAN + core + services)
```
 
---
 
## Provisioning Subscriber Data
 
### Automated (recommended)
 
Run after `make core-4g-up` or `make core-5g-up` (MongoDB must be running):
 
```bash
bash scripts/mongo_insert.sh
```
 
The script drops existing subscribers and inserts all UEs needed for E1–E4. It is idempotent — safe to run multiple times. Subscribers provisioned:
 
| IMSI | Scenario | Role |
|---|---|---|
| `001011234567895` | E1 / E3 / E4 | Valid UE (base) — works for 4G and 5G |
| `001011234567896` | E3 / E4 | Valid 5G UE — SST=1 SD=000001 (internet) |
| `001011234567898` | E4 | Valid 5G UE — SST=1 SD=000001 (internet), gNB2 |
| `001011234567899` | E4 | Valid 5G UE — SST=1 SD=000002 (private), routed to smf2/upf2 |
| `001011234567902` | E2 | DB entry correct, srsue config has **wrong Ki** → auth failure |
| `001011234567903` | E2 | DB entry correct, srsue config has **wrong APN** → PDN reject |
| `001011234567906` | E4 | DB correct, config has **wrong K** → auth failure at AUSF |
| `001011234567908` | E4 | DB correct, config has **wrong DNN** → PDU reject at SMF |
| `001011234567909` | E4 | DB correct (SD=000001), config requests **SD=000003** → reject at AMF |
 
> **Not inserted intentionally**: IMSI `001011234567901` (bad_imsi E2) and `001011234567905` (bad_supi E4). Their absence from MongoDB *is* the fault injection — the core returns `Unknown UE`.
 
### Manual (fallback)
 
Open <http://localhost:9999> (credentials: `admin` / `1423`) to add subscribers one by one via the Open5GS WebUI.
 
Default UE credentials from `.env`:
 
```
IMSI : 001011234567895
Ki   : 8baf473f2f8fd09487cccbd7097c6862
OP   : 11111111111111111111111111111111
```
 
---
 
## Traffic Generation
 
```bash
make traffic
```
 
Runs `scripts/traffic.sh`, which executes ping from all active UE containers. Works for any scenario currently up. For E4, `nr_ue3` sends traffic through `upf2` (DNN=private, subnet 192.168.200.x) while the other UEs go through `upf` (DNN=internet, subnet 192.168.100.x) — demonstrating user plane isolation between slices.
 
---
 
## O&M Module
 
### What it does
 
The O&M module is a Go service (`./om-module`) that runs alongside the testbed and provides:
 
1. **Container discovery** — connects to the Docker daemon, filters containers by Compose project label (`om.*` taxonomy: domain, nf, generation, project), and maintains a live snapshot refreshed every 15 seconds.
2. **Packet capture** — spawns `tshark` as a subprocess on the Docker bridge interface (`auto`-detected or explicitly configured). Captures SCTP (S1AP/NGAP), UDP (GTPv2/PFCP), TCP (Diameter), and HTTP/2 (5G SBI). Parses Elastic-JSON output and emits one OTLP span per packet to Grafana Tempo.
3. **Prometheus metrics** — exposes container resource metrics and capture pipeline counters at `/metrics`.
4. **REST API** — four endpoints for integration and monitoring.
---
 
## Repository Structure
 
```
om-module/               # O&M module Go source
│   ├── internal/
│   │   ├── capture/     # tshark subprocess + packet parser
│   │   ├── collector/   # Docker container snapshot
│   │   ├── docker/      # Docker SDK client wrapper
│   │   ├── exporter/    # Prometheus metrics exporter
│   │   ├── pipeline/    # Packet → OTLP span pipeline + capture metrics
│   │   └── tracing/     # OpenTelemetry tracer init (OTLP/HTTP → Tempo)
│
├── 4G_core.yaml             # Docker Compose — Open5GS EPC (4G core)
├── 5G_core.yaml             # Docker Compose — Open5GS 5GC (5G core)
├── 5G_core_e4.yaml          # Docker Compose — E4 slice extension (smf2 + upf2)
├── ran.yaml                 # Docker Compose — RAN (profiles: ran-4g-srs, ran-4g-e2, ran-5g-srs, ran-5g-ueransim, ran-5g-e4)
├── services.yaml            # Docker Compose — O&M module + observability stack
├── Makefile                 # Automation (see make help)
├── .env                     # IP assignments, UE credentials, MCC/MNC
│
├── scripts/                 # Helper scripts
│   ├── mongo_insert.sh      # Provision all UEs for E1–E4
│   ├── wait_core.sh         # Readiness probe for core startup
│   ├── wait_ran.sh          # Readiness probe for RAN startup
│   ├── run_e2.sh            # Multi-container launch for E2
│   ├── run_e4.sh            # Multi-container launch for E4
│   └── traffic.sh           # Ping from all active UEs
│
├── grafana/                 # Dashboards + provisioning config
├── prometheus/configs/      # Prometheus scrape config (docker SD + json-exporter jobs)
├── json_exporter/           # Config for Prometheus json-exporter (Open5GS REST API)
├── metrics_endpoints/       # Per-NF metrics endpoint definitions
├── promtail/                # Log shipping config (core logs + RAN logs → Loki)
├── loki/                    # Loki storage config
├── tempo/                   # Tempo tracing backend config
│
├── <nf-config dirs>/        # Per-NF Open5GS config
│                            # (amf, ausf, bsf, hss, mme, nrf, nssf, pcf, pcrf,
│                            #  scp, sgwc, sgwu, smf, udm, udr, upf, webui)
├── srslte/  srsran/         # srsRAN LTE / srsRAN Project UE+RAN configs
├── ueransim/                # UERANSIM gNB + UE configs
└── procedures_captures/     # Reference packet captures for E1–E4 (PCAP + JSON)
```

---
 
## Implementation Notes
 
**Network slicing in E4 — SD-based, not SST-based**
E4 uses two slices with the same SST (1) but different Slice Differentiators: SD=000001 (internet) and SD=000002 (private). This reflects real-world deployments where SST identifies the service class and SD identifies the operator-specific instance. Each SD is served by a dedicated SMF+UPF pair with an isolated UE IP subnet, providing true user plane isolation observable via `ogstun` (slice 1) and `ogstun3` (slice 2) interface traffic counters.
 
**Handover — not included as a scenario**
In 5G, srsRAN Project only supports intra-gNB handover and requires a USRP X/N-series radio with two RF chains. In 4G, S1 handover over ZMQ requires GNU Radio Companion as an external broker outside the Docker stack. Both constraints make handover impractical in this fully virtualized testbed.
 
**Multi-UE with ZMQ in 4G**
srsRAN 4G ZMQ sockets are point-to-point (REQ/REPLY) — one eNB can serve only one srsUE at a time. Supporting multiple UEs per eNB would require a GRC broker. E2 works around this by using 4 independent eNB+UE pairs.
 
**srsRAN Project vs UERANSIM in E3**
E3 has two variants: `make e3` uses srsRAN Project and `make e3-ueransim` uses UERANSIM. srsRAN Project is the default because its behavior on N2 loss is more predictable. UERANSIM is available as an alternative and is the primary choice for E4 where its lightweight instances allow running 3 gNBs + 7 UEs with low overhead.
 
**srsRAN UE slice encoding**
srsRAN's UE implementation (`srsue_5g_zmq`) does not support encoding slice-specific NAS IEs without triggering a protocol error in Open5GS AMF. The `[slicing]` section in `srslte/ue_5g_zmq.conf` must remain commented out. Slice assignment for this UE is handled by the AMF based on the subscriber record in MongoDB (SD=000001), not the UE's NAS request.
 
**UERANSIM — 5G only**
UERANSIM operates exclusively over NGAP (N2) and 5G SA interfaces. It has no support for S1AP or 4G EPC.
 
---
 
## License
 
MIT License — Copyright 2026 Rodrigo Barrios.
 
Helper scripts derived from [docker_open5gs](https://github.com/herlesupreeth/docker_open5gs) are BSD 2-Clause — Copyright Supreeth Herle.
