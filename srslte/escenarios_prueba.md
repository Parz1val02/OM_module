# Escenarios de Prueba — Módulo O&M para Testbed 4G/5G

**Tesis:** Desarrollo de un prototipo de módulo de Operación y Mantenimiento (O&M) que mejore la experiencia de aprendizaje en testbeds de redes móviles 4G/5G con herramientas open source  
**Alumno:** Rodrigo Edú Barrios Inga  
**Stack:** docker_open5gs · Open5GS · srsRAN 4G · srsRAN Project · UERANSIM

---

## Resumen de escenarios

| ID | Nombre | Arquitectura | Tipo |
|----|--------|-------------|------|
| E1 | Flujo completo + Fault Injection MME | 4G | Procedimiento normal + fallo NF |
| E2 | Multi-eNB con UEs mixtos | 4G | Carga + fallos de configuración |
| E3 | Flujo completo + Fault Injection AMF | 5G | Procedimiento normal + fallo NF |
| E4 | Multi-gNB con slicing y UEs mixtos | 5G UERANSIM + 5G srsRAN | Carga + slicing + fallos de configuración |

**Comparaciones directas:**
- E1 ↔ E3: flujo base + fault injection, arquitectura 4G vs 5G
- E2 ↔ E4: múltiples estaciones base, fallos de configuración, 4G vs 5G

---

## E1 — Flujo completo + Fault Injection MME (4G)

**Arquitectura:** Open5GS EPC + srsRAN 4G (ZMQ)

**Duración estimada:** 10-15 minutos

**Prerrequisitos:**
- Core 4G levantado: `docker compose -f 4G_core.yaml up -d`
- Módulo O&M levantado: `docker compose -f services.yaml up -d`
- UE1 provisionado en la WebUI (IMSI, Ki, OP/OPc válidos)

**Herramientas de observación:**
- **Grafana** — métricas en tiempo real: `mme_ue_count`, `bearers_active`, `container_health_status`
- **Loki** — logs de señalización S1AP y NAS del MME
- **Tempo** — trazas del flujo Attach → Bearer → Detach
- **Alertmanager** — alertas críticas de contenedor caído en Fase 2

**Topología:**
- 1 eNB (`srsenb_zmq`)
- 1 srsUE (`srsue_zmq`) con credenciales válidas

**Condiciones de éxito:**
- Fase 1: `mme_ue_count` sube a 1 durante el attach y vuelve a 0 en detach. Las trazas muestran los 5 pasos del flujo completos. `bearers_active` incrementa y decrementa correctamente.
- Fase 2: Las alertas críticas de MME caído se disparan en menos de 60s tras el `docker stop`. `mme_ue_count` cae a 0. Tras `docker start`, `mme_ue_count` vuelve a 1 y las alertas se resuelven.

---

**Flujo completo del escenario:**
1. Attach (EPS Attach Request → Accept)
2. EPS Bearer Establishment (Default Bearer Setup)
3. Tráfico de datos activo (ping en loop)
4. Fault Injection — `docker stop mme` → degradación → alertas
5. Recuperación — `docker start mme` → re-attach automático o manual
6. EPS Bearer Release (liberación del bearer de datos)
7. Detach (UE-initiated Detach)

---

### Fase 1 — Flujo completo base

**Pasos:**
1. Attach (EPS Attach Request → Accept)
2. EPS Bearer Establishment (Default Bearer Setup)
3. Tráfico de datos activo (ping en loop)
4. EPS Bearer Release (liberación del bearer de datos)
5. Detach (UE-initiated Detach)

**Valor pedagógico:**  
Muestra el procedimiento de señalización LTE completo extremo a extremo, observable en métricas, logs y trazas del módulo O&M. Sirve como línea base para comparación con E3 y como estado estable previo al fault injection.

**Métricas clave (fase normal):**
- `mme_ue_count` — sube a 1 durante el attach, vuelve a 0 en detach
- `mme_session` — refleja la sesión activa
- `bearers_active` — incrementa en bearer establishment
- `gtp2_sessions_active` — activo durante la sesión de datos

---

### Fase 2 — Fault Injection MME

