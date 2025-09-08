# O&M Module - Real Open5GS Metrics Summary
Generated: 2025-09-08 01:13:56
Deployment Type: 5G
Total Components: 15

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

- **pcf** (172.22.0.27)
  - Open5GS Endpoint: http://172.22.0.27:9091/metrics
  - O&M Module Endpoint: http://localhost:9093/metrics  
  - Health Check: http://localhost:9093/health
  - Educational Dashboard: http://localhost:9093/dashboard
  - Raw Data Debug: http://localhost:9093/debug/raw

- **smf** (172.22.0.7)
  - Open5GS Endpoint: http://172.22.0.7:9091/metrics
  - O&M Module Endpoint: http://localhost:9092/metrics  
  - Health Check: http://localhost:9092/health
  - Educational Dashboard: http://localhost:9092/dashboard
  - Raw Data Debug: http://localhost:9092/debug/raw

- **amf** (172.22.0.10)
  - Open5GS Endpoint: http://172.22.0.10:9091/metrics
  - O&M Module Endpoint: http://localhost:9091/metrics  
  - Health Check: http://localhost:9091/health
  - Educational Dashboard: http://localhost:9091/dashboard
  - Raw Data Debug: http://localhost:9091/debug/raw

### Quick Start Commands

1. **Start Real Metrics Collection:**
   ./om-module real-metrics

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
