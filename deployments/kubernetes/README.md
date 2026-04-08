# APICerebrus Kubernetes Deployment

This directory contains Kubernetes manifests for deploying APICerebrus.

## Structure

```
kubernetes/
├── base/                    # Base Kubernetes resources
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── secret.yaml
│   ├── serviceaccount.yaml
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── hpa.yaml
│   ├── pdb.yaml
│   ├── networkpolicy.yaml
│   ├── ingress.yaml
│   ├── servicemonitor.yaml
│   └── kustomization.yaml
└── overlays/
    ├── development/         # Development environment
    │   └── kustomization.yaml
    └── production/          # Production environment
        ├── kustomization.yaml
        └── pvc.yaml
```

## Quick Start

### Prerequisites

- Kubernetes cluster (1.24+)
- kubectl configured
- kustomize (optional, kubectl has built-in support)

### Deploy to Development

```bash
# Using the deployment script
make deploy-k8s-dev

# Or manually with kubectl
kubectl apply -k deployments/kubernetes/overlays/development

# Or with kustomize
kustomize build deployments/kubernetes/overlays/development | kubectl apply -f -
```

### Deploy to Production

```bash
# Using the deployment script
make deploy-k8s-prod

# Or manually
kubectl apply -k deployments/kubernetes/overlays/production
```

## Configuration

### Secrets

Before deploying, create the required secrets:

```bash
# Create namespace
kubectl create namespace apicerberus-production

# Create secrets
kubectl create secret generic apicerberus-secrets \
  --from-literal=jwt-secret=$(openssl rand -base64 32) \
  --from-literal=admin-api-key=$(openssl rand -base64 32) \
  -n apicerberus-production
```

### Customizing Configuration

Edit the ConfigMap in the overlay:

```yaml
# deployments/kubernetes/overlays/production/kustomization.yaml
patches:
  - target:
      kind: ConfigMap
      name: apicerberus-config
    patch: |-
      - op: replace
        path: /data/apicerberus.yaml
        value: |
          server:
            address: "0.0.0.0:8080"
          # ... your config
```

## Monitoring

The deployment includes Prometheus ServiceMonitor for metrics collection. Ensure you have:

1. Prometheus Operator installed
2. ServiceMonitor CRD available

## High Availability

For production deployments:

- Minimum 3 replicas
- PodDisruptionBudget ensures minimum availability
- Anti-affinity rules spread pods across nodes
- HPA for automatic scaling

## Troubleshooting

```bash
# Check pod status
kubectl get pods -n apicerberus-production

# View logs
kubectl logs -n apicerberus-production -l app.kubernetes.io/name=apicerberus

# Port forward for local access
kubectl port-forward -n apicerberus-production svc/apicerberus 8080:8080

# Check events
kubectl get events -n apicerberus-production --sort-by='.lastTimestamp'
```
