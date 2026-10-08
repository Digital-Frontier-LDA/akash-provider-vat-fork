# Owned builder setup policy candidate

The provider normally sets `AllowPrivilegeEscalation: false` for every tenant
container. That prevents RootlessKit's subordinate-ID setup and therefore blocks
rootless Docker qualification under the current owned CI contract.

This candidate adds the operator-only `--owned-builder-image` setting. Its empty
default preserves the existing policy. A nonempty value must name
`ghcr.io/digital-frontier-lda/df-akash-builder@sha256:<64 lowercase hex digits>`.
Enabling it requires a separately verified, published immutable builder artifact
and operator/security review; a syntactically valid digest is insufficient evidence.
The [builder candidate](https://github.com/Digital-Frontier-LDA/df-akash-runner/pull/15)
is a separate artifact from the GitHub runner image.

The exception matches only owner
`akash14n4rkmz64rn0tey0r5g07l8q5x0fh2h4hu44kt`, service `owned-buildkit`, the exact
operator-pinned image and one replica. It rejects command/argument overrides,
GPU, interconnect or custom runtime requests, mounted payloads, workload API
permissions, image credentials and TEE parameters. The only tenant environment
fields accepted are the three required per-attempt server TLS fields and optional
bounded lifetime; duplicates, empty fields and unknown names reject the exception.
All unmatched shapes receive the ordinary provider security context.

An admitted builder receives outer UID/GID 1000, non-root execution, privileged
mode disabled, all capabilities dropped except SETUID/SETGID, and unconfined
seccomp/AppArmor for rootless namespace/mount operations. Its NoNewPrivs restriction
is disabled at container setup. The immutable image must enforce mutual TLS,
fixed startup, bounded lifetime and NoNewPrivs on the daemon and build steps.
Proc masks are retained. The builder occupies its own service/container; no
GitHub runner, controller credentials or CA/client private key may share it.

## Security Findings

The RootlessKit parent retains NoNewPrivs disabled. No-process-sandbox builds can
affect processes inside their builder container and access its per-attempt server
key. Operator approval of an immutable image is the privileged boundary; SDL
input cannot select an exception for another owner, image or process shape.
The shared owner also serves other production uses, so controller admission,
private workflow groups and per-attempt TLS/receipts remain necessary.

## Checks Performed

Builder package tests exercise the actual generated container security context,
the default-off path, immutable-package validation and hostile owner, image,
service, command, environment, volume, permission and runtime mutations. The
separate builder's hosted actual-container tests verify mutual TLS, a real build
from a separate NoNewPrivs client, lifetime expiry and complete local cleanup.

## Published candidate and application prerequisite

[Builder publication 37696578412](https://github.com/Digital-Frontier-LDA/df-akash-runner/actions/runs/37696578412)
completed after both native AMD64 and ARM64 authentication/build/expiry checks.
Its source is `c52a6ed145b83a275c60607a0854c11f2be18e81` and immutable candidate is
`ghcr.io/digital-frontier-lda/df-akash-builder@sha256:982987d30e40ea7b742b4614d2469800771949683bc75a75bba8dddc764ec3f6`.
The registry package is **private**. This policy rejects `ImageCredentials`, so
publication alone does not make this candidate pullable by the provider. Keep
`--owned-builder-image` unset until a compatible, separately reviewed registry
pull path and live provider qualification pass. No package visibility or provider
values are changed here.

The unchanged AI-Shopper API, web, hah and Caddy Dockerfiles at
`f12bc9c4925b6a555aab95ce310694c2b9a1921f` all built locally through a separate
NoNewPrivs client in 1,638 seconds total, within the original 1,800-second budget.
This prerequisite used AMD64 emulation on an ARM64 host and a pre-publication
builder image from `b992f4e9cf5da4372037aa78610043b81fed67c4`. It used no push,
load, GitHub cache authority or CI credentials; all local containers/network were
removed. It does not establish native provider performance, current published
application-build qualification, or the original workflow cache/publication paths.

## Residual Risk

These tests establish generation and local/container behavior. They do not prove
the live provider runtime accepts namespace setup or that all four application
builds with their original caches and publication controls work on that provider. No provider values are changed
and the exception remains disabled in this candidate.

## Recommendation

Review the bounded setup exception, publish and verify the exact builder digest,
then qualify a receipt-bound builder on the selected owned provider. Before any
Docker workflow routes there, require all four no-push builds with their existing
30-minute limit and independent closure after success, failure and cancellation.
Do not expose publication authority until its subsequent qualification passes.

## Complete pull-request verification

Pull requests run the original build, full tests, recovery-upgrade test, lint,
release dry-run, coverage, YAML policy and CRD integration jobs on isolated
Ubuntu 24.04 hosted runners. Main and tag routes retain their existing runner
labels. Coverage is generated in its own fresh job before the original upload;
it no longer assumes a persistent runner already has another job's output.
The builder policy workflow uses the repository-required `.yaml` suffix, so the
unchanged YAML-extension check includes it without an exception. All original
commands and result gates remain; provider integration must pass before merge.
