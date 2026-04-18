# Product Requirements Document (PRD)

## Kubernetes-Native Network Fabric Control Plane with DHCP as a Managed Protocol

---

## 1. Overview

This system defines a **Kubernetes-native network control plane** where all network state is modeled as declarative resources inside Kubernetes.

Instead of running traditional DHCP servers with embedded logic, the system treats DHCP as a **managed protocol adapter** controlled by a central reconciliation engine.

The core abstraction is a **NetworkFabric**, which represents a complete, policy-driven network domain. DHCP is not a system itself — it is one of several **protocol behaviors controlled by the fabric**.

---

## 2. Core Principle

> DHCP is not implemented as a server.
> DHCP is a **reconciled outcome of network state in Kubernetes**.

The DHCP server is a **stateless translator**, while the operator determines:

- IP assignment
- reservations
- PXE boot behavior
- lease lifecycle

---

## 3. Goals

### 3.1 Primary Goals

- Represent full network configuration as Kubernetes CRDs
- Treat DHCP as a **controlled protocol behavior**, not a system
- Provide deterministic IP allocation via reconciliation
- Support:
  - Static reservations
  - Dynamic IP allocation
  - PXE boot orchestration
  - Multi-subnet fabrics

- Enable full auditability of all network decisions

### 3.2 Secondary Goals

- Enable GitOps-based network management
- Support bare-metal and VLAN-based deployments
- Allow subnet-level DHCP server deployment
- Provide a unified abstraction for future protocols (PXE, BOOTP, provisioning agents)
- Decouple protocol handling from network logic entirely

---

## 4. System Architecture

### 4.1 High-Level Design

```id="arch-1"
+---------------------------------------------------------------+
| Kubernetes API                                                |
|---------------------------------------------------------------|
| CRDs:                                                         |
|  • NetworkFabric   (root network domain)                      |
|  • Subnet          (network segments)                        |
|  • IPPool          (allocation engine)                       |
|  • NetworkPolicy   (reservations + PXE + rules)              |
|  • NetworkRequest  (transaction model - DHCP abstraction)    |
|  • Lease           (runtime allocation state)                |
+----------------------------+----------------------------------+
                             |
                             v
+---------------------------------------------------------------+
| Network Fabric Operator (Control Plane)                      |
|---------------------------------------------------------------|
| - Policy evaluation engine                                  |
| - IPAM allocator (per subnet/pool)                          |
| - DHCP request reconciler                                   |
| - Lease lifecycle manager                                   |
| - Subnet → DHCP server deployment controller                |
+----------------------------+----------------------------------+
                             |
                             v
+---------------------------------------------------------------+
| CoreDHCP Pods (Stateless Protocol Translators)              |
|---------------------------------------------------------------|
| - Convert DHCP packets → NetworkRequest CRDs               |
| - Watch NetworkRequest.status for response                 |
| - Emit DHCP replies (OFFER/ACK)                             |
| - Maintain no persistent state                              |
+---------------------------------------------------------------+
```

---

## 5. How DHCP is Controlled

### 5.1 Control Model

DHCP behavior is controlled through **three layers of declarative state**:

| Layer             | Resource         | Responsibility                            |
| ----------------- | ---------------- | ----------------------------------------- |
| Policy Layer      | `NetworkPolicy`  | Defines reservations, PXE boot, overrides |
| Allocation Layer  | `IPPool`         | Defines IP space and allocation strategy  |
| Transaction Layer | `NetworkRequest` | Represents DHCP packets and responses     |

The operator reconciles these into:

> A deterministic DHCP response

---

### 5.2 DHCP Control Flow

#### Step 1 — Packet Arrival

A DHCP client sends a DISCOVER/REQUEST.

CoreDHCP translates this into:

```yaml id="req-flow"
kind: NetworkRequest
spec:
  protocol: DHCP
  type: DISCOVER
  mac: aa:bb:cc:dd:ee:ff
  subnetHint: office-lan
```

---

#### Step 2 — Policy Evaluation

Operator evaluates:

1. `NetworkPolicy` (MAC match, vendor class, etc.)
2. `Subnet` (scope resolution)
3. `IPPool` (available addresses)

---

#### Step 3 — Decision

Operator produces one of:

- Static reservation
- PXE boot configuration
- Dynamic IP allocation

---

#### Step 4 — Lease Creation

If approved:

```yaml id="lease-flow"
kind: Lease
spec:
  mac: aa:bb:cc:dd:ee:ff
  ip: 192.168.1.101
  subnetRef: office-lan
```

---

#### Step 5 — Response Injection

Operator updates request:

```yaml id="response-flow"
status:
  phase: Approved
  response:
    ip: 192.168.1.101
    leaseTime: 2h
    dhcpOptions:
      dnsServers:
        - 8.8.8.8
```

---