**NF a derribar:** `mme` (contenedor Open5GS)

**Impacto esperado:** Pérdida total de señalización NAS. El eNB detecta pérdida de conexión S1. El UE entra en estado IDLE. No es posible realizar nuevos attaches hasta la recuperación.

#### Subfase 2.1 — Preparación (estado estable)
1. Verificar attach exitoso y tráfico de datos activo desde Fase 1
2. Confirmar en Grafana: `mme_ue_count == 1`, `bearers_active == 1`

#### Subfase 2.2 — Inyección del fallo
3. Ejecutar `docker stop mme`
4. Registrar timestamp exacto para correlación con telemetría

#### Subfase 2.3 — Observación de la degradación
5. Observar en Grafana la caída de métricas:
   - `container_health_status{name="mme"}` → 0
   - `mme_ue_count` → 0 abruptamente
   - `up{job="mme"}` → 0
   - `bearers_active` → 0
6. Verificar disparo de alertas críticas en Alertmanager
7. Observar en logs del eNB la pérdida de conexión S1
8. Confirmar interrupción del tráfico de datos (ping)
9. Notar que `mme_enb_count` permanece activo — el eNB sigue en pie

#### Subfase 2.4 — Recuperación
10. Ejecutar `docker start mme`
11. Esperar log `eNB-S1 accepted` en logs del MME — confirma que el core reconectó
12. Bajar el RAN completo limpiamente: `docker compose -f ran.yaml --profile ran-4g-srs down`
13. Levantar el eNB primero: `docker compose -f ran.yaml --profile ran-4g-srs up -d srsenb_zmq`
14. Esperar log `eNB-S1 accepted` en el MME — confirma que el eNB completó el S1 Setup
15. Levantar el srsUE: `docker compose -f ran.yaml --profile ran-4g-srs up -d srsue_zmq`
16. Verificar re-attach exitoso en logs del MME: `Attach complete`
17. Confirmar restablecimiento del tráfico de datos
18. Confirmar en Grafana: `mme_ue_count` vuelve a 1
19. Confirmar resolución de alertas en Alertmanager

> **Nota de implementación (validada en E1):** El procedimiento más limpio y confiable de recuperación es bajar y levantar el RAN completo tras reiniciar el MME. Esto garantiza un estado completamente limpio en todos los nodos — MME, eNB y srsUE — evitando contextos S1/RRC stale.

**Métricas clave (fault injection):**
- `container_health_status{name="mme"}` — 0 durante fallo, 1 en recuperación
- `up{job="mme"}` — 0 cuando MME deja de responder a Prometheus
- `mme_ue_count` — caída abrupta a 0, recuperación tras reinicio del UE
- `mme_enb_count` — permanece activo durante el fallo
- `testbed_containers_stopped` — incrementa en fallo, decrece en recuperación
- `bearers_active` — cae a 0 con el fallo

**Alertas asociadas:**
- `absent(mme_ue_count)` → MME caído — la métrica desaparece completamente cuando el contenedor se detiene (no devuelve 0)
- `container_health_status{name="mme"} == 0` → **CRÍTICO:** MME caído
- `up{job="mme"} == 0` → MME no responde a Prometheus
- `mme_ue_count == 0` y `mme_enb_count > 0` → eNBs activos pero sin UEs
- `testbed_containers_stopped > 0` → Contenedor detenido inesperadamente

---

## E2 — Multi-eNB con UEs mixtos (4G)

**Arquitectura:** Open5GS EPC + srsRAN 4G (ZMQ)

**Duración estimada:** 15-20 minutos

**Prerrequisitos:**
- Core 4G levantado: `docker compose -f 4G_core.yaml up -d`
- Módulo O&M levantado: `docker compose -f services.yaml up -d`
- Suscriptores en MongoDB: IMSI 895 (UE válido), 902 (bad_ki), 903 (bad_apn) — credenciales correctas en DB, errores en .conf
- IMSI 901 **no** insertado en MongoDB — ese es el error del bad_imsi

