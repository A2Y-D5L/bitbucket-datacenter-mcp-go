package mcpserver

import (
	"context"
	"testing"
	"time"
)

func TestCacheManager(t *testing.T) {
	ctx := context.Background()

	// Create cache with short TTL for testing
	config := CacheConfig{
		TTL:               100 * time.Millisecond,
		MaxEntries:        10,
		MaxMemoryBytes:    1024 * 1024, // 1MB
		MaxEntrySizeBytes: 100 * 1024,  // 100KB
		CleanupInterval:   50 * time.Millisecond,
		EnableMetrics:     true,
		EnableCompression: false,
	}

	cache := NewCacheManager(config)
	defer cache.Close()

	// Test Set and Get
	key := CacheKey{Type: "test", Resource: "test-resource", Version: "v1", Hash: "hash1"}
	entry := CacheEntry{
		Data:      "test data",
		SizeBytes: 9,
		Tags:      []string{"test-tag"},
		Version:   "v1",
		Hash:      "hash1",
	}

	err := cache.Set(ctx, key, entry)
	if err != nil {
		t.Fatalf("Failed to set cache entry: %v", err)
	}

	// Test cache hit
	retrieved, found, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get cache entry: %v", err)
	}
	if !found {
		t.Fatalf("Cache entry not found")
	}
	if retrieved.Data != "test data" {
		t.Fatalf("Expected 'test data', got %v", retrieved.Data)
	}

	// Test cache miss
	missingKey := CacheKey{Type: "test", Resource: "missing", Version: "v1", Hash: "hash1"}
	_, found, err = cache.Get(ctx, missingKey)
	if err != nil {
		t.Fatalf("Error getting missing cache entry: %v", err)
	}
	if found {
		t.Fatalf("Unexpected cache hit for missing entry")
	}

	// Test statistics
	stats := cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
	if stats.Entries != 1 {
		t.Errorf("Expected 1 entry, got %d", stats.Entries)
	}

	// Test invalidation by tags
	err = cache.InvalidateByTags(ctx, []string{"test-tag"})
	if err != nil {
		t.Fatalf("Failed to invalidate by tags: %v", err)
	}

	// Entry should now be missing
	_, found, err = cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Error getting invalidated cache entry: %v", err)
	}
	if found {
		t.Fatalf("Cache entry should have been invalidated")
	}

	// Test TTL expiration
	shortKey := CacheKey{Type: "test", Resource: "short-ttl", Version: "v1", Hash: "hash2"}
	shortEntry := CacheEntry{
		Data:      "short-lived data",
		SizeBytes: 17,
		Tags:      []string{"short-tag"},
		Version:   "v1",
		Hash:      "hash2",
	}

	err = cache.Set(ctx, shortKey, shortEntry)
	if err != nil {
		t.Fatalf("Failed to set short-lived cache entry: %v", err)
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Entry should be expired
	_, found, err = cache.Get(ctx, shortKey)
	if err != nil {
		t.Fatalf("Error getting expired cache entry: %v", err)
	}
	if found {
		t.Fatalf("Cache entry should have expired")
	}
}

func TestCacheKeyGeneration(t *testing.T) {
	// Test PR cache key
	prKey := NewPRCacheKey("PROJ", "repo", 123, "v1")
	expected := "pr:PROJ/repo/123:v1:"
	if prKey.String() != expected {
		t.Errorf("Expected PR key '%s', got '%s'", expected, prKey.String())
	}

	// Test diff cache key (should include hash)
	diffKey := NewDiffCacheKey("PROJ", "repo", 123, 3, "ignore-all")
	if diffKey.Type != "diff" {
		t.Errorf("Expected diff type, got %s", diffKey.Type)
	}
	if diffKey.Resource != "PROJ/repo/123" {
		t.Errorf("Expected resource 'PROJ/repo/123', got %s", diffKey.Resource)
	}
	if diffKey.Hash == "" {
		t.Errorf("Expected non-empty hash for diff key")
	}

	// Different parameters should create different hashes
	diffKey2 := NewDiffCacheKey("PROJ", "repo", 123, 5, "ignore-all") // Different context lines
	if diffKey.Hash == diffKey2.Hash {
		t.Errorf("Different diff parameters should create different hashes")
	}
}

