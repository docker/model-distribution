#!/bin/bash

# Script to check that all Docker Hub AI namespace repositories are included in the Gen AI catalog
# Usage: ./check-ai-catalog-inclusion.sh --namespace <namespace>

set -euo pipefail

# Default values
NAMESPACE=""

# Function to show usage
show_usage() {
    cat << EOF
Usage: $0 --namespace <namespace>

Check that all Docker Hub AI namespace repositories are included in the Gen AI catalog.

Required Arguments:
  --namespace <name>    Docker Hub namespace to check (e.g., 'ai')
EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        *)
            echo "Unknown argument: $1" >&2
            show_usage
            exit 1
            ;;
    esac
done

# Validate required arguments
if [[ -z "$NAMESPACE" ]]; then
    echo "Error: --namespace is required" >&2
    show_usage
    exit 1
fi

# Configuration
BASE_URL="https://hub.docker.com/v2/repositories"
CATALOG_URL="https://hub.docker.com/catalogs/gen-ai"

# Function to make API calls
api_call() {
    local url="$1"
    local temp_file=$(mktemp)
    local http_code

    # Make the API call and capture both response body and HTTP status code
    http_code=$(curl -s -w "%{http_code}" -o "$temp_file" \
        -H "Content-Type: application/json" \
        "$url")

    # Read the response body
    local body=$(cat "$temp_file")
    rm -f "$temp_file"

    if [[ "$http_code" -ne 200 ]]; then
        echo "API call failed with HTTP $http_code" >&2
        echo "URL: $url" >&2
        echo "Response: $body" >&2
        exit 1
    fi

    echo "$body"
}

# Function to fetch catalog page
fetch_catalog() {
    local temp_file=$(mktemp)
    local http_code

    # Fetch the catalog HTML page
    http_code=$(curl -s -w "%{http_code}" -o "$temp_file" \
        -H "User-Agent: Mozilla/5.0 (compatible; DockerHubChecker/1.0)" \
        "$CATALOG_URL")

    # Read the response body
    local body=$(cat "$temp_file")
    rm -f "$temp_file"

    if [[ "$http_code" -ne 200 ]]; then
        echo "Failed to fetch catalog page with HTTP $http_code" >&2
        echo "URL: $CATALOG_URL" >&2
        exit 1
    fi

    echo "$body"
}

# Function to extract AI repositories from catalog HTML
extract_catalog_repos() {
    local html_content="$1"
    
    # Extract repository names from JSON strings in the HTML
    # Look for "ai/[repo-name]" patterns and extract the repository name
    echo "$html_content" | grep -oE '"ai/[^"]*"' | \
        sed 's|"ai/||g' | \
        sed 's|"||g' | \
        sed 's|\\||g' | \
        sort -u
}

# Initialize arrays
ai_repos=()
catalog_repos=()
missing_from_catalog=()

echo "Fetching AI namespace repositories..."

# Get all repositories in the AI namespace with pagination
page=1
page_size=100
total_ai_repos=0

while true; do
    # Fetch repositories page
    url="$BASE_URL/$NAMESPACE/?page=$page&page_size=$page_size"
    response=$(api_call "$url")

    # Parse response
    repos=$(echo "$response" | jq -r '.results[]?.name // empty')
    next=$(echo "$response" | jq -r '.next // empty')

    # If no repositories on this page, break
    if [[ -z "$repos" ]]; then
        break
    fi

    # Add repositories to array
    while IFS= read -r repo_name; do
        [[ -z "$repo_name" ]] && continue
        ai_repos+=("$repo_name")
        total_ai_repos=$((total_ai_repos + 1))
    done <<< "$repos"

    # Check if there are more pages
    if [[ -z "$next" ]]; then
        break
    fi

    page=$((page + 1))
done

echo "Found $total_ai_repos repositories in AI namespace"

echo "Fetching Gen AI catalog..."

# Fetch and parse catalog
catalog_html=$(fetch_catalog)
catalog_repo_list=$(extract_catalog_repos "$catalog_html")

# Convert catalog repos to array
while IFS= read -r repo_name; do
    [[ -z "$repo_name" ]] && continue
    catalog_repos+=("$repo_name")
done <<< "$catalog_repo_list"

total_catalog_repos=${#catalog_repos[@]}
echo "Found $total_catalog_repos AI repositories in Gen AI catalog"

echo "Comparing repositories..."

# Check which AI repos are missing from catalog
for ai_repo in "${ai_repos[@]}"; do
    found=false
    for catalog_repo in "${catalog_repos[@]}"; do
        if [[ "$ai_repo" == "$catalog_repo" ]]; then
            found=true
            break
        fi
    done
    
    if [[ "$found" == false ]]; then
        missing_from_catalog+=("$ai_repo")
    fi
done

# Generate report
echo ""
echo "Gen AI Catalog Inclusion Report"
echo "==============================="
echo "Total AI namespace repositories: $total_ai_repos"
echo "AI repositories in Gen AI catalog: $total_catalog_repos"
echo "AI repositories missing from catalog: ${#missing_from_catalog[@]}"
echo ""

# Report detailed results
has_missing=false

if [[ ${#missing_from_catalog[@]} -gt 0 ]]; then
    has_missing=true
    echo "AI repositories missing from Gen AI catalog:"
    printf '  - %s\n' "${missing_from_catalog[@]}"
    echo ""
    echo "These repositories should be added to the Gen AI catalog at:"
    echo "$CATALOG_URL"
    echo ""
fi

# Final result
if [[ "$has_missing" == true ]]; then
    echo "❌ FAIL: Some AI repositories are missing from the Gen AI catalog"
    exit 1
else
    echo "✅ PASS: All AI repositories are included in the Gen AI catalog"
    exit 0
fi