**Herramientas de observación:**
- **Grafana** — `mme_enb_count`, `mme_ue_count`, `bearers_active`
- **Loki** — logs de rechazo por tipo: HSS reject, Authentication Failure, PDN reject
- **Tempo** — trazas de intentos fallidos para identificar punto de fallo por NF
- **Alertmanager** — alertas de autenticación fallida

**Topología:**
- 4 eNBs: `srsenb_zmq` (eNB1), `srsenb_zmq2` (eNB2), `srsenb_zmq3` (eNB3), `srsenb_zmq4` (eNB4)
- 4 srsUEs: `srsue_zmq` (válido), `srsue_zmq_bad_ki`, `srsue_zmq_bad_imsi`, `srsue_zmq_bad_apn`

**Distribución de UEs:**

| UE | eNB | IMSI | Configuración | Resultado esperado |
|----|-----|------|---------------|-------------------|
| `srsue_zmq` | eNB1 | 895 | Credenciales válidas | Attach exitoso |
| `srsue_zmq_bad_ki` | eNB2 | 902 | Ki incorrecto en .conf | Authentication Failure (MAC failure) |
| `srsue_zmq_bad_imsi` | eNB3 | 901 | IMSI no registrado en MongoDB | Attach Reject — IMSI unknown in HLR |
| `srsue_zmq_bad_apn` | eNB4 | 903 | APN inválido en .conf | Attach exitoso, PDN rechazado (ESM) |

> **Nota de implementación:** ZMQ en srsRAN 4G es punto a punto — un eNB solo puede atender un srsUE simultáneamente. El escenario multi-UE inválido se ejecuta secuencialmente: los UEs con fallos rápidos (autenticación, identidad) se desconectan antes de que el siguiente intente conectarse.

**Condiciones de éxito:**
- `mme_enb_count == 4` durante todo el experimento
- `mme_ue_count == 1` — solo el UE válido del eNB1
- `srsue_zmq_bad_ki` genera `Authentication failure(MAC failure)` — fallo en capa de autenticación
- `srsue_zmq_bad_imsi` genera `Attach reject [OGS_NAS_EMM_CAUSE:8]` — IMSI unknown in HLR
- `srsue_zmq_bad_apn` hace attach exitoso pero genera `Invalid APN[invalid_apn]` — fallo en capa de sesión (ESM)
- Las métricas muestran que ningún UE inválido incrementa `mme_ue_count` permanentemente

> **✅ Validado experimentalmente (2026-04-13):** El fallo se observó correctamente. El UE con IMSI 902 conectó al eNB2 (`CellID[0x19c02]`), fue identificado por el MME, pero falló en autenticación con `Authentication failure(MAC failure)` — causa `OGS_NAS_EMM_CAUSE[20]`. El MME liberó el contexto inmediatamente (`UE Context Release`), `mme_ue_count` subió a 2 brevemente durante el intento y volvió a 1. El UE válido (IMSI 895) no fue interrumpido en ningún momento.

---

### Fase 1 — Preparación
1. Core 4G levantado y módulo O&M activo
2. Levantar eNB1: `docker compose -f ran.yaml --profile ran-4g-srs up -d srsenb_zmq`
3. Esperar `eNB-S1 accepted` en logs del MME
4. Levantar eNB2: `docker compose -f ran.yaml --profile ran-4g-e2 up -d srsenb_zmq2`
5. Esperar segundo `eNB-S1 accepted` en logs del MME
6. Levantar eNB3: `docker compose -f ran.yaml --profile ran-4g-e2 up -d srsenb_zmq3`
7. Esperar tercer `eNB-S1 accepted` en logs del MME
8. Levantar eNB4: `docker compose -f ran.yaml --profile ran-4g-e2 up -d srsenb_zmq4`
9. Esperar cuarto `eNB-S1 accepted` en logs del MME
10. Confirmar en Grafana: `mme_enb_count == 4`

### Fase 2 — Conexión del UE válido
7. Levantar UE1: `docker compose -f ran.yaml --profile ran-4g-srs up -d srsue_zmq`
8. Verificar attach exitoso en logs del MME: `Attach complete IMSI[001011234567895]`
9. Iniciar tráfico de datos: `docker exec srsue_zmq ping -I tun_srsue 8.8.8.8`
10. Confirmar en Grafana: `mme_ue_count == 1`, `bearers_active == 1`

