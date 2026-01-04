#!/bin/bash
# Consolidated script: Start services → Run traffic → Shutdown
# Manages complete lifecycle: MySQL, OTEL Collector, E-commerce App, Traffic Containers

set -euo pipefail

# Configuration
DURATION=${DURATION:-900}  # 15 minutes default (900 seconds)
STARTUP_DELAY=${STARTUP_DELAY:-5}  # Delay before traffic starts (seconds)
HEALTH_CHECK_TIMEOUT=${HEALTH_CHECK_TIMEOUT:-60}  # Max time to wait for health (seconds)
HEALTH_CHECK_INTERVAL=${HEALTH_CHECK_INTERVAL:-2}  # Interval between health checks (seconds)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
# Move to docker directory to run docker-compose
cd ../docker

# State tracking
TRAFFIC_STARTED=false
SERVICES_STARTED=false

# Cleanup function for graceful shutdown
cleanup() {
    echo ""
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}Shutting down services...${NC}"
    echo -e "${YELLOW}========================================${NC}"
    
    # Stop traffic containers first
    if [[ "$TRAFFIC_STARTED" == "true" ]]; then
        echo -e "${BLUE}Stopping traffic containers...${NC}"
        docker-compose stop traffic-user-1001 traffic-user-1002 traffic-user-1003 traffic-user-1004 2>/dev/null || true
        echo -e "${GREEN}✓ Traffic containers stopped${NC}"
    fi
    
    # Stop core services
    if [[ "$SERVICES_STARTED" == "true" ]]; then
        echo -e "${BLUE}Stopping core services...${NC}"
        docker-compose stop ecommerce-app otel-collector mysql 2>/dev/null || true
        echo -e "${GREEN}✓ Core services stopped${NC}"
    fi
    
    echo -e "${GREEN}Shutdown complete${NC}"
    exit 0
}

# Set trap for Ctrl+C and script exit
trap cleanup SIGINT SIGTERM EXIT

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Check if docker-compose is available
check_docker_compose() {
    if command -v docker-compose &> /dev/null; then
        DOCKER_COMPOSE_CMD="docker-compose"
    elif docker compose version &> /dev/null 2>&1; then
        DOCKER_COMPOSE_CMD="docker compose"
    else
        log_error "docker-compose or 'docker compose' is required but not installed"
        exit 1
    fi
}

# Check MySQL health
check_mysql_healthy() {
    local max_attempts=$((HEALTH_CHECK_TIMEOUT / HEALTH_CHECK_INTERVAL))
    local attempt=1
    
    log_info "Waiting for MySQL to be healthy..."
    
    while [[ $attempt -le $max_attempts ]]; do
        # Check if MySQL container is healthy using docker-compose ps
        if $DOCKER_COMPOSE_CMD ps mysql 2>/dev/null | grep -q "healthy"; then
            log_success "MySQL is healthy"
            return 0
        fi
        
        # Also try direct mysqladmin ping as fallback
        if docker exec ecommerce-mysql mysqladmin ping -h localhost -u root -ppassword --silent 2>/dev/null; then
            log_success "MySQL is healthy (via mysqladmin)"
            return 0
        fi
        
        echo -n "."
        sleep "$HEALTH_CHECK_INTERVAL"
        ((attempt++))
    done
    
    echo ""
    log_error "MySQL failed to become healthy after ${HEALTH_CHECK_TIMEOUT} seconds"
    return 1
}

# Check E-commerce App health
check_app_healthy() {
    local max_attempts=$((HEALTH_CHECK_TIMEOUT / HEALTH_CHECK_INTERVAL))
    local attempt=1
    
    log_info "Waiting for E-commerce App to be healthy..."
    
    while [[ $attempt -le $max_attempts ]]; do
        if curl -s -f --max-time 5 --connect-timeout 3 "http://localhost:8080/health" > /dev/null 2>&1; then
            log_success "E-commerce App is healthy"
            return 0
        fi
        
        echo -n "."
        sleep "$HEALTH_CHECK_INTERVAL"
        ((attempt++))
    done
    
    echo ""
    log_error "E-commerce App failed to become healthy after ${HEALTH_CHECK_TIMEOUT} seconds"
    return 1
}

# Check OTEL Collector ready
check_collector_ready() {
    local max_attempts=15  # Reduced to 30 seconds max (15 * 2s)
    local attempt=1
    
    log_info "Waiting for OTEL Collector to be ready..."
    
    while [[ $attempt -le $max_attempts ]]; do
        # Check if collector container is running
        if $DOCKER_COMPOSE_CMD ps otel-collector 2>/dev/null | grep -q "Up"; then
            log_success "OTEL Collector container is running"
            return 0
        fi
        
        echo -n "."
        sleep "$HEALTH_CHECK_INTERVAL"
        ((attempt++))
    done
    
    echo ""
    log_warn "OTEL Collector container may not be fully started, but continuing..."
    return 0  # Don't fail if collector isn't ready, as it's not critical for basic operation
}

