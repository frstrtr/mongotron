#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Deployment configuration
ENVIRONMENT=${1:-"development"}
DEPLOYMENT_TYPE=${2:-"docker"}

echo -e "${GREEN}Deploying MongoTron to ${ENVIRONMENT} using ${DEPLOYMENT_TYPE}...${NC}"

case ${DEPLOYMENT_TYPE} in
  docker)
    echo -e "${YELLOW}Deploying with Docker Compose...${NC}"
    if [ "${ENVIRONMENT}" == "production" ]; then
      docker-compose -f deployments/docker/docker-compose.prod.yml up -d
    else
      docker-compose -f deployments/docker/docker-compose.yml up -d
    fi
    echo -e "${GREEN}Docker deployment complete!${NC}"
    ;;
  
  kubernetes)
    echo -e "${YELLOW}Deploying to Kubernetes...${NC}"
    
    # Apply namespace
    kubectl apply -f deployments/kubernetes/namespace.yml
    
    # Apply ConfigMap and Secrets
    kubectl apply -f deployments/kubernetes/configmap.yml
    
    # Apply Deployment
    kubectl apply -f deployments/kubernetes/deployment.yml
    
    # Apply Service
    kubectl apply -f deployments/kubernetes/service.yml
    
    # Apply Ingress
    kubectl apply -f deployments/kubernetes/ingress.yml
    
    # Apply HPA
    kubectl apply -f deployments/kubernetes/hpa.yml
    
    echo -e "${BLUE}Waiting for deployment to be ready...${NC}"
    kubectl wait --for=condition=available --timeout=300s deployment/mongotron -n mongotron
    
    echo -e "${GREEN}Kubernetes deployment complete!${NC}"
    kubectl get pods -n mongotron
    ;;
  
  *)
    echo -e "${RED}Unknown deployment type: ${DEPLOYMENT_TYPE}${NC}"
    echo "Usage: ./deploy.sh [environment] [docker|kubernetes]"
    exit 1
    ;;
esac

echo -e "${GREEN}Deployment completed successfully!${NC}"
