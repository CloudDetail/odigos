### Modifications

This is a modified version of [Odigos](https://github.com/odigos-io/odigos/tree/v1.0.76s) with the following changes:

1. Redesigned the agent injection process using webhooks - applications won't restart immediately after being selected for instrumentation. Instead, the agent will be injected during the next manual application restart.
2. Replaced the default agents with custom implementations for Java, Python, and NodeJS.
3. Removed direct dependency on Odigos-Collector, allowing users to configure custom endpoints through environment variables.

<p align="center">
<img src="assets/logo.png" width="350" /></br>
<h2>Generate distributed traces for any application in k8s without code changes.</h2>
</p>

<h2 align="center">
    <a href="https://www.youtube.com/watch?v=nynyV7FC4VI">Demo Video</a> • <a href="https://docs.odigos.io">Documentation</a> • <a href="https://join.slack.com/t/odigos/shared_invite/zt-1d7egaz29-Rwv2T8kyzc3mWP8qKobz~A">Join Slack Community</a>
</h2>


### ✨ Language Agnostic Auto-instrumentation

Odigos supports any application written in Java, Python, .NET, Node.js, and **Go**.
Historically, compiled languages like Go have been difficult to instrument without code changes. Odigos solves this problem by uniquely leveraging [eBPF](https://ebpf.io).

![Works on any application](assets/choose_apps.png)


### 🤝 Keep your existing observability tools
Odigos currently supports all the popular managed and open-source destinations.
By producing data in the [OpenTelemetry](https://opentelemetry.io) format, Odigos can be used with any observability tool that supports OTLP.

For a complete list of supported destinations, see [here](#supported-destinations).

![Works with any observability tool](assets/choose_dest.png)

### 🎛️ Collectors Management
Odigos automatically scales OpenTelemetry collectors based on observability data volume.
Manage and configure collectors via a convenient web UI.

![Collectors Management](assets/overview_page.png)

## Installation

Installing Odigos takes less than 5 minutes and requires no code changes.
Download our [CLI](https://docs.odigos.io/installation) and run the following command:


```bash
odigos install
```

For more details, see our [quickstart guide](https://docs.odigos.io/intro).

## Supported Destinations

**For step-by-step instructions detailed for every destination, see these [docs](https://docs.odigos.io/backends).**

### Managed

|                         | Traces  | Metrics | Logs |
|-------------------------| ------- | ------- |------|
| New Relic               | ✅      | ✅      | ✅    |
| Datadog                 | ✅      | ✅      | ✅    |
| Grafana Cloud           | ✅      | ✅      | ✅    |
| Honeycomb               | ✅      | ✅      | ✅    |
| Chronosphere            | ✅      | ✅      |       |
| Logz.io                 | ✅      | ✅      | ✅    |
| qryn.cloud              | ✅      | ✅      | ✅    |
| OpsVerse                | ✅      | ✅      | ✅    |
| Dynatrace               | ✅      | ✅      | ✅    |
| AWS S3                  | ✅      | ✅      | ✅    |
| Google Cloud Monitoring | ✅      |         | ✅    |
| Google Cloud Storage    | ✅      |         | ✅    |
| Azure Blob Storage      | ✅      |         | ✅    |
| Splunk                  | ✅      |         |      |
| Lightstep               | ✅      |         |      |
| Sentry                  | ✅      |         |      |
| Axiom                   | ✅      |         | ✅   |
| Sumo Logic              | ✅      | ✅      | ✅   |
| Coralogix               | ✅      | ✅      | ✅   |

### Open Source

|               | Traces | Metrics | Logs |
| ------------- | ------ | ------- | ---- |
| Prometheus    |        | ✅      |      |
| Tempo         | ✅     |         |      |
| Loki          |        |         | ✅   |
| Jaeger        | ✅     |         |      |
| SigNoz        | ✅     | ✅      | ✅   |
| qryn          | ✅     | ✅      | ✅   |
| Elasticsearch | ✅     |         | ✅   |
| Quickwit      | ✅     |         | ✅   |

Can't find the destination you need? Help us by following our quick [add new destination](https://docs.odigos.io/adding-new-dest) guide and submitting a PR.

## Contributing

Please refer to the [CONTRIBUTING.md](CONTRIBUTING.md) file for information about how to get involved. We welcome issues, questions, and pull requests. Feel free to join our active [Slack Community](https://join.slack.com/t/odigos/shared_invite/zt-1d7egaz29-Rwv2T8kyzc3mWP8qKobz~A).

## All Thanks To Our Contributors

<a href="https://github.com/odigos-io/odigos/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=keyval-dev/odigos" />
</a>

## License

This project is licensed under the terms of the Apache 2.0 open-source license. Please refer to [LICENSE](LICENSE) for the full terms.
