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

> Educational 4G/5G mobile network testbed with a custom O&M module, packet capture, Prometheus metrics, distributed tracing, and 4 controlled fault-injection scenarios.

---

## Overview

This project is a containerized 4G/5G mobile network testbed developed as a PUCP thesis prototype. It extends the [docker_open5gs](https://github.com/herlesupreeth/docker_open5gs) project with a custom **Operations & Maintenance (O&M) module** designed to improve the learning experience in mobile network labs.

The testbed runs Open5GS as the 4G/5G core and srsRAN/UERANSIM as the radio access network simulator. On top of the network stack, the O&M module provides **full observability**: per-container resource metrics, structured log aggregation, and distributed traces that correlate signaling events across network functions — S1AP, NGAP, GTPv2, PFCP, Diameter, and 5G SBI.

Four test scenarios (E1–E4) cover both 4G and 5G with normal attach flows and controlled fault injection (wrong Ki, invalid APN, bad SUPI, wrong DNN/SST), allowing students to observe how the core responds to authentication and session errors in real time.

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
| Open5GS 5GC | 5G core: AMF, NRF, AUSF, UDM, UDR, PCF, NSSF, BSF, SCP | `5G_core.yaml` |
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

# Step 5 — Generate traffic
make traffic
```

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
| E4 | 5G | srsRAN Project + UERANSIM | 3 gNBs + network slicing (SST=1, SST=2) + 4 valid UEs + 4 fault-injected UEs | `make e4` |

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

| Container | gNB | IMSI | Fault mechanism | Expected failure |
|---|---|---|---|---|
| `srsue_5g_zmq` | srsgnb_zmq | 895 | None (valid, SST=1) | ✅ Registration + PDU Session |
| `nr_ue` | gNB1 (UERANSIM) | 896 | None (valid, SST=1) | ✅ Registration + PDU Session |
| `nr_ue2` | gNB2 (UERANSIM) | 898 | None (valid, SST=1) | ✅ Registration + PDU Session |
| `nr_ue3` | gNB2 (UERANSIM) | 899 | None (valid, SST=2) | ✅ Registration + PDU Session on slice 2 |
| `nr_ue_bad_supi` | gNB1 | 905 | **SUPI not in MongoDB** | ❌ `Cannot find SUCI [404]` → Reject [7] |
| `nr_ue_bad_ki` | gNB1 | 906 | Wrong K in `.yaml` (DB correct) | ❌ `Auth failure MAC` → Reject [111] |
| `nr_ue_bad_dnn` | gNB1 | 908 | Wrong DNN in `.yaml` | ⚠️ Registration succeeds, `DNN_NOT_SUPPORTED_OR_NOT_SUBSCRIBED` |
| `nr_ue_bad_sst` | gNB2 | 909 | SST=3 (non-existent slice) | ❌ `Cannot find Requested NSSAI [SST:3]` → Reject [62] |

> **AMF configuration required for E4:** `amf/amf.yaml` must declare `sst: 2` under `plmn_support` in addition to `sst: 1`. Without this, the AMF rejects all SST=2 UEs with Registration reject [62] before authentication begins.

> **UERANSIM stability note:** In long-running E4 sessions with multiple UERANSIM instances, spontaneous disconnections that block reconnection have been observed. Run E4 within bounded time windows.

### Teardown

```bash
make e1-down          # Stop only the RAN for E1 (core + services stay up)
make e2-down          # Stop RAN for E2
make e3-down          # Stop RAN for E3 (srsRAN)
make e3-ueransim-down # Stop RAN for E3 (UERANSIM)
make e4-down          # Stop all RAN profiles for E4
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
| `001011234567895` | E1 / E3 | Valid UE (base) |
| `001011234567896`–`899` | E4 | Valid 5G UEs with SST=1 and SST=2 |
| `001011234567902` | E2 | DB entry correct, but srsue config has **wrong Ki** → auth failure |
| `001011234567903` | E2 | DB entry correct, but srsue config has **wrong APN** → PDN reject |
| `001011234567906`, `908`, `909` | E4 | DB entries correct, but configs have wrong-K / wrong-DNN / wrong-SST |

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

Runs `scripts/traffic.sh`, which executes ping from all active UE containers. Works for any scenario that is currently up.

---

## O&M Module

### What it does

The O&M module is a Go service (`./om-module`) that runs alongside the testbed and provides:

1. **Container discovery** — connects to the Docker daemon, filters containers by Compose project label (`om.*` taxonomy: domain, nf, generation, project), and maintains a live snapshot refreshed every 15 seconds.
2. **Packet capture** — spawns `tshark` as a subprocess on the Docker bridge interface (`auto`-detected or explicitly configured). Captures SCTP (S1AP/NGAP), UDP (GTPv2/PFCP), TCP (Diameter), and HTTP/2 (5G SBI). Parses Elastic-JSON output and emits one OTLP span per packet to Grafana Tempo.
3. **Prometheus metrics** — exposes container resource metrics and capture pipeline counters at `/metrics`.
4. **REST API** — four endpoints for integration and monitoring.

### Configuration — Environment Variables

Set in `services.yaml` or `.env`. The module reads them at startup via `config.Load()`.

| Variable | Default | Description |
|---|---|---|
| `OM_PORT` | `8080` | HTTP server port |
| `DOCKER_SOCKET` | `/var/run/docker.sock` | Docker daemon socket path |
| `COMPOSE_PROJECT` | `om_module` | Compose project label used to filter testbed containers |
| `TEMPO_ENDPOINT` | `tempo:4318` | Grafana Tempo OTLP/HTTP endpoint (POSTs to `<endpoint>/v1/traces`) |
| `CAPTURE_ENABLED` | `true` | Set to `false` to disable packet capture without rebuilding |
| `CAPTURE_INTERFACE` | `auto` | Bridge interface name; `auto` = dynamic discovery via Docker network inspection |
| `MCC` | `001` | Mobile Country Code (used to reconstruct full 5G IMSI from SUCI MSIN in NGAP packets) |
| `MNC` | `01` | Mobile Network Code (same purpose as MCC) |

### API Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/metrics` | Prometheus scrape endpoint |
| `GET` | `/topology` | JSON snapshot of all testbed containers with state, domain, NF, generation, and health status |
| `GET` | `/ping` | Liveness probe — returns `pong` with HTTP 200 |
| `GET` | `/capture/status` | Capture pipeline status: running, interface, generation detected, packet counters (total/4G/5G), restart count, uptime |

### Capture Pipeline — Protocols Captured

| Protocol | Transport | Layer | Used in |
|---|---|---|---|
| S1AP | SCTP | RAN↔MME | 4G |
| NGAP | SCTP | RAN↔AMF | 5G |
| GTPv2 | UDP | MME↔SGWC, SGWC↔PGW (S5/S8) | 4G |
| PFCP | UDP | SMF↔UPF (N4) | 4G / 5G |
| Diameter | TCP | MME↔HSS (S6a), PGW↔PCRF (Gx) | 4G |
| 5G SBI | HTTP/2 | AMF↔NRF/AUSF/UDM/SMF/PCF (Nxx) | 5G |

Each captured packet becomes an OpenTelemetry span with attributes: `src_nf`, `dst_nf`, `generation`, `protocol`, `imsi`, `procedure`, `cause`, `apn_dnn`, `teid`/`seid` (where applicable).

> **ZMQ transport note:** In ZMQ mode (all scenarios), GTP-U N3 counters (`fivegs_ep_n3_gtp_indatapktn3upf` / `outdatapktn3upf`) remain at 0 because user-plane traffic does not traverse a real network interface. Use `fivegs_upffunction_upf_qosflows` as the indicator that the user plane is active.

---

## Prometheus Metrics Reference

### Container metrics (from `internal/exporter/prometheus.go`)

All container metrics carry these labels: `container`, `project`, `domain`, `nf`, `generation`, `image`, `state`. The `generation` label (`4g` | `5g` | `none`) is what powers the 4G-vs-5G comparison panels in Grafana.

| Metric | Type | Description |
|---|---|---|
| `container_cpu_usage_percent` | Gauge | CPU usage percentage (0–100 × numCPUs) |
| `container_memory_usage_bytes` | Gauge | Working-set memory in bytes (usage − cache) |
| `container_network_rx_bytes_total` | Counter | Total bytes received across all container interfaces |
| `container_network_tx_bytes_total` | Counter | Total bytes transmitted across all container interfaces |
| `container_pids` | Gauge | Number of processes running inside the container |
| `container_health_status` | Gauge | `1` = running, `0` = degraded/unknown, `-1` = stopped |

### Capture pipeline metrics (from `internal/pipeline/metrics.go`)

| Metric | Type | Labels | Description |
|---|---|---|---|
| `om_capture_packets_total` | Counter | `protocol`, `generation`, `src_nf`, `dst_nf` | Packets captured per protocol/generation/NF pair |
| `om_capture_sbi_requests_total` | Counter | `service`, `method`, `src_nf`, `dst_nf` | 5G SBI HTTP/2 requests per N-interface service and method |
| `om_capture_errors_total` | Counter | `protocol`, `generation`, `src_nf`, `dst_nf` | Packets with error causes (GTPv2 cause≠16, PFCP cause≠1, Diameter≠2001, SBI status≥400) |

### Open5GS NF metrics (via json-exporter)

Scraped from the Open5GS REST management API (`http://<nf>:9091/`) and proxied through `json-exporter`:

| Job | Source | Example metrics |
|---|---|---|
| `amf_ue` | AMF `/ue-info` | UE registration count |
| `amf_gnb` | AMF `/gnb-info` | Connected gNB count |
| `smf_pdu_5g` | SMF `/pdu-info` | Active PDU sessions (5G) |
| `mme_ue` | MME `/ue-info` | Attached UE count |
| `mme_enb` | MME `/enb-info` | Connected eNB count |
| `smf_pdu_4g` | SMF `/pdu-info` | Active PDU sessions (4G) |

---

## Observability Stack

### Access URLs

| Service | URL | Credentials |
|---|---|---|
| Open5GS WebUI | <http://localhost:9999> | `admin` / `1423` |
| Grafana | <http://localhost:3000> | `open5gs` / `open5gs` |
| Prometheus | <http://localhost:9090> | — |
| Loki API | <http://localhost:3100> | — |
| Tempo API | <http://localhost:3200> | — |
| O&M Module API | <http://localhost:8080> | — |

### Grafana Dashboards

14 pre-built dashboards are provisioned automatically:

- **4G Overview** — MME/HSS/SGWC/UPF container health, attach counts, PDN sessions
- **5G Overview** — AMF/SMF/UPF health, registration counts, PDU sessions
- **Core detail** — per-NF resource usage for all core components
- **4G Traces** — Diameter, GTPv2, PFCP spans in Tempo waterfall view
- **5G Traces** — NGAP, PFCP, SBI spans with procedure labels
- **E1–E4 scenario dashboards** — per-scenario success/failure conditions with PromQL panels and trace links

### Grafana Alerts

4 alert rules are provisioned automatically via `grafana/provisioning/alerting/rules.yml`:

| Alert | Condition | Severity |
|---|---|---|
| `[4G] UE Attach Failure Suspected` | `ues_active > 0 and mme_session == 0` sustained for 1 min | critical |
| `[4G] Auth/config failure in MME` | `count_over_time({nf="mme", procedure="error"}[1m]) > 0` | warning |
| `[5G] Registration failure rate > 20%` | `(reginitreq - reginitsucc) / reginitreq > 0.2` sustained for 1 min | critical |
| `[5G] Auth failure detected` | `fivegs_amffunction_amf_authreject > 0` | warning |

Alerts fire to an email contact point configured in `grafana/provisioning/alerting/contactpoints.yml`. The recipient address is commented out by default — set `ALERT_TO_EMAIL` in `.env` or edit the file directly before deploying.

### Loki — structured log labels

Promtail enriches all core logs with a `procedure` label that classifies each log line by signaling event type. Useful for LogQL queries and the alert rule above:

| Value | Covers |
|---|---|
| `attach` | EPS Attach / 5G Registration request and completion events |
| `session` | Bearer/PDU session creation and teardown |
| `release` | UE context release, detach, deregistration |
| `error` | Authentication failures, attach rejects, invalid APN/DNN/SST |

Example query in Grafana Explore:
```logql
{nf="mme", procedure="error"} | json
{nf="amf", procedure="error"} | json
```

---

## Repository Structure

```
OM_module/
├── om-module/               # Go O&M application
│   ├── api/                 # HTTP handlers (/metrics, /topology, /ping, /capture/status)
│   ├── config/              # Environment variable configuration (config.go)
│   └── internal/
│       ├── capture/         # tshark subprocess manager
│       ├── collector/       # Docker container stats collector (15s refresh)
│       ├── docker/          # Docker client wrapper
│       ├── exporter/        # Prometheus metrics exporter
│       ├── pipeline/        # Packet→OTLP span pipeline + capture metrics
│       └── tracing/         # OpenTelemetry tracer init (OTLP/HTTP → Tempo)
│
├── 4G_core.yaml             # Docker Compose — Open5GS EPC (4G core)
├── 5G_core.yaml             # Docker Compose — Open5GS 5GC (5G core)
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
├── <nf-config dirs>/        # Per-NF Open5GS config (amf, mme, smf, upf, hss, ...)
│                            # (amf, ausf, bsf, hss, mme, nrf, nssf, pcf, pcrf,
│                            #  scp, sgwc, sgwu, smf, udm, udr, upf, webui)
├── srslte/  srsran/         # srsRAN LTE / srsRAN Project UE+RAN configs
├── ueransim/                # UERANSIM gNB + UE configs
└── procedures_captures/     # Reference packet captures for E1–E4 (PCAP + JSON)
```

---

## Troubleshooting

**O&M module doesn't see any containers**
Check that `COMPOSE_PROJECT` matches the Docker Compose project name. Run `docker ps --format '{{.Labels}}'` and look for the `om.project` label. The default is `om_module` (derived from the directory name).

**No traces appear in Tempo**
Verify the capture pipeline is running: `curl localhost:8080/capture/status`. If `running` is `false`, check that `CAPTURE_ENABLED=true` and that the bridge interface was auto-detected correctly (`interface` field). The module requires `NET_ADMIN` + `NET_RAW` capabilities and `network_mode: host`.

**Subscribers not found / core rejects UE immediately**
Run `bash scripts/mongo_insert.sh` after the core is up. If the core was restarted, run it again (it's safe to repeat — drops and re-inserts).

**MongoDB error when provisioning**
MongoDB must be running before provisioning. Run `make core-4g-up` or `make core-5g-up` first and wait for the core to be healthy.

**E4 scenario: SST=2 UEs rejected immediately with Registration reject [62]**
`amf/amf.yaml` must declare `sst: 2` under `plmn_support`. Without it the AMF rejects SST=2 UEs before authentication begins. Add the following to `amf/amf.yaml`:
```yaml
plmn_support:
  - plmn_id:
      mcc: 001
      mnc: 01
    s_nssai:
      - sst: 1
      - sst: 2
```
Then restart the core with `make core-5g-up`.

---

## Implementation Notes

**Handover — not included as a scenario**
In 5G, srsRAN Project only supports intra-gNB handover and requires a USRP X/N-series radio with two RF chains. In 4G, S1 handover over ZMQ requires GNU Radio Companion as an external broker outside the Docker stack. Both constraints make handover impractical in this fully virtualized testbed.

**Multi-UE with ZMQ in 4G**
srsRAN 4G ZMQ sockets are point-to-point (REQ/REPLY) — one eNB can serve only one srsUE at a time. Supporting multiple UEs per eNB would require a GRC broker. E2 works around this by using 4 independent eNB+UE pairs.

**srsRAN Project vs UERANSIM in E3**
E3 has two variants: `make e3` uses srsRAN Project and `make e3-ueransim` uses UERANSIM. srsRAN Project is the default because its behavior on N2 loss is more predictable. UERANSIM is available as an alternative and is the primary choice for E4 where its lightweight instances allow running 3 gNBs + 7 UEs with low overhead.

**UERANSIM — 5G only**
UERANSIM operates exclusively over NGAP (N2) and 5G SA interfaces. It has no support for S1AP or 4G EPC.

---

## License

MIT License — Copyright 2026 Rodrigo Barrios.

Helper scripts under `scripts/` derived from [docker_open5gs](https://github.com/herlesupreeth/docker_open5gs) are BSD 2-Clause — Copyright Supreeth Herle.
