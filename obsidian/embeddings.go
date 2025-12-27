package obsidian

import (
	"cmp"
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/victhorio/opa/agg/core"
	"github.com/victhorio/opa/agg/embeddings"
)

// TODO(refactor): Let's eventually move to leveraging some proper vector DB. For now, we do a
// simple in-memory, in-Go computation which works well enough for a relatively small sized vault.

const (
	opaDirName       = ".opa"
	cacheFileName    = "embeddings.gob"
	cacheVersion     = "1"
	embeddingBatchSize = 100
)

// embeddingEntry represents a single cached embedding with its content hash.
type embeddingEntry struct {
	NoteName    string
	ContentHash string
	Embedding   []float64
}

// embeddingsCache represents the complete cache file structure.
type embeddingsCache struct {
	Version string
	Model   string
	Entries []embeddingEntry
}

type embedIdx struct {
	embedder core.Embedder
	embeds   map[string][]float64
}

// getCachePath returns the full path to the embeddings cache file.
func (v *Vault) getCachePath() string {
	return filepath.Join(v.rootDir, opaDirName, cacheFileName)
}

// loadEmbeddingsCache loads the cached embeddings from disk.
// Returns nil cache (not error) if file doesn't exist or is corrupted.
func (v *Vault) loadEmbeddingsCache() (*embeddingsCache, error) {
	cachePath := v.getCachePath()

	f, err := os.Open(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open cache file: %w", err)
	}
	defer f.Close()

	var cache embeddingsCache
	decoder := gob.NewDecoder(f)
	if err := decoder.Decode(&cache); err != nil {
		log.Printf("warning: failed to decode embeddings cache, will rebuild: %v", err)
		return nil, nil
	}

	if cache.Version != cacheVersion {
		log.Printf("cache version mismatch (have %s, want %s), will rebuild", cache.Version, cacheVersion)
		return nil, nil
	}

	return &cache, nil
}

// saveEmbeddingsCache persists the cache to disk.
// Creates the .opa directory if it doesn't exist.
func (v *Vault) saveEmbeddingsCache(cache *embeddingsCache) error {
	opaDir := filepath.Join(v.rootDir, opaDirName)
	if err := os.MkdirAll(opaDir, 0755); err != nil {
		return fmt.Errorf("failed to create .opa directory: %w", err)
	}

	cachePath := v.getCachePath()

	// Write to temp file first, then rename for atomicity.
	tmpPath := cachePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp cache file: %w", err)
	}

	encoder := gob.NewEncoder(f)
	if err := encoder.Encode(cache); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to encode cache: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp cache file: %w", err)
	}

	if err := os.Rename(tmpPath, cachePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp cache file: %w", err)
	}

	return nil
}