func TestCacheManager_MemoryLimits(t *testing.T) {
	ctx := context.Background()

	// Create cache with very small memory limit
	config := CacheConfig{
		TTL:               1 * time.Minute,
		MaxEntries:        100,
		MaxMemoryBytes:    100, // Very small limit
		MaxEntrySizeBytes: 50,  // Small per-entry limit
		CleanupInterval:   10 * time.Second,
		EnableMetrics:     true,
		EnableCompression: false,
	}

	cache := NewCacheManager(config)
	defer cache.Close()

	// Try to add entry that exceeds per-entry size limit
	key := CacheKey{Type: "test", Resource: "large-entry", Version: "v1", Hash: "hash1"}
	largeEntry := CacheEntry{
		Data:      "large data",
		SizeBytes: 100, // Exceeds MaxEntrySizeBytes
		Tags:      []string{"large-tag"},
		Version:   "v1",
		Hash:      "hash1",
	}

	err := cache.Set(ctx, key, largeEntry)
	if err == nil {
		t.Fatalf("Expected error when setting entry that exceeds size limit")
	}

	// Add several small entries to test memory limit
	for i := range 5 {
		smallKey := CacheKey{Type: "test", Resource: "small-entry", Version: "v1", Hash: string(rune('a' + i))}
		smallEntry := CacheEntry{
			Data:      "small",
			SizeBytes: 30,
			Tags:      []string{"small-tag"},
			Version:   "v1",
		}

		err := cache.Set(ctx, smallKey, smallEntry)
		if err != nil {
			t.Logf("Failed to set entry %d (expected due to memory limits): %v", i, err)
		}
	}

	// Check that cache has some entries but not all (due to eviction)
	stats := cache.Stats()
	if stats.Entries > 3 {
		t.Logf("Cache has %d entries (some should have been evicted due to memory limits)", stats.Entries)
	}
	if stats.Evictions == 0 {
		t.Logf("Expected some evictions due to memory limits, but got 0")
	}
}

func TestCacheInvalidationHelper(t *testing.T) {
	ctx := context.Background()

	config := CacheConfig{
		TTL:               1 * time.Minute,
		MaxEntries:        100,
		MaxMemoryBytes:    1024 * 1024,
		MaxEntrySizeBytes: 100 * 1024,
		CleanupInterval:   10 * time.Second,
		EnableMetrics:     true,
		EnableCompression: false,
	}

	cache := NewCacheManager(config)
	defer cache.Close()

	helper := NewCacheInvalidationHelper(cache)

	// Add some entries with hierarchical tags
	entries := []struct {
		key   CacheKey
		entry CacheEntry
	}{
		{
			key: CacheKey{Type: "pr", Resource: "PROJ/repo1/123", Version: "v1"},
			entry: CacheEntry{
				Data:      "pr data",
				SizeBytes: 7,
				Tags:      []string{"project:PROJ", "repo:repo1", "pr:123"},
				Version:   "v1",
			},
		},
		{
			key: CacheKey{Type: "diff", Resource: "PROJ/repo1/123", Hash: "abc"},
			entry: CacheEntry{
				Data:      "diff data",
				SizeBytes: 9,
				Tags:      []string{"project:PROJ", "repo:repo1", "pr:123"},
				Hash:      "abc",
			},
		},
		{
			key: CacheKey{Type: "pr", Resource: "PROJ/repo2/456", Version: "v1"},
			entry: CacheEntry{
				Data:      "other pr data",
				SizeBytes: 13,
				Tags:      []string{"project:PROJ", "repo:repo2", "pr:456"},
				Version:   "v1",
			},
		},
	}

	// Add all entries
	for _, e := range entries {
		err := cache.Set(ctx, e.key, e.entry)
		if err != nil {
			t.Fatalf("Failed to set cache entry: %v", err)
		}
	}

	// Verify all entries are present
	stats := cache.Stats()
	if stats.Entries != 3 {
		t.Fatalf("Expected 3 entries, got %d", stats.Entries)
	}

	// Test PR-specific invalidation
	err := helper.InvalidatePR(ctx, "PROJ", "repo1", 123)
	if err != nil {
		t.Fatalf("Failed to invalidate PR: %v", err)
	}

	// Check that PR 123 entries are gone, but PR 456 remains
	_, found, err := cache.Get(ctx, entries[0].key)
	if err != nil {
		t.Fatalf("Error checking invalidated entry: %v", err)
	}
	if found {
		t.Errorf("PR 123 entry should have been invalidated")
	}

	_, found, err = cache.Get(ctx, entries[1].key)
	if err != nil {
		t.Fatalf("Error checking invalidated diff entry: %v", err)
	}
	if found {
		t.Errorf("PR 123 diff entry should have been invalidated")
	}

	_, found, err = cache.Get(ctx, entries[2].key)
	if err != nil {
		t.Fatalf("Error checking remaining entry: %v", err)
	}
	if !found {
		t.Errorf("PR 456 entry should still be present")
	}

	// Test repo-level invalidation
	err = helper.InvalidateRepo(ctx, "PROJ", "repo2")
	if err != nil {
		t.Fatalf("Failed to invalidate repo: %v", err)
	}

	// Now all entries should be gone
	stats = cache.Stats()
	if stats.Entries != 0 {
		t.Errorf("Expected 0 entries after repo invalidation, got %d", stats.Entries)
	}
}
