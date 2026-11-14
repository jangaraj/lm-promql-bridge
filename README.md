# LogicMonitor PromQL Bridge (`lm-promql-bridge`)

A high-performance, lightweight Go proxy bridge that translates **Prometheus API v1 (PromQL)** specifications into **LogicMonitor OpenTelemetry REST API v3** (available in LogicMonitor 247+). 

---

## Why This Project?

LogicMonitor provides robust OpenTelemetry metric ingestion and REST APIs, but native monitoring tools, dashboards, and enterprise visualization platforms like Grafana rely heavily on the **Prometheus API v1 and PromQL** ecosystem. 

Deploying a heavy intermediate time-series database (like Thanos or Mimir) just to bridge this gap introduces unnecessary complexity and operational overhead. `lm-promql-bridge` sits directly between any Prometheus-compatible client (such as Grafana) and LogicMonitor, translating queries on-the-fly.

### Key Advantages

1. **Extended Time Ranges & Chunking**: LogicMonitor enforces a strict 30-day single query duration limit. The bridge automatically chunks query ranges exceeding 25 days into parallel segments, executes them concurrently using `errgroup`, and seamlessly merges and deduplicates the time-series data.
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

---

## Configuration & Environment Variables

Configure the bridge using environment variables or a `.env` file:

| Variable | Description | Required | Default |
| :--- | :--- | :--- | :--- |
| `LM_PORTAL` | Your LogicMonitor portal domain (e.g., `yourcompany.logicmonitor.com`) | **Yes** | — |
| `LM_ACCESS_ID` | LogicMonitor API Access ID (for LMv1 HMAC auth) | *Conditional* | — |
| `LM_ACCESS_KEY` | LogicMonitor API Access Key (for LMv1 HMAC auth) | *Conditional* | — |
| `LM_BEARER_TOKEN` | LogicMonitor API Bearer Token (alternative to LMv1 credentials) | *Conditional* | — |
| `SERVER_PORT` | Bridge HTTP endpoint port | No | `8080` |
| `SERVER_USERNAME` | Optional username for securing bridge HTTP endpoints | No | `""` (Disabled) |
| `SERVER_PASSWORD` | Optional password for securing bridge HTTP endpoints | No | `""` (Disabled) |
| `DEBUG` | Enable verbose debug logging (`true`/`false`) | No | `false` |

---

## Grafana Integration

1. In Grafana, navigate to **Connections > Data sources > Add data source**.
2. Select **Prometheus**.
3. Set the **Connection** URL to point to your bridge instance (e.g., `http://localhost:3000`).
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

### Running Tests
```bash
make test
```