func (v *Vault) RefreshEmbeddings() error {
	// TODO(correctness): accept a context here

	// TODO: change this to OpenAILarge, it's cheap enough - no reason not to
	embedder, err := embeddings.NewOpenAIEmbedder(embeddings.OpenAISmall, nil)
	if err != nil {
		return fmt.Errorf("failed to create embedder: %w", err)
	}

	e := &embedIdx{
		embedder: embedder,
		embeds:   make(map[string][]float64),
	}

	// Load existing cache.
	cache, err := v.loadEmbeddingsCache()
	if err != nil {
		return fmt.Errorf("failed to load embeddings cache: %w", err)
	}

	// Build lookup map from cached entries.
	cachedByName := make(map[string]embeddingEntry)
	if cache != nil {
		currentModel := string(embeddings.OpenAISmall)
		if cache.Model != currentModel {
			log.Printf("embedding model changed (%s -> %s), rebuilding all embeddings", cache.Model, currentModel)
			cache = nil
		} else {
			for _, entry := range cache.Entries {
				cachedByName[entry.NoteName] = entry
			}
		}
	}

	// Determine which notes need embedding.
	var notesToEmbed []string
	var contentsToEmbed []string

	for noteName, note := range v.idx.notes {
		cachedEntry, exists := cachedByName[noteName]

		if exists && cachedEntry.ContentHash == note.contentHash {
			// Cache hit - use existing embedding.
			e.embeds[noteName] = cachedEntry.Embedding
			continue
		}

		// Cache miss - need to compute embedding.
		content, err := v.ReadNote(noteName)
		if err != nil {
			return fmt.Errorf("failed to read note %s: %w", noteName, err)
		}
		notesToEmbed = append(notesToEmbed, noteName)
		contentsToEmbed = append(contentsToEmbed, content)
	}

	// Compute embeddings for new/modified notes (if any).
	if len(notesToEmbed) > 0 {
		log.Printf("computing embeddings for %d notes (%d cached)", len(notesToEmbed), len(e.embeds))

		result, err := v.embedInBatches(context.Background(), embedder, contentsToEmbed)
		if err != nil {
			return fmt.Errorf("failed to embed contents: %w", err)
		}
		log.Printf("embedded %d notes, cost: $%.4f", len(notesToEmbed), float64(result.Cost)/1_000_000_000)

		for i, noteName := range notesToEmbed {
			e.embeds[noteName] = result.Vectors[i]
		}
	} else {
		log.Printf("all %d embeddings loaded from cache", len(e.embeds))
	}

	// Build new cache with all current embeddings.
	newCache := &embeddingsCache{
		Version: cacheVersion,
		Model:   string(embeddings.OpenAISmall),
		Entries: make([]embeddingEntry, 0, len(v.idx.notes)),
	}

	for noteName, note := range v.idx.notes {
		newCache.Entries = append(newCache.Entries, embeddingEntry{
			NoteName:    noteName,
			ContentHash: note.contentHash,
			Embedding:   e.embeds[noteName],
		})
	}

	// Save updated cache.
	if err := v.saveEmbeddingsCache(newCache); err != nil {
		log.Printf("warning: failed to save embeddings cache: %v", err)
	}

	v.idx.embeds = e
	return nil
}

// embedInBatches splits a large embedding request into smaller batches to avoid API limits.
func (v *Vault) embedInBatches(ctx context.Context, embedder core.Embedder, contents []string) (*core.EmbeddingsResult, error) {
	if len(contents) <= embeddingBatchSize {
		return embedder.Embed(ctx, contents, nil)
	}

	allVectors := make([][]float64, len(contents))
	var totalCost int64

	for i := 0; i < len(contents); i += embeddingBatchSize {
		end := i + embeddingBatchSize
		if end > len(contents) {
			end = len(contents)
		}

		batch := contents[i:end]
		log.Printf("embedding batch %d-%d of %d", i+1, end, len(contents))

		result, err := embedder.Embed(ctx, batch, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to embed batch %d-%d: %w", i, end, err)
		}

		for j, vec := range result.Vectors {
			allVectors[i+j] = vec
		}
		totalCost += result.Cost
	}

	return &core.EmbeddingsResult{
		Vectors: allVectors,
		Cost:    totalCost,
	}, nil
}

func (v *Vault) SemanticSearch(query string, k int) ([]SemanticMatch, error) {
	// TODO(correctness): accept a context here

	if v.idx.embeds == nil {
		return nil, fmt.Errorf("embeddings not computed")
	}

	qResult, err := v.idx.embeds.embedder.Embed(context.Background(), []string{query}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}
	qEmbed := qResult.Vectors[0]

	topNotes := make([]SemanticMatch, 0, k)

	for name, embed := range v.idx.embeds.embeds {
		// We assume embeddings are unit vectors (guaranteed by OpenAI) so that the dot product is
		// already the cosine similarity.
		score := dotProduct(qEmbed, embed)

		if len(topNotes) < k {
			topNotes = append(topNotes, SemanticMatch{Name: name, Score: score})

			// Let's keep the topNotes ordered in descending order of score.
			slices.SortFunc(
				topNotes,
				func(a, b SemanticMatch) int {
					return cmp.Compare(b.Score, a.Score)
				},
			)

			continue
		}

		if score > topNotes[k-1].Score {
			topNotes[k-1] = SemanticMatch{Name: name, Score: score}

			slices.SortFunc(
				topNotes,
				func(a, b SemanticMatch) int {
					return cmp.Compare(b.Score, a.Score)
				},
			)
		}
	}

	return topNotes, nil
}

type SemanticMatch struct {
	Name  string
	Score float64
}

func dotProduct(a, b []float64) float64 {
	var dotProduct float64
	for i := range a {
		dotProduct += a[i] * b[i]
	}
	return dotProduct
}
