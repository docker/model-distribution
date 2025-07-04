#!/bin/bash

# Script to check Docker Hub namespace repositories for missing descriptions
# Usage: ./check-namespace-descriptions.sh --namespace <namespace> --token <token>

set -euo pipefail

# Default values
NAMESPACE=""

# Function to show usage
show_usage() {
    cat << EOF
Usage: $0 --namespace <namespace> [options]

Check Docker Hub namespace repositories for missing descriptions.

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

# Function to check if a string is empty or null
is_empty_or_null() {
    local value="$1"
    [[ "$value" == "null" || "$value" == '""' || "$value" == "" ]]
}

# Initialize counters and arrays
total_repos=0
repos_missing_description=()
repos_missing_full_description=()
repos_missing_both=()

# Get all repositories in the namespace with pagination
page=1
page_size=100

while true; do

    # Fetch repositories page
    url="$BASE_URL/$NAMESPACE/?page=$page&page_size=$page_size"
    response=$(api_call "$url")

    # Parse response
    repos=$(echo "$response" | jq -r '.results[]?.name // empty')
    count=$(echo "$response" | jq -r '.count // 0')
    next=$(echo "$response" | jq -r '.next // empty')

    # If no repositories on this page, break
    if [[ -z "$repos" ]]; then
        break
    fi

    # Process each repository
    while IFS= read -r repo_name; do
        [[ -z "$repo_name" ]] && continue

        total_repos=$((total_repos + 1))

        # Get detailed repository information
        repo_url="$BASE_URL/$NAMESPACE/$repo_name/"
        repo_response=$(api_call "$repo_url")

        # Extract description fields
        description=$(echo "$repo_response" | jq -r '.description // null')
        full_description=$(echo "$repo_response" | jq -r '.full_description // null')

        # Check if descriptions are missing
        description_missing=false
        full_description_missing=false

        if is_empty_or_null "$description"; then
            description_missing=true
            repos_missing_description+=("$repo_name")
        fi

        if is_empty_or_null "$full_description"; then
            full_description_missing=true
            repos_missing_full_description+=("$repo_name")
        fi

        if [[ "$description_missing" == true && "$full_description_missing" == true ]]; then
            repos_missing_both+=("$repo_name")
        fi

    done <<< "$repos"

    # Check if there are more pages
    if [[ -z "$next" ]]; then
        break
    fi

    page=$((page + 1))
done

echo "Summary Report"
echo "================="
echo "Total repositories checked: $total_repos"
echo "Repositories missing description: ${#repos_missing_description[@]}"
echo "Repositories missing full_description: ${#repos_missing_full_description[@]}"
echo "Repositories missing both: ${#repos_missing_both[@]}"
echo ""

# Report detailed results
has_issues=false

if [[ ${#repos_missing_description[@]} -gt 0 ]]; then
    has_issues=true
    echo "Repositories missing description:"
    printf '  - %s\n' "${repos_missing_description[@]}"
    echo ""
fi

if [[ ${#repos_missing_full_description[@]} -gt 0 ]]; then
    has_issues=true
    echo "Repositories missing full_description:"
    printf '  - %s\n' "${repos_missing_full_description[@]}"
    echo ""
fi

if [[ ${#repos_missing_both[@]} -gt 0 ]]; then
    echo "Repositories missing both description and full_description:"
    printf '  - %s\n' "${repos_missing_both[@]}"
    echo ""
fi

# Final result
if [[ "$has_issues" == true ]]; then
    exit 1
else
    exit 0
fi