### Fase 3 — Conexión de UEs inválidos (secuencial)

#### UE bad_ki — fallo de autenticación
11. Levantar UE bad_ki: `docker compose -f ran.yaml --profile ran-4g-e2 up -d srsue_zmq_bad_ki`
12. Observar en logs del MME: `Authentication failure(MAC failure) IMSI[001011234567902]`
13. Observar `UE Context Release [Action:3]` — el MME libera el contexto inmediatamente
14. Confirmar en Grafana que `mme_ue_count` sube brevemente a 2 y vuelve a 1

> **✅ Validado (2026-04-13):** Causa `OGS_NAS_EMM_CAUSE[20]` (MAC failure) — el MME encontró el suscriptor en el HSS pero la derivación de claves falló por Ki incorrecto. `mme_ue_count` spike transitorio a 2, vuelve a 1.

#### UE bad_imsi — fallo de identidad
15. Levantar UE bad_imsi: `docker compose -f ran.yaml --profile ran-4g-e2 up -d srsue_zmq_bad_imsi`
16. Observar en logs del MME: `Authentication Information failed [5001]` → `Attach reject [OGS_NAS_EMM_CAUSE:8]`
17. Observar `UE Context Release` — contexto liberado sin incrementar `mme_ue_count`
18. Verificar que el tráfico de datos del UE válido no se interrumpe

> **✅ Validado (2026-04-13):** Causa `OGS_NAS_EMM_CAUSE[8]` (IMSI unknown in HLR) — el MME consultó el HSS pero el suscriptor no existe en MongoDB. El fallo ocurre antes de iniciar la autenticación, a diferencia del bad_ki donde sí se inicia. `CellID[0x19d03]` confirma conexión al eNB3 correcto.

**Contraste pedagógico clave:**
- **bad_ki** — suscriptor existe en DB, falla en derivación de claves (capa de autenticación)
- **bad_imsi** — suscriptor no existe en DB, el HSS rechaza la consulta antes de autenticar (capa de identidad)

### Fase 4 — Observación de telemetría
23. Verificar en Loki los logs de rechazo de los tres UEs inválidos
24. Revisar trazas distribuidas — bad_ki falla en autenticación (HSS), bad_imsi antes de auth (HSS), bad_apn después de attach (ESM)
25. Confirmar que `mme_enb_count` se mantiene en 4 durante todo el experimento

**Métricas clave:**
- `mme_enb_count` — debe mostrar 4 eNBs registrados
- `mme_ue_count` — solo refleja el UE válido (1), con spikes transitorios durante intentos fallidos
- `mme_session` — spike a 1 durante el intento de bad_apn (sesión creada y destruida)
- `bearers_active` — solo el bearer del UE válido
- `gtp2_sessions_active` — solo la sesión del UE válido

**Alertas asociadas:**
- `mme_ue_count < mme_enb_count` sostenido → eNBs activos pero con UEs fallidos

---

## E3 — Flujo completo + Fault Injection AMF (5G)

**Arquitectura:** Open5GS 5GC + srsRAN Project (`srsgnb_zmq`) + srsUE

> **Nota:** Se usa srsRAN Project (no UERANSIM) para este escenario porque su comportamiento ante pérdida de N2 es más predecible y la recuperación tras reinicio del AMF está mejor documentada. Mantiene consistencia con E4.

**Duración estimada:** 10-15 minutos

**Prerrequisitos:**
- Core 5G levantado: `docker compose -f 5G_core.yaml up -d`
- Módulo O&M levantado: `docker compose -f services.yaml up -d`
- UE1 provisionado en la WebUI (SUPI, K, OPc válidos, SST=1)

**Herramientas de observación:**
- **Grafana** — métricas en tiempo real: `amf_ue_count`, `fivegs_upffunction_upf_sessionnbr`, `container_health_status`
- **Loki** — logs de señalización N2/NGAP y NAS del AMF
- **Tempo** — trazas del flujo Registration → PDU Session → Deregistration
- **Alertmanager** — alertas críticas de contenedor caído en Fase 2