# Start core services
start_core_services() {
    log_info "Starting core services (MySQL, OTEL Collector, E-commerce App)..."
    
    # Stop any existing containers first
    log_info "Stopping any existing containers..."
    
    # Force remove specific containers to avoid conflicts
    docker rm -f ecommerce-mysql ecommerce-app ecommerce-otel-collector traffic-user-1001 traffic-user-1002 traffic-user-1003 traffic-user-1004 2>/dev/null || true
    
    $DOCKER_COMPOSE_CMD down 2>/dev/null || true

    # Ensure network exists
    docker network create signoz-net 2>/dev/null || true
    
    # Start MySQL first
    log_info "Starting MySQL..."
    $DOCKER_COMPOSE_CMD up -d mysql
    
    # Wait for MySQL to be healthy
    if ! check_mysql_healthy; then
        log_error "Failed to start MySQL"
        $DOCKER_COMPOSE_CMD logs mysql --tail 30
        exit 1
    fi
    
    # Start OTEL Collector
    log_info "Starting OTEL Collector..."
    $DOCKER_COMPOSE_CMD up -d otel-collector
    
    # Wait for OTEL Collector to be ready
    check_collector_ready
    
    # Start E-commerce App
    log_info "Starting E-commerce App..."
    $DOCKER_COMPOSE_CMD up -d --build ecommerce-app
    
    # Wait for E-commerce App to be healthy
    if ! check_app_healthy; then
        log_error "Failed to start E-commerce App"
        $DOCKER_COMPOSE_CMD logs ecommerce-app --tail 30
        exit 1
    fi
    
    SERVICES_STARTED=true
    log_success "All core services are running"
}

# Start traffic containers
start_traffic_containers() {
    log_info "Starting traffic generator containers..."
    
    # Start all 4 traffic containers
    $DOCKER_COMPOSE_CMD up -d --build traffic-user-1001 traffic-user-1002 traffic-user-1003 traffic-user-1004
    
    # Wait a moment for containers to start
    sleep 2
    
    # Verify containers are running
    local running_count=0
    for container in traffic-user-1001 traffic-user-1002 traffic-user-1003 traffic-user-1004; do
        if $DOCKER_COMPOSE_CMD ps "$container" 2>/dev/null | grep -q "Up"; then
            ((running_count++))
        fi
    done
    
    if [[ $running_count -eq 4 ]]; then
        log_success "All 4 traffic containers are running"
        TRAFFIC_STARTED=true
    else
        log_warn "Only $running_count/4 traffic containers are running"
        TRAFFIC_STARTED=true  # Still mark as started to allow cleanup
    fi
}

# Wait for specified duration
wait_for_duration() {
    local duration=$1
    local elapsed=0
    local interval=10  # Update every 10 seconds
    
    log_info "Running traffic generation for ${duration} seconds..."
    log_info "Press Ctrl+C to stop early"
    
    while [[ $elapsed -lt $duration ]]; do
        sleep "$interval"
        elapsed=$((elapsed + interval))
        local remaining=$((duration - elapsed))
        
        if [[ $remaining -gt 0 ]]; then
            local minutes=$((remaining / 60))
            local seconds=$((remaining % 60))
            echo -e "${CYAN}[${elapsed}s/${duration}s]${NC} Remaining: ${minutes}m ${seconds}s"
        fi
    done
    
    log_success "Traffic generation completed (${duration} seconds)"
}

# Shutdown traffic containers
shutdown_traffic() {
    log_info "Stopping traffic containers..."
    $DOCKER_COMPOSE_CMD stop traffic-user-1001 traffic-user-1002 traffic-user-1003 traffic-user-1004 2>/dev/null || true
    log_success "Traffic containers stopped"
}

# Shutdown core services
shutdown_services() {
    log_info "Stopping core services..."
    $DOCKER_COMPOSE_CMD stop ecommerce-app otel-collector mysql 2>/dev/null || true
    log_success "Core services stopped"
}

# Main function
main() {
    echo -e "${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║  Full Session Script - E-commerce Application              ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    
    echo -e "${BLUE}Configuration:${NC}"
    echo "  Duration:        ${DURATION} seconds ($(($DURATION / 60)) minutes)"
    echo "  Startup Delay:   ${STARTUP_DELAY} seconds"
    echo "  Health Timeout:  ${HEALTH_CHECK_TIMEOUT} seconds"
    echo ""
    
    # Check dependencies
    check_docker_compose
    
    if ! command -v curl &> /dev/null; then
        log_error "curl is required but not installed"
        exit 1
    fi
    
    # Step 1: Start core services
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}Step 1: Starting Core Services${NC}"
    echo -e "${YELLOW}========================================${NC}"
    start_core_services
    
    # Step 2: Wait for additional delay
    echo ""
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}Step 2: Waiting ${STARTUP_DELAY} seconds before starting traffic${NC}"
    echo -e "${YELLOW}========================================${NC}"
    sleep "$STARTUP_DELAY"
    
    # Step 3: Start traffic containers
    echo ""
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}Step 3: Starting Traffic Containers${NC}"
    echo -e "${YELLOW}========================================${NC}"
    start_traffic_containers
    
    # Step 4: Run for specified duration
    echo ""
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}Step 4: Running Traffic Generation${NC}"
    echo -e "${YELLOW}========================================${NC}"
    wait_for_duration "$DURATION"
    
    # Step 5: Shutdown traffic containers
    echo ""
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}Step 5: Shutting Down Traffic Containers${NC}"
    echo -e "${YELLOW}========================================${NC}"
    shutdown_traffic
    
    # Step 6: Shutdown core services
    echo ""
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}Step 6: Shutting Down Core Services${NC}"
    echo -e "${YELLOW}========================================${NC}"
    shutdown_services
    
    # Disable trap since we're shutting down normally
    trap - SIGINT SIGTERM EXIT
    
    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║  Session Complete!                                           ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
}

# Run main function
main "$@"
