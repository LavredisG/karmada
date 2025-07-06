# Network Connectivity Issue Report
## Docker Container to Host Service Communication Problem

**Date:** June 28, 2025  
**Issue Type:** Network Connectivity  
**Severity:** High  
**Status:** Resolved  

---

## Executive Summary

The Karmada scheduler running inside a Docker container experienced connectivity timeouts when attempting to communicate with the AHP (Analytic Hierarchy Process) service running on the host machine. The issue was resolved by configuring the UFW firewall to allow traffic from the Docker network to the specific service port.

---

## Problem Description

### Symptoms
- **Karmada scheduler** (running in `host-control-plane` container) could not connect to the AHP service
- **Target endpoint:** `http://172.18.0.1:6000/distribution_score`
- **Error message:** `dial tcp 172.18.0.1:6000: i/o timeout`
- **Service status:** AHP service was confirmed running and listening on `172.18.0.1:6000`

### Impact
- Complete failure of the custom distribution scoring functionality
- Karmada scheduler unable to make intelligent cluster selection decisions
- Development and testing workflow blocked

---

## Technical Investigation

### Network Topology Analysis
```
Docker Bridge Network: br-cf8561ad8755 (172.18.0.0/16)
├── Gateway: 172.18.0.1 (Host/AHP Service)
├── host-control-plane: 172.18.0.2
├── edge-control-plane: 172.18.0.3
├── fog-control-plane: 172.18.0.4
└── cloud-control-plane: 172.18.0.5
```

### Connectivity Tests
| Source | Destination | Protocol | Result |
|--------|-------------|----------|---------|
| Host laptop | 172.18.0.1:6000 | HTTP | ✅ Success |
| Container | 172.18.0.1:6000 | HTTP | ❌ Timeout |

### Root Cause Discovery

1. **Service Verification**
   ```bash
   sudo ss -tulpn | grep :6000
   # Output: tcp LISTEN 0 128 172.18.0.1:6000 0.0.0.0:* users:(("python3",pid=400977,fd=3))
   ```

2. **Firewall Analysis**
   ```bash
   sudo iptables -L INPUT -n -v
   # Revealed: Chain INPUT (policy DROP 26322 packets, 1190K bytes)
   
   sudo ufw status
   # Output: Status: active with default deny incoming policy
   ```

3. **Key Finding**
   - UFW (Uncomplicated Firewall) was blocking incoming traffic with default DENY policy
   - 26,322 packets had already been dropped
   - Only SSH (port 22) was allowed through the firewall
   - Container-to-host traffic was being treated as "incoming" and blocked

---

## Solution Implementation

### UFW Rule Addition
```bash
sudo ufw allow from 172.18.0.0/16 to any port 6000
```

### Rule Analysis
- **Source restriction:** `172.18.0.0/16` (Docker network only)
- **Destination:** Any interface on the host
- **Port:** 6000 (AHP service port)
- **Security benefit:** Maintains firewall protection while allowing necessary communication

### Verification Tests
```bash
# Test container connectivity
docker exec host-control-plane curl -v http://172.18.0.1:6000/

# Result: ✅ Connection successful
# Output: HTTP/1.1 404 NOT FOUND (expected for root path)

# Test actual endpoint
docker exec host-control-plane curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"distributions":[{"id":"test","metrics":{"cost":100}}],"criteria":{"cost":{"higher_is_better":false,"weight":1.0}}}' \
  http://172.18.0.1:6000/distribution_score

# Result: ✅ {"scores":[{"id":"test","score":100}]}
```

---

## Resolution Confirmation

### Before Fix
- Connection attempts resulted in timeouts after 30+ seconds
- Scheduler logs showed repeated connection failures
- Development workflow completely blocked

### After Fix
- Instant connection establishment
- Proper HTTP responses from AHP service
- Karmada scheduler successfully communicating with AHP service
- All functionality restored

---

## Lessons Learned

### Technical Insights
1. **Container networking complexity:** Even when containers and services are on the same Docker network, host firewall rules can still block communication
2. **UFW behavior:** Container-to-host traffic is treated as "incoming" traffic subject to INPUT chain rules
3. **Diagnostic approach:** Systematic testing (host-to-service vs container-to-service) was crucial for isolating the issue

### Best Practices
1. **Firewall documentation:** Maintain clear documentation of required firewall rules for containerized applications
2. **Network testing:** Include container-to-host connectivity tests in deployment procedures
3. **Security balance:** Use network-specific firewall rules rather than broad port openings

---

## Security Considerations

### Risk Assessment
- **Low risk:** Rule only allows access from Docker network (172.18.0.0/16)
- **No public exposure:** Port 6000 remains blocked from external networks
- **Principle of least privilege:** Only necessary traffic is allowed

### Alternative Solutions Considered
1. **Host network mode:** Would reduce security isolation
2. **Public IP binding:** Would expose service to external networks
3. **Port forwarding:** Would add unnecessary complexity

---

## Future Recommendations

1. **Documentation:** Add firewall requirements to project setup documentation
2. **Automation:** Include UFW rule creation in deployment scripts
3. **Monitoring:** Implement connectivity health checks in container startup procedures
4. **Testing:** Add container-to-host connectivity tests to CI/CD pipeline

---

## Appendix

### Environment Details
- **Host OS:** Linux
- **Docker Version:** Community Edition
- **UFW Version:** Uncomplicated Firewall
- **Network Driver:** Bridge
- **Container Runtime:** Docker

### Commands Reference
```bash
# Check UFW status
sudo ufw status

# View iptables INPUT chain
sudo iptables -L INPUT -n -v

# Check listening ports
sudo ss -tulpn | grep :6000

# Test connectivity from container
docker exec <container-name> curl -v http://172.18.0.1:6000/

# Add UFW rule for Docker network
sudo ufw allow from 172.18.0.0/16 to any port 6000
```

---

**Document Prepared By:** Network Troubleshooting Session  
**Review Status:** Complete  
**Distribution:** Development Team, System Administrators
