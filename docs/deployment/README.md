# MongoTron Deployment Guide

## Prerequisites

- Docker 20.10+
- Docker Compose 2.0+ (for Docker deployment)
- Kubernetes 1.24+ (for K8s deployment)
- MongoDB 6.0+
- Access to a Tron full node

## Docker Deployment

### Development
```bash
cd deployments/docker
docker-compose up -d
```

### Production
```bash
cd deployments/docker
docker-compose -f docker-compose.prod.yml up -d
```

## Kubernetes Deployment

### Step 1: Create Secrets
```bash
kubectl create secret generic mongotron-secrets \
  --from-literal=mongodb-uri='mongodb://admin:password@mongodb:27017/mongotron' \
  -n mongotron
```

### Step 2: Apply Manifests
```bash
kubectl apply -f deployments/kubernetes/namespace.yml
kubectl apply -f deployments/kubernetes/configmap.yml
kubectl apply -f deployments/kubernetes/deployment.yml
kubectl apply -f deployments/kubernetes/service.yml
kubectl apply -f deployments/kubernetes/ingress.yml
kubectl apply -f deployments/kubernetes/hpa.yml
```

### Step 3: Verify Deployment
```bash
kubectl get pods -n mongotron
kubectl get svc -n mongotron
```

## Configuration

Copy the example configuration:
```bash
cp configs/.env.example configs/.env
```

Edit the configuration file with your settings.

## Monitoring

Access Grafana at:
```
http://localhost:3000
```

Default credentials:
- Username: admin
- Password: admin123

## Troubleshooting

### Check logs
```bash
# Docker
docker logs mongotron

# Kubernetes
kubectl logs -f deployment/mongotron -n mongotron
```

### Check health
```bash
curl http://localhost:8080/health
```