**Topología:**
- 1 gNB srsRAN Project (`srsgnb_zmq`)
- 1 srsUE con credenciales válidas

**Condiciones de éxito:**
- Fase 1: `amf_ue_count` sube a 1 durante el registro y vuelve a 0 en deregistration. Las trazas muestran los 7 pasos del flujo completos. `fivegs_upffunction_upf_sessionnbr` incrementa y decrementa correctamente.
- Fase 2: Las alertas críticas de AMF caído se disparan en menos de 60s tras el `docker stop`. `amf_ue_count` cae a 0. Tras `docker start`, `amf_ue_count` vuelve a 1 y las alertas se resuelven.

---

**Flujo completo del escenario:**
1. Registration (Initial Registration Request → Accept)
2. PDU Session Establishment
3. Tráfico de datos activo (ping en loop)
4. Fault Injection — `docker stop amf` → degradación → alertas
5. Recuperación — `docker start amf` → re-registration automático o manual
6. PDU Session Release (liberación de la sesión de datos)
7. Deregistration (UE-initiated Deregistration)

---

### Fase 1 — Flujo completo base

**Pasos:**
1. Registration (Initial Registration Request → Accept)
2. PDU Session Establishment
3. Tráfico de datos activo (ping en loop)
4. PDU Session Release (liberación de la sesión de datos)
5. Deregistration (UE-initiated Deregistration)

**Valor pedagógico:**  
Muestra el procedimiento de señalización 5G SA completo extremo a extremo. Sirve como línea base para comparación con E1 y permite contrastar las diferencias de arquitectura y señalización entre 4G EPC y 5G SA.

**Métricas clave (fase normal):**
- `fivegs_amffunction_rm_reginitreq` / `fivegs_amffunction_rm_reginitsucc` — request y éxito de registro
- `amf_ue_count` — sube a 1 durante el registro
- `fivegs_smffunction_sm_pdusessioncreationreq` / `fivegs_smffunction_sm_pdusessioncreationsucc`
- `fivegs_upffunction_upf_sessionnbr` — sesión activa en UPF
- `fivegs_ep_n3_gtp_indatapktn3upf` / `fivegs_ep_n3_gtp_outdatapktn3upf` — tráfico de datos

---

### Fase 2 — Fault Injection AMF

**NF a derribar:** `amf` (contenedor Open5GS)

**Impacto esperado:** Pérdida total de señalización NAS 5G. El gNB pierde la conexión N2. Todos los UEs quedan desregistrados. No es posible realizar nuevas registrations hasta la recuperación.

#### Subfase 2.1 — Preparación (estado estable)
1. Verificar registration exitoso y tráfico de datos activo desde Fase 1
2. Confirmar en Grafana: `amf_ue_count == 1`, `fivegs_upffunction_upf_sessionnbr == 1`

#### Subfase 2.2 — Inyección del fallo
3. Ejecutar `docker stop amf`
4. Registrar timestamp exacto para correlación con telemetría

#### Subfase 2.3 — Observación de la degradación
5. Observar en Grafana la caída de métricas:
   - `container_health_status{name="amf"}` → 0
   - `amf_ue_count` → 0 abruptamente
   - `up{job="amf"}` → 0
   - `fivegs_upffunction_upf_sessionnbr` → 0
6. Verificar disparo de alertas críticas en Alertmanager
7. Observar en logs del gNB la pérdida de conexión N2: `NG connection lost`
8. Confirmar interrupción del tráfico de datos (ping)
9. Notar que `amf_gnb_count` permanece activo — el gNB sigue en pie

#### Subfase 2.4 — Recuperación
10. Ejecutar `docker start amf`
11. Esperar log `gNB-N2 accepted` en logs del AMF — confirma que el core reconectó
12. Bajar el RAN completo limpiamente: `docker compose -f ran.yaml --profile ran-5g-srs down`
13. Levantar el gNB primero: `docker compose -f ran.yaml --profile ran-5g-srs up -d srsgnb_zmq`
14. Esperar log `gNB-N2 accepted` en el AMF — confirma que el gNB completó el NG Setup
15. Levantar el srsUE: `docker compose -f ran.yaml --profile ran-5g-srs up -d srsue_5g_zmq`
16. Verificar re-registration exitoso en logs del AMF: `Registration complete`
15. Confirmar restablecimiento del tráfico de datos
16. Confirmar en Grafana: `amf_ue_count` vuelve a 1
17. Confirmar resolución de alertas en Alertmanager

