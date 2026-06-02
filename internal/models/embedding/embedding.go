package embedding

import "fmt"

const Dimensions = 384

type Embedding struct {
	values []float32
}

func New(values []float32) (Embedding, error) {
	if len(values) != Dimensions {
		return Embedding{}, fmt.Errorf("embedding must have %d dimensions", Dimensions)
	}

	copied := make([]float32, len(values))
	copy(copied, values)

	return Embedding{values: copied}, nil
}

func (embedding Embedding) Values() []float32 {
	copied := make([]float32, len(embedding.values))
	copy(copied, embedding.values)

	return copied
}

func (embedding Embedding) Len() int {
	return len(embedding.values)
}
