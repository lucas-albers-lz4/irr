# Understanding the Use of `default-values.yaml` in Testing

This document explains why our `test-charts.py` script utilizes a custom `default-values.yaml` file when testing Helm charts, even though charts typically include their own default values.

## Helm Chart Defaults: The Standard `values.yaml`

*   **Built-in Defaults:** Nearly every Helm chart comes packaged with a `values.yaml` file. This file contains the chart maintainer's default settings for configuration options like image tags, replica counts, service types, resource limits, etc.
*   **Standard Usage:** When you execute `helm install my-chart` or `helm template my-chart` without specifying any overrides (`-f` or `--set`), Helm relies entirely on the defaults defined within that chart's internal `values.yaml`.

## Why We Override with `-f default-values.yaml`

The primary purpose of using command-line flags like `-f <your-values-file>` or `--set key=value` is to **override** the chart's built-in defaults. This allows users to customize a chart for their specific environment or requirements.

Our `test-charts.py` script uses `-f default-values.yaml` for several specific, crucial reasons related to our testing goals:

1.  **Enforcing the Image Mirror (`harbor.home.arpa/docker`)**
    *   **Critical Goal:** We need *all* charts tested by the script to attempt pulling images from our local Harbor mirror (`harbor.home.arpa/docker`) instead of public registries (like Docker Hub).
    *   **Benefit:** This avoids public registry rate limits and ensures tests run against potentially cached or internally approved images.
    *   **Challenge:** Different charts specify their image registry using various keys (e.g., `global.imageRegistry`, `image.registry`, Bitnami's `registry.server` or specific `image.registry` sections).
    *   **Solution:** Our `default-values.yaml` attempts to set these common registry keys to `harbor.home.arpa/docker`. Passing this file via `-f` forces the Helm template rendering process to *try* using our mirror, irrespective of the chart's original default registry.

2.  **Handling Bitnami Image Verification (`allowInsecureImages`)**
    *   **Context:** Bitnami charts include a security check (`allowInsecureImages`). This check typically fails when we force the chart to use images from our mirror (e.g., `harbor.home.arpa/docker/bitnami/...`) instead of their official `docker.io/bitnami/...` images.
    *   **Requirement:** The default for this check is `false`. To proceed with our mirrored images, we *must* explicitly override this setting.
    *   **Solution:** Our `default-values.yaml` sets `global.security.allowInsecureImages: true`. We cannot rely on the chart's default value here; an explicit override is necessary for the tests involving Bitnami charts using our mirror.

3.  **Ensuring Successful Templating (`helm template`)**
    *   **Problem:** Some charts might fail the `helm template` command entirely if certain basic required values (like a mandatory password or specific storage configurations) are missing.
    *   **Goal:** The immediate goal for `test-charts.py` is just to successfully *render* the chart's templates so we can analyze and override the images within them. The resulting manifests don't need to be fully deployable at this stage.
    *   **Solution:** Our `default-values.yaml` provides minimal, generally safe defaults (e.g., `storageClass: ""`) to increase the likelihood that `helm template` completes successfully, even if further customization would be needed for actual deployment.

4.  **Consistency**
    *   **Benefit:** Using a single `default-values.yaml` provides a consistent baseline configuration applied to *all* charts processed by the test script.

## Summary

We use our custom `default-values.yaml` not because Helm charts lack their own defaults, but because **we need to impose specific, non-standard configurations consistently across diverse charts for our testing purposes.** The primary drivers are:

*   Forcing the use of our internal image mirror (`harbor.home.arpa/docker`).
*   Bypassing Bitnami's specific image security checks when using mirrored images.
*   Maximizing the chance of successful template rendering for image analysis.

These universal requirements for our test suite cannot be reliably met by depending on the individual default `values.yaml` file within each chart.