> **Nota de implementación (validada en E3):** El procedimiento más limpio y confiable de recuperación es bajar y levantar el RAN completo tras reiniciar el AMF, en lugar de reiniciar gNB y srsUE por separado. Esto garantiza un estado completamente limpio en todos los nodos — AMF, gNB y srsUE — evitando contextos N2/RRC stale. El paso clave es esperar la confirmación de reconexión del gNB al AMF antes de bajar el RAN.

**Métricas clave (fault injection):**
- `container_health_status{name="amf"}` — 0 durante fallo, 1 en recuperación
- `up{job="amf"}` — 0 cuando AMF deja de responder a Prometheus
- `amf_ue_count` — caída abrupta a 0, recuperación tras reinicio del UE
- `amf_gnb_count` — permanece activo durante el fallo
- `testbed_containers_stopped` — incrementa en fallo, decrece en recuperación
- `fivegs_upffunction_upf_sessionnbr` — cae a 0 con el fallo

**Alertas asociadas:**
- `absent(amf_ue_count)` → AMF caído — la métrica desaparece completamente cuando el contenedor se detiene (por analogía con E1 validado)
- `fivegs_amffunction_rm_reginitreq > 0` y `fivegs_amffunction_rm_reginitsucc == 0` durante más de 30s → Registration fallido
- `container_health_status{name="amf"} == 0` → **CRÍTICO:** AMF caído
- `up{job="amf"} == 0` → AMF no responde a Prometheus
- `amf_ue_count == 0` y `amf_gnb_count > 0` → gNBs activos pero sin UEs
- `testbed_containers_stopped > 0` → Contenedor detenido inesperadamente

---

## E4 — Multi-gNB con slicing y UEs mixtos (5G)

**Arquitectura:** Open5GS 5GC + srsRAN Project (ZMQ) + UERANSIM

**Duración estimada:** 20-25 minutos

**Prerrequisitos:**
- Core 5G levantado: `docker compose -f 5G_core.yaml up -d`
- Módulo O&M levantado: `docker compose -f services.yaml up -d`
- Suscriptores en MongoDB: 895 (srsRAN válido), 896 (UERANSIM gNB1 SST=1), 898 (gNB2 SST=1), 899 (gNB2 SST=2), 906 (K incorrecto), 908 (DNN incorrecto), 909 (SST=3)
- IMSI 905 **no** insertado en MongoDB (SUPI no registrado)

**Herramientas de observación:**
- **Grafana** — `amf_gnb_count`, `amf_ue_count`, `fivegs_amffunction_amf_authreject`, `amf_ue_allowed_slices_count`
- **Loki** — logs de rechazo por NF: AMF (slice), AUSF (AKA), UDM (SUPI), SMF (DNN)
- **Tempo** — trazas de intentos fallidos mostrando el NF exacto donde falla cada UE
- **Alertmanager** — alertas de auth rejects y diferencial de registrations

**Topología:**
- 1 gNB srsRAN Project (`srsgnb_zmq`) + 1 srsUE (`srsue_5g_zmq`) — perfil `ran-5g-srs`
- 1 gNB UERANSIM (`nr_gnb`, SST=1 básico) — perfil `ran-5g-e4`
- 1 gNB UERANSIM (`nr_gnb2`, anuncia SST=1 y SST=2) — perfil `ran-5g-e4`
- 1 UE UERANSIM válido gNB1 SST=1 (`nr_ue`) — IMSI 896
- 1 UE UERANSIM válido gNB2 SST=1 (`nr_ue2`) — IMSI 898
- 1 UE UERANSIM válido gNB2 SST=2 (`nr_ue3`) — IMSI 899
- 3 UEs UERANSIM inválidos gNB1 (`nr_ue_bad_supi`, `nr_ue_bad_ki`, `nr_ue_bad_dnn`)
- 1 UE UERANSIM inválido gNB2 (`nr_ue_bad_sst`)

