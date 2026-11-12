package app

import (
	"testing"

	"github.com/Han8931/gorae/internal/meta"
)

// :index skips work by comparing recorded state against the file on disk. The
// case that matters here is the one file size alone cannot see: the config's
// embedding_model changed, so stored vectors have the wrong width and must be
// rebuilt even though the document is untouched.
func TestVectorsUpToDate(t *testing.T) {
	const model = "nomic-embed-text"

	tests := []struct {
		name     string
		state    *meta.VecIndexState
		fileSize int64
		embModel string
		want     bool
	}{
		{
			name:     "never embedded",
			state:    nil,
			fileSize: 100,
			embModel: model,
			want:     false,
		},
		{
			name:     "same file, same model",
			state:    &meta.VecIndexState{FileSize: 100, EmbeddingModel: model},
			fileSize: 100,
			embModel: model,
			want:     true,
		},
		{
			name:     "file changed",
			state:    &meta.VecIndexState{FileSize: 100, EmbeddingModel: model},
			fileSize: 250,
			embModel: model,
			want:     false,
		},
		{
			name:     "model changed, file untouched",
			state:    &meta.VecIndexState{FileSize: 100, EmbeddingModel: model},
			fileSize: 100,
			embModel: "mxbai-embed-large",
			want:     false,
		},
		{
			name:     "vectors predate provenance tracking",
			state:    &meta.VecIndexState{FileSize: 100, EmbeddingModel: ""},
			fileSize: 100,
			embModel: model,
			want:     false,
		},
		{
			name:     "both file and model changed",
			state:    &meta.VecIndexState{FileSize: 100, EmbeddingModel: model},
			fileSize: 250,
			embModel: "mxbai-embed-large",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vectorsUpToDate(tt.state, tt.fileSize, tt.embModel); got != tt.want {
				t.Errorf("vectorsUpToDate() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The re-embed counter exists to explain a surprising :index run, so it must
// fire only for the model-change case and not for ordinary edits.
func TestVectorsStaleFromModelChange(t *testing.T) {
	const model = "nomic-embed-text"

	tests := []struct {
		name     string
		state    *meta.VecIndexState
		fileSize int64
		embModel string
		want     bool
	}{
		{
			name:     "model changed, file untouched",
			state:    &meta.VecIndexState{FileSize: 100, EmbeddingModel: model},
			fileSize: 100,
			embModel: "mxbai-embed-large",
			want:     true,
		},
		{
			name:     "unknown provenance counts as a model change",
			state:    &meta.VecIndexState{FileSize: 100, EmbeddingModel: ""},
			fileSize: 100,
			embModel: model,
			want:     true,
		},
		{
			name:     "nothing changed",
			state:    &meta.VecIndexState{FileSize: 100, EmbeddingModel: model},
			fileSize: 100,
			embModel: model,
			want:     false,
		},
		{
			name:     "file edited is ordinary re-indexing, not a model change",
			state:    &meta.VecIndexState{FileSize: 100, EmbeddingModel: model},
			fileSize: 250,
			embModel: model,
			want:     false,
		},
		{
			name:     "never embedded is a first index, not a model change",
			state:    nil,
			fileSize: 100,
			embModel: model,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vectorsStaleFromModelChange(tt.state, tt.fileSize, tt.embModel); got != tt.want {
				t.Errorf("vectorsStaleFromModelChange() = %v, want %v", got, tt.want)
			}
		})
	}
}
