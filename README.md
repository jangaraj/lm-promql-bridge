# LogicMonitor PromQL Bridge (`lm-promql-bridge`)

A high-performance, lightweight Go proxy bridge that translates **Prometheus API v1 (PromQL)** specifications into **LogicMonitor OpenTelemetry REST API v3** (available in LogicMonitor 247+). 

---

## Why This Project?

LogicMonitor provides robust OpenTelemetry metric ingestion and REST APIs, but native monitoring tools, dashboards, and enterprise visualization platforms like Grafana rely heavily on the **Prometheus API v1 and PromQL** ecosystem. 

Deploying a heavy intermediate time-series database (like Thanos or Mimir) just to bridge this gap introduces unnecessary complexity and operational overhead. `lm-promql-bridge` sits directly between any Prometheus-compatible client (such as Grafana) and LogicMonitor, translating queries on-the-fly.

### Key Advantages

1. **Extended Time Ranges & Chunking**: LogicMonitor enforces a strict 30-day single query duration limit. The bridge automatically chunks range queries and bounded metadata queries exceeding 25 days into parallel segments (up to 100 chunks / 2,500 days using 10 concurrent workers), and merges and deduplicates the results.
2. **Grafana & Ecosystem Compatibility**: Instantly unlock the vast Grafana dashboard ecosystem, alerting tools, and PromQL tooling without modifying your underlying monitoring telemetry.
3. **High-Performance Architecture**: 
   - Aggressive HTTP transport keep-alives and connection pooling.
   - High-performance JSON parsing via `json-iterator/go`.
   - Inbound concurrency limiting to prevent resource exhaustion and upstream rate-limiting (`429 Too Many Requests`).
   - Full context propagation for immediate request cancellation when dashboard panels time out or close.
4. **Flexible Authentication & Deployment**: Supports both LogicMonitor `LMv1` HMAC signatures and Bearer tokens, alongside optional server-side HTTP Basic Authentication for secure edge exposure.

---

## Supported Prometheus Endpoints

- `/api/v1/query` (Instant queries)
- `/api/v1/query_range` (Range queries with automatic parallel chunking)
- `/api/v1/labels` (Label discovery and filtering)
- `/api/v1/label/__name__/values` (Metric name discovery and filtering)
- `/api/v1/label/{name}/values` (Label value extraction)
- `/api/v1/series` (Series matching)
- `/api/v1/metadata` (Metric metadata discovery)
- `/healthz` (Unauthenticated health check endpoint)

---

## Configuration & Environment Variables

Configure the bridge using environment variables or a `.env` file:

| Variable | Description | Required | Default |
| :--- | :--- | :--- | :--- |
| `LM_PORTAL` | Your LogicMonitor portal domain (e.g., `yourcompany.logicmonitor.com` or `yourcompany`) | **Yes** | — |
| `LM_ACCESS_ID` | LogicMonitor API Access ID (for LMv1 HMAC auth) | *Conditional* | — |
| `LM_ACCESS_KEY` | LogicMonitor API Access Key (for LMv1 HMAC auth) | *Conditional* | — |
| `LM_BEARER_TOKEN` | LogicMonitor API Bearer Token (takes precedence over LMv1 credentials if both are set) | *Conditional* | — |
| `SERVER_PORT` | Bridge HTTP endpoint port | No | `8080` |
| `SERVER_USERNAME` | Optional username for HTTP Basic Auth (must be set with `SERVER_PASSWORD`) | No | `""` (Disabled) |
| `SERVER_PASSWORD` | Optional password for HTTP Basic Auth (must be set with `SERVER_USERNAME`) | No | `""` (Disabled) |
| `SERVER_TLS_ENABLE` | Enable HTTPS (`true`/`false`, `yes`/`no`, `1`/`0`) | No | `false` |
| `SERVER_TLS_CERT` | Path to TLS certificate file (set with `SERVER_TLS_KEY`, or omit both for a self-signed cert) | No | `""` |
| `SERVER_TLS_KEY` | Path to TLS private key file (set with `SERVER_TLS_CERT`, or omit both for a self-signed cert) | No | `""` |
| `REQUEST_TIMEOUT` | Upstream LogicMonitor HTTP request timeout in seconds | No | `120` |
| `DEBUG` | Enable verbose debug logging (`true`/`false`, `yes`/`no`, `1`/`0`) | No | `false` |

---

## Grafana Integration

1. In Grafana, navigate to **Connections > Data sources > Add data source**.
2. Select **Prometheus**.
3. Set the **Connection** URL to point to your bridge instance (e.g., `http://localhost:8080`).
4. If `SERVER_USERNAME` and `SERVER_PASSWORD` are configured on the bridge, enable **Basic auth** in the Grafana data source settings and supply the corresponding credentials.
5. Click **Save & test**. You can now build dashboards and run PromQL queries directly against your LogicMonitor metrics.

---

## Building and Running

### Local Build
```bash
make all
```

### Running the Bridge
```bash
export LM_PORTAL="yourcompany.logicmonitor.com"
export LM_ACCESS_ID="your_access_id"
export LM_ACCESS_KEY="your_access_key"
./bin/lm-promql-bridge
```

Enable HTTPS with an automatically generated self-signed certificate:
```bash
export SERVER_TLS_ENABLE=true
./bin/lm-promql-bridge
```

Or provide a certificate and matching private key:
```bash
export SERVER_TLS_ENABLE=true
export SERVER_TLS_CERT=/path/to/server.crt
export SERVER_TLS_KEY=/path/to/server.key
./bin/lm-promql-bridge
```

### Running Tests
```bash
make test
```

### Run with Docker (GitHub Container Registry)

```bash
docker pull ghcr.io/monitoringartist/lm-promql-bridge:latest

docker run --rm \
  --name lm-promql-bridge \
  -p 8080:8080 \
  -e LM_PORTAL="yourcompany.logicmonitor.com" \
  -e LM_ACCESS_ID="your_access_id" \
  -e LM_ACCESS_KEY="your_access_key" \
  ghcr.io/monitoringartist/lm-promql-bridge:latest
```

To use a LogicMonitor bearer token instead of LMv1 credentials, replace `LM_ACCESS_ID` and `LM_ACCESS_KEY` with:

```bash
-e LM_BEARER_TOKEN="your_bearer_token"
```

The bridge will be available at `http://localhost:8080`.

### Run with Docker Compose (Bridge + Grafana)

Create a `.env` file with your LogicMonitor credentials (`LM_PORTAL` and either `LM_BEARER_TOKEN` or `LM_ACCESS_ID`/`LM_ACCESS_KEY`), then start both services:

```bash
docker compose up -d
```

- Bridge: `http://localhost:8080`
- Grafana: `http://localhost:3000` (default credentials: `admin` / `admin`)
