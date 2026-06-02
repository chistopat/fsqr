package query

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	DefaultLimit  = 10
	MinLimit      = 1
	MaxLimit      = 100
	MaxQueryRunes = 512
)

type Query struct {
	value string
	limit int
}

func New(value string, limit int) (Query, error) {
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < MinLimit || limit > MaxLimit {
		return Query{}, fmt.Errorf("search limit must be between %d and %d", MinLimit, MaxLimit)
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return Query{}, fmt.Errorf("search query must not be empty")
	}
	if utf8.RuneCountInString(value) > MaxQueryRunes {
		return Query{}, fmt.Errorf("search query must be at most %d characters", MaxQueryRunes)
	}

	return Query{
		value: value,
		limit: limit,
	}, nil
}

func (query Query) String() string {
	return query.value
}

func (query Query) Limit() int {
	return query.limit
}
