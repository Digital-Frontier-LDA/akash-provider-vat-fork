# Enforce the builder lifetime before granting setup privileges

The existing default-off operator contract accepts any nonempty
`BUILDER_TTL_SECONDS`, although the immutable builder entrypoint accepts only
one to four ASCII decimal digits representing 60–3600 seconds. Reject an invalid
lifetime before granting the namespace setup exception or its dedicated pull
secret. Omission retains the image's 3600-second default. Leading zeroes are
accepted only within the image's four-character limit; signs, whitespace,
non-ASCII digits, overflow and duplicate fields remain inadmissible.

The operator-pinned image, fixed lease owner and `owned-buildkit` service stay
intact. CRD decoding always materializes a GPU pointer, including zero-unit CPU
services. Treat only a zero-unit GPU with no attributes as absent; reject positive
GPU units, GPU attributes and scheduler GPU/interconnect assignments. Verify this
through the actual CRD round trip, rather than relying on the hand-built fixture.
An unmatched contract retains
the ordinary security context and pull behavior. This change neither enables
operator settings nor provisions credentials, modifies a live namespace or
qualifies a native application build.

Implement in `cluster/kube/builder/owned_builder.go`, following existing early
returns. Table tests in `owned_builder_test.go` exercise accepted boundary values
and hostile inputs through actual container and pull-secret generation. Full
Deployment and StatefulSet create/update tests must verify the non-root container,
limited capabilities, disabled service-account mount, isolated pod defaults and
absence of mounted payloads; a rejected lifetime must restore NoNewPrivs and
withhold the dedicated pull secret in both paths.

Run `gofmt`, `git diff --check` and
`go test -count=1 ./cluster/kube/builder ./cmd/provider-services/cmd`. Existing
hosted provider CI remains required before merge. Published-image verification
39257916461 tested source c52a6ed145b83a275c60607a0854c11f2be18e81 and digest
982987d30e40ea7b742b4614d2469800771949683bc75a75bba8dddc764ec3f6 on AMD64 and
ARM64; it is supporting entrypoint evidence, not live provider qualification.

The security boundary is trusted operator configuration plus authenticated lease
ownership versus tenant environment input. No environment value is logged.
The RootlessKit parent still has NoNewPrivs disabled for setup; the immutable
image must enforce mutual TLS, daemon/build NoNewPrivs and expiry. Provider runtime,
read-only registry credential delivery and receipt-bound native cleanup remain
separate qualification prerequisites.
