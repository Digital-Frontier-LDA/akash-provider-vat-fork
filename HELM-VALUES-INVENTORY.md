# Helm-Values Inventory — forked provider deployment

This forked provider (`akash-provider-vat-fork`) needs a small set of additions
to its Helm-values to deploy alongside the Digital Frontier VAT-evidence sidecar.
The deployment manifests themselves may live in `akash-providers-IaC` (see
`vat-evidence-pipeline/CLAUDE.md`). **This file is the canonical inventory of
what those Helm-values changes are** — the actual values land when the cluster
is deployed.

The forked provider deploys into the **`akash-services` namespace** — the sidecar
co-locates with the provider it serves (DEC-04).

There are **NO logic changes to the provider chart** — only environment
variables, one shared volume, and one mounted config. This preserves the
fork-tiny discipline (`CLAUDE.md` guardrail #1).

## Environment variables (provider Pod must export)

Read by `df-telemetry/config.go` (MP-02 — multi-provider awareness):

| Env var | Purpose |
|---------|---------|
| `DF_PROVIDER_ID` | Operator-configured provider identifier. Carried on every emitted event. |
| `DF_PROVIDER_WALLET` | Provider's on-chain wallet address. Carried on every emitted event. |
| `DF_PROVIDER_REGION` | Provider region label. |
| `DF_PROVIDER_SITE` | Provider site label. |
| `DF_TELEMETRY_SOCKET` | UDS path the emitter writes to. Default `/var/run/digitalfrontier/vat-telemetry.sock`. |
| `DF_SOURCE_IP_POLICY_PATH` | Path to the mounted `source_ip_policy.yaml` (DEC-06). |

## Volume (shared UDS socket)

An `emptyDir` (or `hostPath`) volume shared between the provider container and
the VAT-evidence sidecar container, mounted at `/var/run/digitalfrontier/` in
**both** containers. The forked provider writes the NDJSON telemetry stream to
the Unix domain socket at `DF_TELEMETRY_SOCKET`; the sidecar reads from it.

## ConfigMap / Secret (source_ip_policy)

A ConfigMap carrying `source_ip_policy.yaml` — validated against
`vat-evidence-pipeline/schemas/source_ip_policy.schema.json` (DEC-06) — mounted
into the provider container at `DF_SOURCE_IP_POLICY_PATH`. This drives how the
hook resolves the client source IP (PROXY protocol / trusted XFF / remote_addr).

## Summary

The forked provider's Helm-values gain: 6 environment variables, 1 shared
`emptyDir` volume mounted in two containers, and 1 mounted `source_ip_policy`
config. Namespace: `akash-services`. No provider-chart logic is altered.