**Distribución de UEs:**

| UE | gNB | IMSI | Configuración | Resultado esperado |
|----|-----|------|---------------|-------------------|
| `srsue_5g_zmq` | srsgnb_zmq | 895 | Válido srsRAN | Registration + PDU Session exitosos |
| `nr_ue` | gNB1 | 896 | Válido SST=1 | Registration + PDU Session exitosos |
| `nr_ue2` | gNB2 | 898 | Válido SST=1 | Registration + PDU Session exitosos |
| `nr_ue3` | gNB2 | 899 | Válido SST=2 | Registration + PDU Session en slice 2 |
| `nr_ue_bad_supi` | gNB1 | 905 | SUPI no registrado | `Cannot find SUCI [404]` → Reject [7] |
| `nr_ue_bad_ki` | gNB1 | 906 | K incorrecto en .yaml | `Auth failure MAC` → Reject [111] |
| `nr_ue_bad_dnn` | gNB1 | 908 | DNN inválido en .yaml | Attach exitoso, `DNN_NOT_SUPPORTED` |
| `nr_ue_bad_sst` | gNB2 | 909 | SST=3 inexistente en .yaml | `Cannot find NSSAI SST:3` → Reject [62] |

---

> **Nota de configuración (validada en E4):** El `amf.yaml` debe tener SST=2 en `plmn_support` además de SST=1. Sin este cambio el AMF rechaza todos los UEs con SST=2 con `Registration reject [62]` antes de iniciar la autenticación. Agregar en `amf/amf.yaml`:
> ```yaml
> plmn_support:
>   - plmn_id:
>       mcc: MCC
>       mnc: MNC
>     s_nssai:
>       - sst: 1
>       - sst: 2
> ```

### Fase 1 — Preparación
1. Core 5G levantado y módulo O&M activo
2. Levantar gNB srsRAN primero — sirve como referencia de conectividad N2: `docker compose -f ran.yaml --profile ran-5g-srs up -d srsgnb_zmq`
3. Esperar `gNB-N2 accepted` en logs del AMF
4. Levantar gNB1 UERANSIM (SST=1 básico): `docker compose -f ran.yaml --profile ran-5g-e4 up -d nr_gnb`
5. Esperar segundo `gNB-N2 accepted` en logs del AMF
6. Levantar gNB2 UERANSIM (SST=1 y SST=2): `docker compose -f ran.yaml --profile ran-5g-e4 up -d nr_gnb2`
7. Esperar tercer `gNB-N2 accepted` en logs del AMF
8. Confirmar en Grafana: `amf_gnb_count == 3`

### Fase 2 — Conexión de UEs válidos
9. Levantar srsUE: `docker compose -f ran.yaml --profile ran-5g-srs up -d srsue_5g_zmq`
10. Levantar UE UERANSIM gNB1: `docker compose -f ran.yaml --profile ran-5g-e4 up -d nr_ue`
11. Levantar UE UERANSIM gNB2 SST=1: `docker compose -f ran.yaml --profile ran-5g-e4 up -d nr_ue2`
12. Levantar UE UERANSIM gNB2 SST=2: `docker compose -f ran.yaml --profile ran-5g-e4 up -d nr_ue3`
13. Confirmar en Grafana: `amf_ue_count == 4`, `amf_ue_allowed_slices_count` por UE
14. Iniciar tráfico de datos en los UEs válidos

### Fase 3 — Conexión de UEs inválidos (secuencial)
15. Levantar `nr_ue_bad_supi` → observar `Cannot find SUCI [404]` en AMF y `Registration reject [7]`
16. Confirmar que `amf_ue_count` no incrementa permanentemente
17. Levantar `nr_ue_bad_ki` → observar `Authentication failure` y `Registration reject [111]` en AMF
18. Observar incremento de `fivegs_amffunction_amf_authreject`
19. Levantar `nr_ue_bad_dnn` → `Registration complete` exitoso, luego `DNN_NOT_SUPPORTED_OR_NOT_SUBSCRIBED` — el UE queda registrado sin PDU Session
20. Levantar `nr_ue_bad_sst` → observar `Cannot find Requested NSSAI [SST:3]` y `Registration reject [62]` en AMF

