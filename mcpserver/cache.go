package mcpserver

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Cache interface provides a clean abstraction for testability and flexibility
type Cache interface {
	Get(ctx context.Context, key CacheKey) (CacheEntry, bool, error)
	Set(ctx context.Context, key CacheKey, entry CacheEntry) error
	Delete(ctx context.Context, key CacheKey) error
	InvalidateByPattern(ctx context.Context, pattern string) error
	InvalidateByTags(ctx context.Context, tags []string) error
	Stats() CacheStats
	Close() error
}

// CacheEntry represents a cached item with metadata for intelligent management
type CacheEntry struct {
	Data       any       `json:"data"`
	SizeBytes  int64     `json:"sizeBytes"` // Memory usage tracking
	Tags       []string  `json:"tags"`      // For bulk invalidation
	Version    string    `json:"version"`   // PR version for invalidation
	Hash       string    `json:"hash"`      // Content hash for diffs
	CreatedAt  time.Time `json:"createdAt"`
	AccessedAt time.Time `json:"accessedAt"` // For LRU tracking
}

// CacheKey represents a unique identifier for cached items
type CacheKey struct {
	Type     string // "pr", "diff", "comments", "activities"
	Resource string // "PROJ/repo/123"
	Version  string // PR version for invalidation
	Hash     string // Content hash for diffs
}

// String serializes the cache key for storage
func (k CacheKey) String() string {
	return fmt.Sprintf("%s:%s:%s:%s", k.Type, k.Resource, k.Version, k.Hash)
}

// CacheConfig holds configuration for cache behavior
type CacheConfig struct {
	TTL               time.Duration `default:"10m"`
	MaxEntries        int           `default:"1000"`
	MaxMemoryBytes    int64         `default:"100MB"`
	MaxEntrySizeBytes int64         `default:"10MB"`
	CleanupInterval   time.Duration `default:"1m"`
	EnableMetrics     bool          `default:"true"`
	EnableCompression bool          `default:"true"` // For large diffs
}

// CacheStats provides observability into cache performance
type CacheStats struct {
	Hits        int64   `json:"hits"`
	Misses      int64   `json:"misses"`
	Evictions   int64   `json:"evictions"`
	Entries     int64   `json:"entries"`
	MemoryUsage int64   `json:"memoryUsage"`
	HitRate     float64 `json:"hitRate"`
}

// CacheManager implements the Cache interface with in-memory storage
type CacheManager struct {
	mu           sync.RWMutex
	entries      map[string]*cacheItem
	config       CacheConfig
	memoryUsage  int64
	stats        CacheStats
	cleanupTimer *time.Timer
	stopCleanup  chan bool
}

// cacheItem is the internal representation with expiration
type cacheItem struct {
	entry     CacheEntry
	expiresAt time.Time
}

// NewCacheManager creates a new cache manager with the given configuration
func NewCacheManager(config CacheConfig) *CacheManager {
	// Set defaults
	if config.TTL == 0 {
		config.TTL = 10 * time.Minute
	}
	if config.MaxEntries == 0 {
		config.MaxEntries = 1000
	}
	if config.MaxMemoryBytes == 0 {
		config.MaxMemoryBytes = 100 * 1024 * 1024 // 100MB
	}
	if config.MaxEntrySizeBytes == 0 {
		config.MaxEntrySizeBytes = 10 * 1024 * 1024 // 10MB
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 1 * time.Minute
	}

	cm := &CacheManager{
		entries:     make(map[string]*cacheItem),
		config:      config,
		stopCleanup: make(chan bool),
	}

	// Start cleanup routine
	cm.startCleanup()

	return cm
}

// Get retrieves an entry from the cache
func (cm *CacheManager) Get(ctx context.Context, key CacheKey) (CacheEntry, bool, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	keyStr := key.String()
	item, exists := cm.entries[keyStr]

	if !exists {
		cm.stats.Misses++
		return CacheEntry{}, false, nil
	}

	// Check expiration
	if time.Now().After(item.expiresAt) {
		delete(cm.entries, keyStr)
		cm.memoryUsage -= item.entry.SizeBytes
		cm.stats.Misses++
		cm.stats.Evictions++
		return CacheEntry{}, false, nil
	}

	// Update access time for LRU
	item.entry.AccessedAt = time.Now()
	cm.stats.Hits++

	return item.entry, true, nil
}

