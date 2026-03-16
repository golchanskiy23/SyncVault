#!/bin/bash

# SyncVault Development Environment Script

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo -e "${BLUE}================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}================================${NC}"
}

# Function to check if service is ready
wait_for_service() {
    local service=$1
    local port=$2
    local host=${3:-localhost}
    
    print_status "Waiting for $service to be ready..."
    
    while ! nc -z $host $port; do
        sleep 1
    done
    
    print_status "$service is ready!"
}

# Function to start dependencies
start_dependencies() {
    print_header "Starting Dependencies"
    
    # Start docker containers
    docker-compose up -d postgres redis mongodb
    
    # Wait for services to be ready
    wait_for_service "PostgreSQL" 5432
    wait_for_service "Redis" 6379
    wait_for_service "MongoDB" 27017
    
    print_status "All dependencies are ready!"
}

# Function to start microservices
start_microservices() {
    print_header "Starting Microservices"
    
    # Create logs directory
    mkdir -p logs
    
    # Start each service in background
    print_status "Starting Auth Service (port 50056)..."
    go run cmd/auth-service/main.go > logs/auth-service.log 2>&1 &
    AUTH_PID=$!
    
    sleep 2
    
    print_status "Starting Storage Service (port 50054)..."
    go run cmd/storage-service/main.go > logs/storage-service.log 2>&1 &
    STORAGE_PID=$!
    
    sleep 2
    
    print_status "Starting File Service (port 50052)..."
    go run cmd/file-service/main.go > logs/file-service.log 2>&1 &
    FILE_PID=$!
    
    sleep 2
    
    print_status "Starting Sync Service (port 50053)..."
    go run cmd/sync-service/main.go > logs/sync-service.log 2>&1 &
    SYNC_PID=$!
    
    sleep 2
    
    print_status "Starting Notification Service (port 50055)..."
    go run cmd/notification-service/main.go > logs/notification-service.log 2>&1 &
    NOTIFICATION_PID=$!
    
    # Save PIDs for cleanup
    echo $AUTH_PID > logs/auth-service.pid
    echo $STORAGE_PID > logs/storage-service.pid
    echo $FILE_PID > logs/file-service.pid
    echo $SYNC_PID > logs/sync-service.pid
    echo $NOTIFICATION_PID > logs/notification-service.pid
    
    print_status "All microservices started!"
}

# Function to stop services
stop_services() {
    print_header "Stopping Services"
    
    # Stop microservices
    if [ -f logs/auth-service.pid ]; then
        kill $(cat logs/auth-service.pid) 2>/dev/null || true
        rm logs/auth-service.pid
    fi
    
    if [ -f logs/storage-service.pid ]; then
        kill $(cat logs/storage-service.pid) 2>/dev/null || true
        rm logs/storage-service.pid
    fi
    
    if [ -f logs/file-service.pid ]; then
        kill $(cat logs/file-service.pid) 2>/dev/null || true
        rm logs/file-service.pid
    fi
    
    if [ -f logs/sync-service.pid ]; then
        kill $(cat logs/sync-service.pid) 2>/dev/null || true
        rm logs/sync-service.pid
    fi
    
    if [ -f logs/notification-service.pid ]; then
        kill $(cat logs/notification-service.pid) 2>/dev/null || true
        rm logs/notification-service.pid
    fi
    
    # Stop dependencies
    docker-compose down
    
    print_status "All services stopped!"
}

# Function to show status
show_status() {
    print_header "Service Status"
    
    echo "Dependencies:"
    docker-compose ps
    
    echo ""
    echo "Microservices:"
    ps aux | grep "go run cmd/" | grep -v grep || echo "No microservices running"
    
    echo ""
    echo "Recent logs:"
    for log in logs/*.log; do
        if [ -f "$log" ]; then
            echo "--- $(basename $log) ---"
            tail -3 "$log"
            echo ""
        fi
    done
}

# Function to show help
show_help() {
    echo "SyncVault Development Environment"
    echo ""
    echo "Usage: $0 [COMMAND]"
    echo ""
    echo "Commands:"
    echo "  start     - Start dependencies and microservices"
    echo "  stop      - Stop all services"
    echo "  restart   - Restart all services"
    echo "  status    - Show service status"
    echo "  logs      - Show service logs"
    echo "  deps      - Start only dependencies"
    echo "  services  - Start only microservices"
    echo "  help      - Show this help"
    echo ""
    echo "Examples:"
    echo "  $0 start           # Start everything"
    echo "  $0 deps            # Start only databases"
    echo "  $0 services        # Start only microservices"
    echo "  $0 status          # Check status"
}

# Main script logic
case "${1:-help}" in
    "start")
        start_dependencies
        start_microservices
        print_header "Development Environment Ready!"
        echo ""
        echo "📊 Available services:"
        echo "  🗄️  PostgreSQL: localhost:5432"
        echo "  🔴 Redis: localhost:6379"
        echo "  🍃 MongoDB: localhost:27017"
        echo ""
        echo "🔧 Microservices:"
        echo "  🔐 Auth Service: localhost:50056"
        echo "  💾 Storage Service: localhost:50054"
        echo "  📁 File Service: localhost:50052"
        echo "  🔄 Sync Service: localhost:50053"
        echo "  🔔 Notification Service: localhost:50055"
        echo ""
        echo "📋 Logs: ./logs/"
        echo "🛑 Stop with: $0 stop"
        ;;
    "stop")
        stop_services
        ;;
    "restart")
        stop_services
        sleep 2
        start_dependencies
        start_microservices
        ;;
    "status")
        show_status
        ;;
    "logs")
        print_header "Service Logs"
        tail -f logs/*.log
        ;;
    "deps")
        start_dependencies
        ;;
    "services")
        start_microservices
        ;;
    "help"|*)
        show_help
        ;;
esac
