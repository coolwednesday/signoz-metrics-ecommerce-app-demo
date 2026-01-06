#!/bin/bash

###############################################################################
# Traffic Generation Script for Single User
#
# This script simulates a single user's behavior on the e-commerce application.
# It performs various actions like browsing products, managing cart, and
# creating orders.
#
# Usage:
#   ./generate-traffic-single-user.sh [OPTIONS]
#
# Options:
#   -u, --url URL          Base URL (default: http://localhost:8080)
#   -d, --duration SEC     Duration in seconds (default: infinite)
#   -i, --id USER_ID       User ID to simulate (default: random 1000-9999)
#   -h, --help             Show this help message
###############################################################################

set -euo pipefail

# Configuration
BASE_URL="${BASE_URL:-http://localhost:8080}"
DURATION="${DURATION:-0}"
USER_ID="${USER_ID:-$(($RANDOM % 9000 + 1000))}"

# Internal state
SCRIPT_START_TIME=$(date +%s)
REQUEST_STATUS_CODE=0
REQUEST_RESPONSE_BODY=""
PRODUCT_IDS=()
DB_USER_ID=0

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Helper: Random integer
random_int() {
    local min=$1
    local max=$2
    echo $((RANDOM % (max - min + 1) + min))
}

# Helper: Random element from array
random_element() {
    local arr=("$@")
    echo "${arr[$RANDOM % ${#arr[@]}]}"
}

# Helper: Random delay
random_delay() {
    local delay=$(awk "BEGIN {printf \"%.2f\", 0.5 + rand() * 2.5}")
    sleep "$delay"
}

