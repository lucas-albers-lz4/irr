# Integration Test Remediation Plan

Based on analysis of project documentation (`docs/`), integration test code (`test/integration/`), and `make test` output, this plan outlines the steps to fix the failing Go integration tests for the `irr` tool.

## 1. Prerequisites - Test Chart Setup

*   [ ] Verify/Create the `unsupported-test` chart in `test-data/charts/` based on the structure needed by `setupChartWithUnsupportedStructure` in `integration_test.go`. This chart should contain an image defined with non-standard keys (e.g., `version` instead of `tag`) to test `--strict` mode.
*   [ ] Verify/Create the `helmignore-test` chart needed for `TestHelmIgnoreFileProcessing`. This chart should contain a `.helmignore` file and image references that might be affected by ignored files/templates.
*   [ ] Ensure all other required charts (`minimal-test`, `parent-test`, `kube-prometheus-stack`, `cert-manager`, `ingress-nginx`) exist and are complete in `test-data/charts/`.

## 2. Address `helm template` Validation Failures

*   [ ] **Bitnami (`ingress-nginx`):** Modify the `TestHarness.ValidateOverrides` function (or the specific test cases like `TestIngressNginxIntegration` and the relevant `TestComplexChartFeatures` subtest) to inject `global.security.allowInsecureImages=true` when running the *validation* `helm template` command *with* the generated overrides.
    *   The command during validation should look similar to: `helm template <release> <chart> -f <generated-overrides.yaml> --set global.security.allowInsecureImages=true`.
*   [ ] **Other `helm template` failures:** Investigate other `ValidateOverrides` failures by examining the generated `overrides.yaml` and the Helm error messages. Fix any incorrect override structures that break Helm templating.

## 3. Fix Override Generation Logic (`kube-prometheus-stack`)

*   [ ] Debug why images (`prometheus`, `alertmanager`, `prometheus-operator`, `node-exporter`, `kube-state-metrics`, `grafana`) are missed or incorrectly processed in `TestComplexChartFeatures/kube-prometheus-stack_with_all_components`.
*   [ ] Analyze the `kube-prometheus-stack` chart's `values.yaml` structure carefully, looking for potentially complex or unusual image definitions.
*   [ ] Step through the value traversal and image identification logic in `pkg/chart` and `pkg/override` for this specific chart to identify and fix the root cause. Ensure the `prefix-source-registry` strategy is applied correctly to all identified images.

## 4. Fix Flag/Mode Tests

*   [ ] **`TestDryRunFlag`:**
    *   Ensure `make build` is run before tests executing the binary.
    *   Debug the `../../bin/irr override ... --dry-run` command execution. Determine why it exits with status 4 instead of 0.
    *   Verify argument parsing, execution flow, and side effects (no file created, expected stdout content).
*   [ ] **`TestStrictMode`:**
    *   Ensure `make build` is run.
    *   Debug the `../../bin/irr override ... --strict` execution against the `unsupported-test` chart.
    *   Fix the parsing logic (`pkg/chart/generator.go`, `pkg/override/override.go`) so that encountering the unsupported structure defined in `setupChartWithUnsupportedStructure` triggers an error (non-zero exit code) when `--strict` is active. The test assertion `assert.Error(t, err)` should then pass.

## 5. Address Remaining Test Failures

Address the following tests systematically, ensuring the `irr` tool (and potentially the binary execution tests) handle these scenarios correctly:

*   [ ] **`TestRegistryMappingFile`:** Review test setup and core logic for loading/applying registry mappings.
*   [ ] **`TestMissingValuesFile`:** Verify expected error handling when `values.yaml` is missing.
*   [ ] **`TestInvalidTargetRegistry`:** Check input validation logic for the `--target-registry` flag format.
*   [ ] **`TestNoSourceRegistries`:** Confirm correct behavior when `--source-registries` is empty or omitted.
*   [ ] **`TestOutputFileFlag`:** Debug the test using `--output-file`, ensuring file I/O works and the binary executes correctly.

## 6. Final Review & Cleanup

*   [ ] Run `make test` again to confirm all integration tests pass.
*   [ ] Review changes for clarity, efficiency, and adherence to project standards.
*   [ ] Update this document (`INTEGRATION.md`) with progress and any relevant findings. 