// Set stores an entry in the cache
func (cm *CacheManager) Set(ctx context.Context, key CacheKey, entry CacheEntry) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check entry size limit
	if entry.SizeBytes > cm.config.MaxEntrySizeBytes {
		return fmt.Errorf("entry size %d exceeds maximum %d bytes", entry.SizeBytes, cm.config.MaxEntrySizeBytes)
	}

	keyStr := key.String()

	// Calculate entry size if not provided
	if entry.SizeBytes == 0 {
		if data, err := json.Marshal(entry.Data); err == nil {
			entry.SizeBytes = int64(len(data))
		}
	}

	entry.CreatedAt = time.Now()
	entry.AccessedAt = time.Now()

	// Check if we need to evict entries
	cm.ensureCapacity(entry.SizeBytes)

	// Remove existing entry if present
	if existing, exists := cm.entries[keyStr]; exists {
		cm.memoryUsage -= existing.entry.SizeBytes
	}

	// Add new entry
	cm.entries[keyStr] = &cacheItem{
		entry:     entry,
		expiresAt: time.Now().Add(cm.config.TTL),
	}
	cm.memoryUsage += entry.SizeBytes

	return nil
}

// Delete removes an entry from the cache
func (cm *CacheManager) Delete(ctx context.Context, key CacheKey) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	keyStr := key.String()
	if item, exists := cm.entries[keyStr]; exists {
		delete(cm.entries, keyStr)
		cm.memoryUsage -= item.entry.SizeBytes
	}

	return nil
}

// InvalidateByPattern removes entries matching a pattern
func (cm *CacheManager) InvalidateByPattern(ctx context.Context, pattern string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var toDelete []string
	for key, item := range cm.entries {
		if strings.Contains(key, pattern) {
			toDelete = append(toDelete, key)
			cm.memoryUsage -= item.entry.SizeBytes
		}
	}

	for _, key := range toDelete {
		delete(cm.entries, key)
		cm.stats.Evictions++
	}

	return nil
}

// InvalidateByTags removes entries that have any of the specified tags
func (cm *CacheManager) InvalidateByTags(ctx context.Context, tags []string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}

	var toDelete []string
	for key, item := range cm.entries {
		for _, entryTag := range item.entry.Tags {
			if tagSet[entryTag] {
				toDelete = append(toDelete, key)
				cm.memoryUsage -= item.entry.SizeBytes
				break
			}
		}
	}

	for _, key := range toDelete {
		delete(cm.entries, key)
		cm.stats.Evictions++
	}

	return nil
}

// Stats returns current cache statistics
func (cm *CacheManager) Stats() CacheStats {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	stats := cm.stats
	stats.Entries = int64(len(cm.entries))
	stats.MemoryUsage = cm.memoryUsage

	if stats.Hits+stats.Misses > 0 {
		stats.HitRate = float64(stats.Hits) / float64(stats.Hits+stats.Misses)
	}

	return stats
}

// Close shuts down the cache manager
func (cm *CacheManager) Close() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.stopCleanup != nil {
		close(cm.stopCleanup)
		cm.stopCleanup = nil
	}

	if cm.cleanupTimer != nil {
		cm.cleanupTimer.Stop()
	}

	// Clear all entries
	cm.entries = make(map[string]*cacheItem)
	cm.memoryUsage = 0

	return nil
}

// ensureCapacity evicts entries if necessary to make room for new entry
func (cm *CacheManager) ensureCapacity(newEntrySize int64) {
	// Check memory limit
	for cm.memoryUsage+newEntrySize > cm.config.MaxMemoryBytes && len(cm.entries) > 0 {
		cm.evictLRU()
	}

	// Check entry count limit
	for len(cm.entries) >= cm.config.MaxEntries {
		cm.evictLRU()
	}
}

// evictLRU removes the least recently used entry
func (cm *CacheManager) evictLRU() {
	var oldestKey string
	var oldestTime time.Time = time.Now()

	for key, item := range cm.entries {
		if item.entry.AccessedAt.Before(oldestTime) {
			oldestTime = item.entry.AccessedAt
			oldestKey = key
		}
	}

	if oldestKey != "" {
		if item, exists := cm.entries[oldestKey]; exists {
			delete(cm.entries, oldestKey)
			cm.memoryUsage -= item.entry.SizeBytes
			cm.stats.Evictions++
		}
	}
}

// startCleanup begins the periodic cleanup of expired entries
func (cm *CacheManager) startCleanup() {
	cm.cleanupTimer = time.NewTimer(cm.config.CleanupInterval)

	go func() {
		for {
			select {
			case <-cm.cleanupTimer.C:
				cm.cleanup()
				cm.cleanupTimer.Reset(cm.config.CleanupInterval)
			case <-cm.stopCleanup:
				return
			}
		}
	}()
}