### Fase 4 — Observación de telemetría y alertas
21. Verificar en Grafana que `amf_ue_count` refleja solo los 4 UEs válidos
22. Verificar que `amf_gnb_count` se mantiene en 3
23. Revisar en Loki los logs de rechazo por tipo de UE inválido
24. Revisar trazas distribuidas — cada fallo debe mostrar el NF donde ocurrió

**Métricas clave:**
- `amf_gnb_count` — debe mostrar 3 gNBs registrados (srsRAN + 2 UERANSIM)
- `amf_ue_count` — solo UEs exitosamente registrados (4)
- `fivegs_amffunction_amf_authreject` — rechazos de autenticación
- `amf_ue_allowed_slices_count` — slices permitidos por UE
- `fivegs_smffunction_sm_pdusessioncreationreq` vs `succ` — diferencial de PDU sessions

**Alertas asociadas:**
- `increase(fivegs_amffunction_amf_authreject[1m]) > 2` → Rechazos de autenticación repetidos
- `increase(fivegs_amffunction_rm_reginitreq[1m]) - increase(fivegs_amffunction_rm_reginitsucc[1m]) > 2` → Diferencial creciente entre intentos y éxitos

---

## Pendiente — Post-implementación de E2 y E4

### Reglas de alerta a calibrar
Una vez ejecutados los escenarios, ajustar los umbrales de las siguientes reglas con valores reales observados:

- Ventanas de tiempo óptimas para `increase()` en E2 y E4
- Umbral exacto de `authreject/authreq` que distingue E4 anómalo de ruido normal
- Tiempo mínimo antes de disparar alerta de `absent(mme_ue_count)` / `absent(amf_ue_count)` en E1/E3

### Fault injection adicionales a evaluar
Evaluar como casos de estudio adicionales una vez validados los 4 escenarios principales:

- **Caída del SMF (4G y 5G)** — ver si el AMF/MME detecta la pérdida y qué pasa con sesiones activas en curso
- **Caída del UPF** — plano de usuario afectado pero señalización intacta, observable en métricas de tráfico GTP
- **Comportamiento de `ues_active` stale** — validado en E1/E3: el SMF mantiene el contexto de sesión tras caída abrupta del MME/AMF ya que nunca recibe señal de liberación. Representa la diferencia entre fallo graceful y fallo abrupto en redes móviles. Evaluar si reinicio del SMF o timeout limpia el valor.

---

## Notas de implementación

**Handover — no incluido como escenario:**  
En 5G, srsRAN Project solo soporta handover intra-gNB y requiere obligatoriamente USRP X/N-series con dos cadenas RF. En 4G, el handover S1 con ZMQ requiere GNU Radio Companion como broker externo al stack Docker. Ambas restricciones impiden su implementación en el testbed virtualizado actual.

**Multi-UE con ZMQ en 4G:**  
Los sockets REQ/REPLY de ZMQ en srsRAN 4G solo permiten una conexión simultánea por par de puertos. Múltiples srsUEs en el mismo eNB requieren broker GRC. La solución adoptada en E2 es múltiples pares eNB+srsUE independientes.

**UERANSIM — solo 5G:**  
UERANSIM opera únicamente sobre NGAP (N2) e interfaces 5G SA. No tiene soporte para S1AP ni EPC 4G.

**Estabilidad UERANSIM multi-UE:**  
En escenarios de larga duración con múltiples UEs UERANSIM, se han reportado desconexiones espontáneas que impiden la reconexión. Los experimentos de E4 deben ejecutarse en ventanas de tiempo acotadas.

**srsRAN Project vs UERANSIM en E3:**  
Se usa srsRAN Project para E3 (fault injection) por su comportamiento más predecible ante pérdida de N2 y mejor documentación de recuperación. UERANSIM se reserva para E4 donde la ligereza de sus instancias es la ventaja principal.
