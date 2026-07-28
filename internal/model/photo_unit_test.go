package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhotoTableName(t *testing.T) {
	p := Photo{}
	assert.Equal(t, "photos", p.TableName())
}

func TestPhotoEmbeddingTableName(t *testing.T) {
	e := PhotoEmbedding{}
	assert.Equal(t, "photo_embeddings", e.TableName())
}