// cleanup removes expired entries
func (cm *CacheManager) cleanup() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	var toDelete []string

	for key, item := range cm.entries {
		if now.After(item.expiresAt) {
			toDelete = append(toDelete, key)
			cm.memoryUsage -= item.entry.SizeBytes
		}
	}

	for _, key := range toDelete {
		delete(cm.entries, key)
		cm.stats.Evictions++
	}

	if len(toDelete) > 0 {
		log.Printf("Cache cleanup: removed %d expired entries", len(toDelete))
	}
}

// Helper functions for cache key generation

// NewPRCacheKey creates a cache key for PR data
func NewPRCacheKey(projectKey, repoSlug string, prID int, version string) CacheKey {
	return CacheKey{
		Type:     "pr",
		Resource: fmt.Sprintf("%s/%s/%d", projectKey, repoSlug, prID),
		Version:  version,
	}
}

// NewDiffCacheKey creates a cache key for diff data with content hash
func NewDiffCacheKey(projectKey, repoSlug string, prID int, contextLines int, whitespace string) CacheKey {
	hash := fmt.Sprintf("%x", md5.Sum(fmt.Appendf(nil, "%d:%s", contextLines, whitespace)))
	return CacheKey{
		Type:     "diff",
		Resource: fmt.Sprintf("%s/%s/%d", projectKey, repoSlug, prID),
		Hash:     hash,
	}
}

// NewCommentsCacheKey creates a cache key for comments data
func NewCommentsCacheKey(projectKey, repoSlug string, prID int, start, limit int) CacheKey {
	hash := fmt.Sprintf("%x", md5.Sum(fmt.Appendf(nil, "%d:%d", start, limit)))
	return CacheKey{
		Type:     "comments",
		Resource: fmt.Sprintf("%s/%s/%d", projectKey, repoSlug, prID),
		Hash:     hash,
	}
}

// NewActivitiesCacheKey creates a cache key for activities data
func NewActivitiesCacheKey(projectKey, repoSlug string, prID int, start, limit int) CacheKey {
	hash := fmt.Sprintf("%x", md5.Sum(fmt.Appendf(nil, "%d:%d", start, limit)))
	return CacheKey{
		Type:     "activities",
		Resource: fmt.Sprintf("%s/%s/%d", projectKey, repoSlug, prID),
		Hash:     hash,
	}
}

// CachedMCPHandler provides a middleware wrapper for MCP tools with caching
type CachedMCPHandler[T any, R any] struct {
	cache   Cache
	handler mcp.ToolHandlerFor[T, R]
	keyFunc func(T) CacheKey
	tags    func(T) []string
	enabled bool
}

// NewCachedMCPHandler creates a new cached MCP handler
func NewCachedMCPHandler[T any, R any](
	cache Cache,
	handler mcp.ToolHandlerFor[T, R],
	keyFunc func(T) CacheKey,
	tags func(T) []string,
) mcp.ToolHandlerFor[T, R] {
	cachedHandler := &CachedMCPHandler[T, R]{
		cache:   cache,
		handler: handler,
		keyFunc: keyFunc,
		tags:    tags,
		enabled: true,
	}

	// Return a function that matches ToolHandlerFor signature
	return func(ctx context.Context, req *mcp.CallToolRequest, input T) (*mcp.CallToolResult, R, error) {
		return cachedHandler.Handle(ctx, req, input)
	}
}

// Handle implements the MCP tool handler interface with caching
func (h *CachedMCPHandler[T, R]) Handle(ctx context.Context, req *mcp.CallToolRequest, input T) (*mcp.CallToolResult, R, error) {
	// If caching is disabled, call handler directly
	if !h.enabled || h.cache == nil {
		return h.handler(ctx, req, input)
	}

	key := h.keyFunc(input)

	// Try to get from cache first
	if entry, found, err := h.cache.Get(ctx, key); err == nil && found {
		if result, ok := entry.Data.(R); ok {
			log.Printf("Cache hit for key: %s", key.String())
			return nil, result, nil
		}
	}

	// Cache miss - call the original handler
	log.Printf("Cache miss for key: %s", key.String())
	toolResult, result, err := h.handler(ctx, req, input)
	if err != nil {
		return toolResult, result, err
	}

	// Store successful result in cache
	entry := CacheEntry{
		Data: result,
		Tags: h.tags(input),
	}

	// Calculate size
	if data, marshalErr := json.Marshal(result); marshalErr == nil {
		entry.SizeBytes = int64(len(data))
	}

	if cacheErr := h.cache.Set(ctx, key, entry); cacheErr != nil {
		log.Printf("Failed to cache result for key %s: %v", key.String(), cacheErr)
	}

	return toolResult, result, nil
}

