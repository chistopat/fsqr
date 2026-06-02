package level

import "fmt"

const (
	Min = 1
	Max = 6
)

type Level int

func New(value int) (Level, error) {
	if value < Min || value > Max {
		return 0, fmt.Errorf("category level must be between %d and %d", Min, Max)
	}

	return Level(value), nil
}

func (level Level) Int() int {
	return int(level)
}

func (level Level) Normalized() float64 {
	return float64(level-Min) / float64(Max-Min)
}
