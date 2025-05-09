package builder_test

import (
	"path/filepath"
)

var (
	testGGUFFile = filepath.Join("..", "assets", "dummy.gguf")
)

//func TestFromGGUF(t *testing.T) {
//	// Create temp directory for store
//	builder, err := builder.FromGGUF(testGGUFFile)
//	if err != nil {
//		t.Fatalf("Failed to create new model: %v", err)
//	}
//
//	// Create a test registry
//	server := httptest.NewServer(registry.New())
//	defer server.Close()
//
//	// Create a tag for the model
//	uri, err := url.Parse(server.URL)
//	if err != nil {
//		t.Fatalf("Failed to parse registry URL: %v", err)
//	}
//	tag := uri.Host + "/incomplete-test/model:v1.0.0"
//
//	if err := builder.WriteToRegistry(tag); err != nil {
//		t.Fatalf("Failed to write image to registry: %v", err)
//	}
//
//	tag := uri.Host + "/incomplete-test/model:v1.0.0"
//	builder.Write()
//
//	// Create client
//	client, err := NewClient(WithStoreRootPath(tempDir))
//	if err != nil {
//		t.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create a test registry
//	server := httptest.NewServer(registry.New())
//	defer server.Close()
//
//	// Create a tag for the model
//	uri, err := url.Parse(server.URL)
//	if err != nil {
//		t.Fatalf("Failed to parse registry URL: %v", err)
//	}
//	tag := uri.Host + "/incomplete-test/model:v1.0.0"
//
//	// Write a test model to the store with the given tag
//	builder, err := gguf.NewModel(testGGUFFile)
//	if err != nil {
//		t.Fatalf("Failed to create model: %v", err)
//	}
//	digest, err := builder.ID()
//	if err != nil {
//		t.Fatalf("Failed to get digest of original model: %v", err)
//	}
//
//	if err := client.store.Write(builder, []string{tag}, nil); err != nil {
//		t.Fatalf("Failed to push model to store: %v", err)
//	}
//
//	// Push the model to the registry
//	if err := client.PushModel(context.Background(), tag, nil); err != nil {
//		t.Fatalf("Failed to push model: %v", err)
//	}
//
//	// Delete local copy (so we can test pulling)
//	if err := client.DeleteModel(tag, false); err != nil {
//		t.Fatalf("Failed to delete model: %v", err)
//	}
//
//	// Test that model can be pulled successfully
//	if err := client.PullModel(context.Background(), tag, nil); err != nil {
//		t.Fatalf("Failed to pull model: %v", err)
//	}
//
//	// Test that model the pulled model is the same as the original (matching digests)
//	mdl2, err := client.GetModel(tag)
//	if err != nil {
//		t.Fatalf("Failed to get pulled model: %v", err)
//	}
//	digest2, err := mdl2.ID()
//	if err != nil {
//		t.Fatalf("Failed to get digest of the pulled model: %v", err)
//	}
//	if digest != digest2 {
//		t.Fatalf("Digests don't match: got %s, want %s", digest2, digest)
//	}
//}