// Enable/disable caching for this handler
func (h *CachedMCPHandler[T, R]) SetEnabled(enabled bool) {
	h.enabled = enabled
}

// Cache utility functions for PR operations

// CreatePRKeyFunc creates a cache key function for PR operations
func CreatePRKeyFunc(keyType string) func(any) CacheKey {
	return func(input any) CacheKey {
		switch keyType {
		case "pr-get":
			if in, ok := input.(GetPRInput); ok {
				return NewPRCacheKey(in.ProjectKey, in.RepoSlug, in.ID, "")
			}
		case "pr-list":
			if in, ok := input.(ListPRsInput); ok {
				hash := fmt.Sprintf("%x", md5.Sum(fmt.Appendf(nil, "%s:%d:%d", in.State, in.Start, in.Limit)))
				return CacheKey{
					Type:     "pr-list",
					Resource: fmt.Sprintf("%s/%s", in.ProjectKey, in.RepoSlug),
					Hash:     hash,
				}
			}
		case "diff":
			if in, ok := input.(DiffPRInput); ok {
				return NewDiffCacheKey(in.ProjectKey, in.RepoSlug, in.ID, in.ContextLines, in.Whitespace)
			}
		case "comments":
			if in, ok := input.(ListCommentsInput); ok {
				return NewCommentsCacheKey(in.ProjectKey, in.RepoSlug, in.ID, in.Start, in.Limit)
			}
		case "activities":
			if in, ok := input.(ActivitiesInput); ok {
				return NewActivitiesCacheKey(in.ProjectKey, in.RepoSlug, in.ID, in.Start, in.Limit)
			}
		}
		return CacheKey{}
	}
}

// CreatePRTagsFunc creates a tags function for PR operations
func CreatePRTagsFunc(input any) []string {
	tags := []string{}

	switch in := input.(type) {
	case GetPRInput:
		tags = append(tags,
			fmt.Sprintf("project:%s", in.ProjectKey),
			fmt.Sprintf("repo:%s", in.RepoSlug),
			fmt.Sprintf("pr:%d", in.ID),
		)
	case ListPRsInput:
		tags = append(tags,
			fmt.Sprintf("project:%s", in.ProjectKey),
			fmt.Sprintf("repo:%s", in.RepoSlug),
		)
	case DiffPRInput:
		tags = append(tags,
			fmt.Sprintf("project:%s", in.ProjectKey),
			fmt.Sprintf("repo:%s", in.RepoSlug),
			fmt.Sprintf("pr:%d", in.ID),
		)
	case ListCommentsInput:
		tags = append(tags,
			fmt.Sprintf("project:%s", in.ProjectKey),
			fmt.Sprintf("repo:%s", in.RepoSlug),
			fmt.Sprintf("pr:%d", in.ID),
		)
	case ActivitiesInput:
		tags = append(tags,
			fmt.Sprintf("project:%s", in.ProjectKey),
			fmt.Sprintf("repo:%s", in.RepoSlug),
			fmt.Sprintf("pr:%d", in.ID),
		)
	}

	return tags
}

// CacheInvalidationHelper provides utilities for cache invalidation
type CacheInvalidationHelper struct {
	cache Cache
}

// NewCacheInvalidationHelper creates a new cache invalidation helper
func NewCacheInvalidationHelper(cache Cache) *CacheInvalidationHelper {
	return &CacheInvalidationHelper{cache: cache}
}

// InvalidatePR invalidates all cache entries for a specific PR
func (h *CacheInvalidationHelper) InvalidatePR(ctx context.Context, projectKey, repoSlug string, prID int) error {
	return h.cache.InvalidateByPattern(ctx, fmt.Sprintf("%s/%s/%d", projectKey, repoSlug, prID))
}

// InvalidateRepo invalidates all cache entries for a specific repository
func (h *CacheInvalidationHelper) InvalidateRepo(ctx context.Context, projectKey, repoSlug string) error {
	return h.cache.InvalidateByPattern(ctx, fmt.Sprintf("%s/%s", projectKey, repoSlug))
}

// InvalidateProject invalidates all cache entries for a specific project
func (h *CacheInvalidationHelper) InvalidateProject(ctx context.Context, projectKey string) error {
	tags := []string{fmt.Sprintf("project:%s", projectKey)}
	return h.cache.InvalidateByTags(ctx, tags)
}
