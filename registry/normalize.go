package registry

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

const (
	// defaultDomain is the default domain used for models on Docker Hub.
	// It is used to normalize "familiar" names to canonical names, for example,
	// to convert "llama" to "docker.io/ai/llama:latest".
	//
	// Note that actual domain of Docker Hub's registry is registry-1.docker.io.
	// This domain will continue to be supported, but there are plans to consolidate
	// legacy domains to new "canonical" domains. Once those domains are decided
	// on, we must update the normalization functions, but preserve compatibility
	// with existing installs, clients, and user configuration.
	defaultDomain = "docker.io"

	// officialRepoPrefix is the namespace used for official AI models on Docker Hub.
	// It is used to normalize "familiar" names to canonical names, for example,
	// to convert "llama" to "docker.io/ai/llama:latest".
	officialRepoPrefix = "ai/"

	// defaultTag is the default tag applied when no tag is specified
	defaultTag = "latest"
)

// Normalize takes a model reference and returns a normalized reference.
// It converts "familiar" names to canonical names following these rules:
//   - If no domain is specified, defaultDomain is used
//   - If no namespace is specified and the domain is defaultDomain, officialRepoPrefix is used
//   - If no tag is specified, defaultTag is used
//
// Examples:
//   - "llama" -> "docker.io/ai/llama:latest"
//   - "myorg/llama" -> "docker.io/myorg/llama:latest"
//   - "llama:v1.0" -> "docker.io/ai/llama:v1.0"
//   - "registry.example.com/llama" -> "registry.example.com/llama:latest"
//   - "docker.io/myorg/llama:v1.0" -> "docker.io/myorg/llama:v1.0" (no change)
func Normalize(reference string) (string, error) {
	if reference == "" {
		return "", fmt.Errorf("reference cannot be empty")
	}

	// Parse the reference manually to avoid go-containerregistry's built-in normalization
	normalizedRef := normalizeReference(reference)

	// Validate the normalized reference
	if _, err := name.ParseReference(normalizedRef); err != nil {
		return "", fmt.Errorf("failed to normalize reference %q: %w", reference, err)
	}

	return normalizedRef, nil
}

// normalizeReference performs the actual normalization logic
func normalizeReference(reference string) string {
	// Handle digest references
	if strings.Contains(reference, "@") {
		parts := strings.SplitN(reference, "@", 2)
		namePort := parts[0]
		digest := parts[1]
		normalizedName := normalizeNameWithoutTag(namePort)
		return normalizedName + "@" + digest
	}

	// Handle tag references  
	var name, tag string
	
	// Look for the pattern where we have a tag (colon followed by something that doesn't look like a port)
	if colonIdx := strings.LastIndex(reference, ":"); colonIdx != -1 {
		beforeColon := reference[:colonIdx]
		afterColon := reference[colonIdx+1:]
		
		// It's a tag if:
		// 1. The part after colon doesn't contain slashes, dots, or other port-like characters
		// 2. AND (the part before colon contains a slash OR the part before colon doesn't contain dots)
		// This handles: "repo:tag", "org/repo:tag", "registry.com/repo:tag" but not "registry.com:port"
		hasSlashBefore := strings.Contains(beforeColon, "/")
		hasDotBefore := strings.Contains(beforeColon, ".")
		validTagChars := !strings.ContainsAny(afterColon, "/:")
		
		isTag := validTagChars && (hasSlashBefore || !hasDotBefore)
		
		if isTag {
			name = beforeColon
			tag = afterColon
		} else {
			// It's a port, treat whole thing as name
			name = reference
			tag = defaultTag
		}
	} else {
		name = reference
		tag = defaultTag
	}

	normalizedName := normalizeNameWithoutTag(name)
	return normalizedName + ":" + tag
}

// normalizeNameWithoutTag normalizes just the name part (without tag or digest)
func normalizeNameWithoutTag(name string) string {
	// If it contains a dot in the first component, it likely has a registry
	parts := strings.Split(name, "/")
	if len(parts) > 1 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		// Has registry domain
		registry := parts[0]
		repo := strings.Join(parts[1:], "/")
		
		// Special handling for docker.io domain
		if registry == defaultDomain || registry == "index.docker.io" {
			// Use our default domain
			registry = defaultDomain
			
			// If repo doesn't contain slash and doesn't start with our prefix, add it
			if !strings.Contains(repo, "/") && !strings.HasPrefix(repo, officialRepoPrefix) {
				repo = officialRepoPrefix + repo
			}
		}
		
		return registry + "/" + repo
	}

	// No registry specified - use defaults
	if strings.Contains(name, "/") {
		// Has organization but no registry
		return defaultDomain + "/" + name
	}

	// Simple name - add default domain and official prefix
	return defaultDomain + "/" + officialRepoPrefix + name
}