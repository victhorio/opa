package obsidian

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"slices"

	"github.com/victhorio/opa/agg/core"
	"github.com/victhorio/opa/agg/embeddings"
)

// TODO(refactor): Let's eventually move to leveraging some proper vector DB. For now, we do a
// simple in-memory, in-Go computation which works well enough for a relatively small sized vault.

type embedIdx struct {
	embedder core.Embedder
	embeds   map[string][]float64
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

	names := make([]string, 0, len(v.idx.notes))
	contents := make([]string, 0, len(v.idx.notes))

	for name := range v.idx.notes {
		content, err := v.ReadNote(name)
		if err != nil {
			return fmt.Errorf("failed to read note %s: %w", name, err)
		}
		names = append(names, name)
		contents = append(contents, content)
	}

	log.Printf("embedding %d notes", len(names))
	result, err := embedder.Embed(context.Background(), contents, nil)
	if err != nil {
		return fmt.Errorf("failed to embed contents: %w", err)
	}
	log.Printf("embedded %d notes, cost: $%.4f", len(names), float64(result.Cost)/1_000_000_000)

	for i, name := range names {
		e.embeds[name] = result.Vectors[i]
	}

	v.idx.embeds = e
	return nil
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