# Helper: Make HTTP request
make_request() {
    local method=$1
    local endpoint=$2
    local data="${3:-}"
    
    local url="${BASE_URL}${endpoint}"
    # Use DB_USER_ID if available, otherwise fallback to USER_ID
    local uid=$USER_ID
    if [[ $DB_USER_ID -gt 0 ]]; then
        uid=$DB_USER_ID
    fi

    if [[ "$endpoint" != "/api/v1/users" ]]; then
        if [[ "$endpoint" == *"?"* ]]; then
            url="${url}&user_id=${uid}"
        else
            url="${url}?user_id=${uid}"
        fi
    fi
    
    local response
    if [[ "$method" == "GET" ]]; then
        response=$(curl -s --max-time 10 -w "\n%{http_code}" "$url" 2>&1 || echo -e "\n000")
    elif [[ "$method" == "POST" ]]; then
        response=$(curl -s --max-time 10 -w "\n%{http_code}" -X POST \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$url" 2>&1 || echo -e "\n000")
    elif [[ "$method" == "PUT" ]]; then
        response=$(curl -s --max-time 10 -w "\n%{http_code}" -X PUT \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$url" 2>&1 || echo -e "\n000")
    fi
    
    # Robust status code extraction using awk
    local status_line=$(echo "$response" | tail -n1)
    REQUEST_STATUS_CODE=$(echo "$status_line" | awk '{print $1}' | grep -oE '[0-9]{3}' || echo "000")
    REQUEST_RESPONSE_BODY=$(echo "$response" | sed '$d')
    
    if [[ "$REQUEST_STATUS_CODE" == "000" ]]; then
        echo -e "${RED}[ERROR] ${method} ${endpoint} - Connection failed${NC}" >&2
        return 1
    elif [[ "$REQUEST_STATUS_CODE" -ge 500 ]]; then
        echo -e "${RED}[ERROR] ${method} ${endpoint} failed with status ${REQUEST_STATUS_CODE}${NC}" >&2
        return 1
    fi
    
    return 0
}

# Fetch product IDs
fetch_product_ids() {
    echo -e "${YELLOW}Fetching product IDs...${NC}"
    if make_request "GET" "/api/v1/products?limit=100"; then
        PRODUCT_IDS=($(echo "$REQUEST_RESPONSE_BODY" | grep -oE '"id":[0-9]+' | cut -d: -f2))
        echo -e "${GREEN}Found ${#PRODUCT_IDS[@]} products${NC}"
    else
        echo -e "${RED}Failed to fetch product IDs, using defaults${NC}"
        PRODUCT_IDS=(1 2 3 4 5)
    fi
}

# Register/Verify user and get DB ID
register_user() {
    echo -e "${YELLOW}Registering/Verifying user user${USER_ID}@example.com...${NC}"
    make_request "POST" "/api/v1/users" "{\"id\": ${USER_ID}, \"email\": \"user${USER_ID}@example.com\", \"name\": \"User ${USER_ID}\"}"
    
    if [[ "$REQUEST_STATUS_CODE" == "201" ]] || [[ "$REQUEST_STATUS_CODE" == "409" ]]; then
        # Extract ID from response (if 201) or fetch by email (if 409)
        if [[ "$REQUEST_STATUS_CODE" == "201" ]]; then
            DB_USER_ID=$(echo "$REQUEST_RESPONSE_BODY" | grep -oE '"id":[0-9]+' | cut -d: -f2)
        else
            DB_USER_ID=$(echo "$REQUEST_RESPONSE_BODY" | grep -oE '"id":[0-9]+' | cut -d: -f2 || echo "0")
        fi

        if [[ $DB_USER_ID -gt 0 ]]; then
            echo -e "${GREEN}User verified with Database ID: ${DB_USER_ID}${NC}"
            return 0
        fi
    fi

    echo -e "${RED}Failed to get Database User ID${NC}"
    return 1
}

# Check duration
check_duration() {
    if [[ $DURATION -gt 0 ]]; then
        local elapsed=$(($(date +%s) - SCRIPT_START_TIME))
        if [[ $elapsed -ge $DURATION ]]; then
            return 1
        fi
    fi
    return 0
}

# Actions
browse_products() {
    # List products
    make_request "GET" "/api/v1/products?limit=20"
    random_delay

    # View random products
    local num=$(random_int 2 5)
    for ((i=0; i<num; i++)); do
        local pid=$(random_element "${PRODUCT_IDS[@]}")
        make_request "GET" "/api/v1/products/${pid}"
        random_delay

        if [[ $(random_int 1 100) -le 40 ]]; then
            make_request "GET" "/api/v1/products/${pid}/inventory?warehouse_id=WH-001"
            random_delay
        fi
    done
}

manage_cart() {
    local pid=$(random_element "${PRODUCT_IDS[@]}")
    local qty=$(random_int 1 3)

    # Add to cart
    make_request "POST" "/api/v1/cart/add" "{\"product_id\": ${pid}, \"quantity\": ${qty}}"
    random_delay

    # View cart
    make_request "GET" "/api/v1/cart"
    random_delay

    # Sometimes remove
    if [[ $(random_int 1 100) -le 30 ]]; then
        make_request "POST" "/api/v1/cart/remove" "{\"product_id\": ${pid}}"
        random_delay
    fi
}

# ============================================
# ENHANCED: Multiple Order Creation Variants
# ============================================
create_order() {
    # Ensure items in cart
    manage_cart

    # FIX #1: RANDOM PAYMENT METHOD
    local payment_methods=("credit_card" "debit_card" "paypal" "bank_transfer")
    local payment_method=$(random_element "${payment_methods[@]}")

    local data="{\"payment_method\": \"${payment_method}\", \"currency\": \"USD\"}"
    make_request "POST" "/api/v1/orders" "$data"

    if [[ $REQUEST_STATUS_CODE -eq 201 ]]; then
        echo -e "${GREEN}[SUCCESS] User ${USER_ID}: Order created (${payment_method})${NC}"

        # FIX #2: MIXED ORDER STATES (70% completed, 30% pending)
        local order_id=$(echo "$REQUEST_RESPONSE_BODY" | grep -oE '"id":[0-9]+' | head -1 | cut -d: -f2)

        if [[ -n "$order_id" && "$order_id" -gt 0 ]]; then
            # Random decision: 70% chance to complete, 30% chance to leave as pending
            local completion_roll=$(random_int 1 100)

            if [[ $completion_roll -le 70 ]]; then
                # 70% - Auto-complete the order (simulate successful payment)
                sleep 0.5
                local status_data='{"status": "completed"}'
                if make_request "PUT" "/api/v1/orders/${order_id}/status" "$status_data"; then
                    if [[ $REQUEST_STATUS_CODE -eq 200 ]]; then
                        echo -e "${GREEN}[SUCCESS] User ${USER_ID}: Order ${order_id} auto-completed (${payment_method})${NC}"
                    fi
                fi
            else
                # 30% - Leave order as pending (simulate pending payment)
                echo -e "${YELLOW}[INFO] User ${USER_ID}: Order ${order_id} left as PENDING (${payment_method})${NC}"
            fi
        fi
    fi
    random_delay
}

# Main loop
echo -e "${GREEN}Starting traffic generation for user ${USER_ID}...${NC}"
echo -e "${YELLOW}Base URL: ${BASE_URL}${NC}"

# Register/Verify user and get DB ID
if ! register_user; then
    echo -e "${RED}Failed to register/verify user, exiting${NC}"
    exit 1
fi

# Fetch product IDs
fetch_product_ids

while check_duration; do
    roll=$(random_int 1 100)
    if [[ $roll -le 40 ]]; then
        browse_products
    elif [[ $roll -le 70 ]]; then
        manage_cart
    elif [[ $roll -le 90 ]]; then
        create_order
    else
        # Occasional health check and user info
        make_request "GET" "/health"
        make_request "GET" "/api/v1/users"
    fi
    
    # Random delay between session actions
    random_delay
done

echo -e "${GREEN}Traffic generation for user ${USER_ID} completed.${NC}"