#### Step 6 — DHCP Response

CoreDHCP watches CR status and responds to client.

---

## 6. Core CRDs

---

## 6.1 NetworkFabric

Represents a complete network domain.

```yaml id="fabric"
apiVersion: network.k8s.io/v1alpha1
kind: NetworkFabric
metadata:
  name: office
spec:
  cidr: 192.168.0.0/16

  dns:
    domain: office.local
    servers:
      - 8.8.8.8

  defaults:
    leaseTime: 2h
    gateway: 192.168.0.1
```

---

## 6.2 Subnet

Defines network segmentation.

```yaml id="subnet"
kind: Subnet
spec:
  fabricNameRef: office
  cidr: 192.168.1.0/24
  vlan: 10

  dhcp:
    enabled: true
```

---

## 6.3 IPPool

Defines allocation space.

```yaml id="pool"
kind: IPPool
spec:
  subnetNameRef: office-lan

  range:
    start: 192.168.1.100
    end: 192.168.1.200

  strategy: sequential
```

---

## 6.4 NetworkPolicy (DHCP Control Layer)

Controls reservations, PXE, overrides.

```yaml id="policy"
kind: NetworkPolicy
spec:
  match:
    mac: "00:11:22:33:44:55"

  action:
    type: Reservation
    ip: 192.168.1.50
```

PXE example:

```yaml id="pxe"
spec:
  match:
    vendorClass: "PXEClient"

  action:
    type: PXE
    boot:
      nextServer: 192.168.1.20
      filename: "bootx64.efi"
```

---

## 6.5 NetworkRequest

Represents DHCP transaction.

```yaml id="request"
kind: NetworkRequest
spec:
  protocol: DHCP
  type: DISCOVER
  mac: aa:bb:cc:dd:ee:ff
```

---

## 6.6 Lease

Represents active allocation.

```yaml id="lease"
kind: Lease
spec:
  mac: aa:bb:cc:dd:ee:ff
  ip: 192.168.1.101
```

---

## 7. DHCP Server (CoreDHCP Adapter)

### Responsibilities

- Stateless DHCP packet receiver
- Converts packets → `NetworkRequest`
- Waits for operator response
- Emits DHCP replies
- No IPAM logic exists locally

### Key Principle

> The DHCP server is a **protocol bridge only**

---

## 8. Operator Responsibilities

### 8.1 Core Functions

#### Policy Engine

- Match MAC / vendor class / subnet
- Resolve conflicting policies via priority

#### IPAM Engine

- Allocate IPs from IPPool
- Prevent collisions
- Track availability

#### Request Reconciler

- Process NetworkRequest objects
- Generate Lease + response

#### Subnet Controller

- Deploy one DHCP server per Subnet
- Bind to correct interface/VLAN

---

## 9. Deployment Model

### Per-Subnet DHCP Control

Each Subnet results in a dedicated DHCP instance:

```
Subnet office-lan → coredhcp-office-lan pod
Subnet pxe-lan    → coredhcp-pxe-lan pod
```

### Benefits

- Isolation per network segment
- Deterministic IP scope per pod
- VLAN-native design
- Independent scaling per subnet

---

## 10. Observability

### Metrics

- DHCP request rate per subnet
- Lease allocation latency
- IP pool utilization
- Policy match frequency

### Events

- Lease created / expired
- Policy matched
- Allocation failure
- Subnet provisioning

### Auditability

Every DHCP decision is traceable via:

- NetworkRequest
- Lease
- Policy match history (optional extension)

---

## 11. Failure Modes

| Failure         | Behavior               |
| --------------- | ---------------------- |
| Operator down   | No new leases assigned |
| DHCP pod down   | Subnet unavailable     |
| IP exhaustion   | Request rejected       |
| Policy conflict | Highest priority wins  |
| API latency     | Delayed DHCP responses |

---

## 12. Scalability Requirements

- 10k–100k leases per cluster
- Stateless DHCP pods (horizontal scaling)
- Sharded reconciliation by Subnet
- Event-driven architecture
- Eventually consistent IPAM state

---

## 13. Future Extensions

- DHCPv6 support
- Multi-cluster NetworkFabric federation
- VLAN/VXLAN orchestration
- Integration with NetBox/IPAM systems
- Boot orchestration beyond PXE (iPXE, HTTPBoot)
- Admission webhooks for policy validation
- Network topology graph CRD layer

---

## 14. Success Criteria

- DHCP fully controlled via Kubernetes CRDs
- No persistent state inside DHCP servers
- All IP decisions reproducible from cluster state
- PXE and reservations fully declarative
- Network behavior is policy-driven, not server-driven
- Operator is the single source of truth for all network decisions

---

## 15. Key Conceptual Outcome

This system is not a DHCP replacement.

It is:

> A **declarative network fabric control plane where DHCP is just one protocol behavior**
