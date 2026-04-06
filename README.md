# Distributed API Gateway with Rate Limiting, Analytics & Auto-Scaling

## Overview

This project is a **production-style distributed API Gateway** built using:

* **Go (Golang)** → High-performance API layer
* **Redis** → Rate limiting + event streaming
* **MySQL** → Persistent analytics storage
* **Kubernetes (Minikube)** → Container orchestration
* **Prometheus + Grafana** → Monitoring & visualization

---

## What This System Does

* Enforces **rate limiting per API key**
* Logs requests asynchronously using **Redis Streams**
* Processes logs using a **worker service**
* Stores analytics in **MySQL**
* Displays insights via a **dashboard**
* Auto-scales using **Kubernetes HPA (Horizontal Pod Autoscaler)**

---

## Architecture

```text
Client → API Gateway (Go)
        ↓
     Redis (Rate limit + Stream)
        ↓
     Worker Service
        ↓
     MySQL (Analytics DB)
        ↓
     Dashboard (Charts)

Kubernetes:
Gateway Pods ↔ Service ↔ HPA ↔ Prometheus ↔ Grafana
```

---

## Tech Stack

| Layer            | Technology           |
| ---------------- | -------------------- |
| API Layer        | Go (net/http)        |
| Rate Limiting    | Redis                |
| Async Queue      | Redis Streams        |
| Worker           | Go                   |
| Database         | MySQL                |
| Containerization | Docker               |
| Orchestration    | Kubernetes           |
| Monitoring       | Prometheus + Grafana |

---

## 🐳 Local Setup (Docker)

## 1. Create `.env` file (IMPORTANT)

Create a file named `.env` in the root directory:

```
MYSQL_ROOT_PASSWORD=rootpassword

DB_USER=root
DB_PASS=rootpassword
DB_HOST=mysql:3306
DB_NAME=api_gateway_analytics
REDIS_ADDR=redis:6379
```

Do NOT push `.env` to GitHub  
Add this to `.gitignore`

### 2. Start services

```bash
docker compose up --build
```

---

### 3. Services

| Service     | Port |
| ----------- | ---- |
| API Gateway | 8080 |
| MySQL       | 3306 |
| Redis       | 6379 |

---

### 4. Test API

```bash
curl -H "x-api-key: user123" http://localhost:8080/test
```

---

### 5. Dashboard

Open:

```
http://localhost:8080/dashboard
```
![alt text](image.png)
---

## ☸️ Kubernetes Setup (Minikube)

### 1. Start cluster

```bash
minikube start
```

---

### 2. Use Minikube Docker

```bash
minikube docker-env | Invoke-Expression
```

---

### 3. Build image

```bash
docker build -t rate-limiter-gateway .
```

---

### 4. Apply manifests

```bash
kubectl apply -f .
```

---

### 5. Access service

```bash
minikube service gateway-service
```
![alt text](image.png)
---

## 📈 Auto Scaling (HPA)

* Scales based on **CPU utilization**
* Requires **resource requests**

```yaml
resources:
  requests:
    cpu: "100m"
  limits:
    cpu: "500m"
```

---

## 🔐 Configuration Management

### Secrets (Sensitive)

```bash
kubectl create secret generic gateway-secret --from-literal=DB_USER=root --from-literal=DB_PASS=rootpassword
```

---

### ConfigMap (Non-sensitive)

```yaml
DB_HOST: mysql:3306
DB_NAME: api_gateway_analytics
REDIS_ADDR: redis:6379
```

---

## 📊 Monitoring

### Prometheus

* Scrapes `/metrics` endpoint

### Grafana

* Visualizes:

  * Request count
  * Rate limiting
  
![alt text](image-2.png)
---

## Features Implemented

* ✅ Sliding window rate limiting (Redis)
* ✅ Per-user + per-endpoint limits
* ✅ Async logging (Redis Streams)
* ✅ Worker-based processing
* ✅ MySQL analytics
* ✅ Dashboard (Chart.js)
* ✅ Dockerized system
* ✅ Kubernetes deployment
* ✅ Horizontal Pod Autoscaling(HPA)
* ✅ Prometheus metrics
* ✅ Grafana dashboards
* ✅ Secrets + ConfigMaps

---

## ⚠️ Real Issues Faced & Fixes

### 1. Redis connection refused

**Cause:** Redis container not running
**Fix:** Start Redis / correct hostname

---

### 2. `go.mod not found`

**Cause:** Go module not initialized
**Fix:**

```bash
go mod init
```

---

### 3. HPA not scaling

**Cause:**

```text
missing CPU requests
```

**Fix:**

```yaml
resources:
  requests:
    cpu: "100m"
```

---

### 4. CPU not increasing

**Cause:** API too lightweight
**Fix:** Add artificial load for testing

---

### 5. Invalid UTF-8 (Secrets) using secrets.yaml

**Cause:** Incorrect base64 encoding
**Fix:** Use:

```bash
kubectl create secret ...
```

---

### 6. MySQL connection failed in Docker

**Cause:** Using localhost instead of service name
**Fix:**

```text
mysql:3306
```

---

### 7. Multiple containers not scaling (Docker)

**Cause:** Port conflict
**Fix:** Use load balancer (NGINX / K8s Service)

---

### 8. Dashboard empty in K8s

**Cause:** No data in DB
**Fix:** Generate traffic

---

### 9. CORS issues

**Fix:** Add middleware

---

### 10. Metrics showing zero

**Cause:** Metrics not incremented
**Fix:** Add Prometheus counters

---

## Key Learnings

### 🔹 System Design

* Stateless API with shared Redis
* Async processing improves performance

### 🔹 Scalability

* HPA works on **resource usage, not requests**
* CPU requests are mandatory

### 🔹 Redis vs MySQL

* Redis → fast, ephemeral
* MySQL → persistent analytics

### 🔹 Kubernetes

* Service handles load balancing
* Pods scale horizontally
* Secrets & ConfigMaps separate config

### 🔹 Observability

* Metrics + logs = production readiness

---
