# O&M Module - Real Open5GS Metrics Summary
Generated: 2025-09-08 03:56:16
Deployment Type: 4G
Total Components: 9

## Real Open5GS Metrics Collection

This O&M module now fetches REAL metrics from actual Open5GS components.
No simulation - 100% live telecommunications data!

### Supported Network Functions with Real Metrics

- **upf** (172.22.0.8)
  - Open5GS Endpoint: http://172.22.0.8:9091/metrics
  - O&M Module Endpoint: http://localhost:9094/metrics  
  - Health Check: http://localhost:9094/health
  - Educational Dashboard: http://localhost:9094/dashboard
  - Raw Data Debug: http://localhost:9094/debug/raw

- **mme** (172.22.0.9)
  - Open5GS Endpoint: http://172.22.0.9:9091/metrics
  - O&M Module Endpoint: http://localhost:9095/metrics  
  - Health Check: http://localhost:9095/health
  - Educational Dashboard: http://localhost:9095/dashboard
  - Raw Data Debug: http://localhost:9095/debug/raw

- **pcrf** (172.22.0.4)
  - Open5GS Endpoint: http://172.22.0.4:9091/metrics
  - O&M Module Endpoint: http://localhost:9096/metrics  
  - Health Check: http://localhost:9096/health
  - Educational Dashboard: http://localhost:9096/dashboard
  - Raw Data Debug: http://localhost:9096/debug/raw

- **smf** (172.22.0.7)
  - Open5GS Endpoint: http://172.22.0.7:9091/metrics
  - O&M Module Endpoint: http://localhost:9092/metrics  
  - Health Check: http://localhost:9092/health
  - Educational Dashboard: http://localhost:9092/dashboard
  - Raw Data Debug: http://localhost:9092/debug/raw

### Quick Start Commands

1. **Start Real Metrics Collection:**
   ./om-module orchestrator

2. **Test Real Metrics:**
   curl http://localhost:9091/metrics  # AMF real metrics
   curl http://localhost:9092/metrics  # SMF real metrics
   curl http://localhost:9091/debug/raw  # Raw Open5GS AMF data

3. **Configure Prometheus:**
   prometheus --config.file=prometheus_real_open5gs.yml

4. **Monitor Health:**
   curl http://localhost:9091/health  # AMF health

### Real Metrics Examples

The system collects actual Open5GS metrics like:
- fivegs_amffunction_rm_reginitreq (AMF registration requests)
- pfcp_sessions_active (SMF PFCP sessions)  
- ues_active (Active user equipments)
- gtp2_sessions_active (GTP sessions)
- ran_ue (Connected RAN UEs)

### Architecture

This O&M module fetches metrics from Open5GS components and re-exposes them with:
- Enhanced labeling for better organization
- Educational information for learning
- Health monitoring and status reporting
- Debug access to raw Open5GS data

**No simulation - Real telecommunications monitoring!** 🚀
