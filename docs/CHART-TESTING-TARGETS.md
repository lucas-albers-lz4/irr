# Helm Chart Testing Targets

This document outlines the Helm charts we'll use for systematic testing of the helm-image-override tool. Charts are organized by category and complexity.

## Initial Testing Set (Top 10 Priority)

1. **Nginx-Ingress**
   - Category: Infrastructure
   - Complexity: Medium
   - Key Features: Multiple container types, init containers
   - URL: https://github.com/kubernetes/ingress-nginx/tree/main/charts/ingress-nginx

2. **Cert-Manager**
   - Category: Security/Infrastructure
   - Complexity: Medium
   - Key Features: CRDs, webhook containers
   - URL: https://github.com/cert-manager/cert-manager/tree/master/deploy/charts/cert-manager

3. **Prometheus**
   - Category: Monitoring
   - Complexity: High
   - Key Features: Multiple components, extensive configuration
   - URL: https://github.com/prometheus-community/helm-charts/tree/main/charts/prometheus

4. **Grafana**
   - Category: Monitoring/Visualization
   - Complexity: Medium
   - Key Features: Plugins, datasource configurations
   - URL: https://github.com/grafana/helm-charts/tree/main/charts/grafana

5. **Redis**
   - Category: Database
   - Complexity: Low-Medium
   - Key Features: Clustering, metrics exporter
   - URL: https://github.com/bitnami/charts/tree/main/bitnami/redis

6. **MySQL**
   - Category: Database
   - Complexity: Medium
   - Key Features: Primary-replica setup, backup containers
   - URL: https://github.com/bitnami/charts/tree/main/bitnami/mysql

7. **Argo CD**
   - Category: CI/CD
   - Complexity: High
   - Key Features: Multiple services, RBAC, Redis dependency
   - URL: https://github.com/argoproj/argo-helm/tree/main/charts/argo-cd

8. **Istio**
   - Category: Service Mesh
   - Complexity: Very High
   - Key Features: Multiple charts, complex dependencies
   - URL: https://github.com/istio/istio/tree/master/manifests/charts

9. **Harbor**
   - Category: Registry
   - Complexity: High
   - Key Features: Multiple components, database dependencies
   - URL: https://github.com/goharbor/harbor-helm

10. **Kube-Prometheus-Stack**
    - Category: Monitoring
    - Complexity: Very High
    - Key Features: Multiple charts, extensive CRDs
    - URL: https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack

## Extended Testing Categories

### Infrastructure & Networking
- Traefik
- External-DNS
- Consul
- etcd

### Monitoring & Observability
- Loki
- Fluentd
- Elasticsearch
- Kibana
- kube-state-metrics

### Databases
- PostgreSQL
- MongoDB
- Cassandra
- MinIO

### CI/CD & GitOps
- Jenkins
- Flux
- GitLab

### Applications & Services
- WordPress
- Drupal
- Kafka
- RabbitMQ
- Keycloak
- Airflow
- Jupyterhub

### Security
- Anchore
- Falco
- OPA
- Vault
- Gatekeeper

### Platform Services
- Knative
- Spark
- Zookeeper
- ChartMuseum
- Helmfile

## Complexity Levels Defined

- **Low**: Single container, minimal configuration, no dependencies
- **Medium**: Multiple containers, some configuration options, simple dependencies
- **High**: Multiple components, extensive configuration, multiple dependencies
- **Very High**: Multiple charts, complex dependencies, CRDs, extensive customization

## Testing Priority Strategy

1. **Phase 1**: Test simple charts first (Redis, MySQL) to validate basic functionality
2. **Phase 2**: Move to medium complexity (Nginx-Ingress, Cert-Manager)
3. **Phase 3**: Test high complexity charts (Prometheus, Argo CD)
4. **Phase 4**: Test very high complexity charts (Istio, Kube-Prometheus-Stack)

## Chart Properties to Test

1. **Image Reference Patterns**
   - Standard repository/tag format
   - Digest-based references
   - Global registry settings
   - Custom image pull policies

2. **Dependencies**
   - Number of subchart levels
   - Conditional dependencies
   - Global value overrides

3. **Configuration Complexity**
   - Value structure depth
   - Array-based configurations
   - Dynamic template usage

4. **Special Cases**
   - Init containers
   - Sidecar injection
   - Custom resource definitions
   - Image pull secrets

## Success Criteria

For each chart:
1. All image references correctly identified
2. Proper handling of subchart dependencies
3. Generated overrides maintain chart functionality
4. No unintended modifications to non-image values 