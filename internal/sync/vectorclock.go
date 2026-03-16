package sync

import "encoding/json"

// VectorClock отслеживает причинно-следственные связи изменений между узлами.
// Ключ — nodeID, значение — счётчик изменений на этом узле.
type VectorClock map[string]uint64

func NewVectorClock() VectorClock {
	return make(VectorClock)
}

// Increment увеличивает счётчик для данного узла
func (vc VectorClock) Increment(nodeID string) VectorClock {
	next := vc.Clone()
	next[nodeID]++
	return next
}

// Clone создаёт копию
func (vc VectorClock) Clone() VectorClock {
	c := make(VectorClock, len(vc))
	for k, v := range vc {
		c[k] = v
	}
	return c
}

// Merge объединяет два clock, беря максимум по каждому узлу
func (vc VectorClock) Merge(other VectorClock) VectorClock {
	result := vc.Clone()
	for k, v := range other {
		if result[k] < v {
			result[k] = v
		}
	}
	return result
}

// Relation описывает отношение между двумя clock
type Relation int

const (
	Before     Relation = iota // vc < other (vc устарел)
	After                      // vc > other (other устарел)
	Equal                      // vc == other
	Concurrent                 // конфликт — изменения параллельны
)

// Compare сравнивает два VectorClock
func (vc VectorClock) Compare(other VectorClock) Relation {
	vcGT, otherGT := false, false

	keys := make(map[string]struct{})
	for k := range vc {
		keys[k] = struct{}{}
	}
	for k := range other {
		keys[k] = struct{}{}
	}

	for k := range keys {
		a, b := vc[k], other[k]
		if a > b {
			vcGT = true
		} else if b > a {
			otherGT = true
		}
	}

	switch {
	case !vcGT && !otherGT:
		return Equal
	case vcGT && !otherGT:
		return After
	case !vcGT && otherGT:
		return Before
	default:
		return Concurrent
	}
}

func (vc VectorClock) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]uint64(vc))
}

func (vc *VectorClock) UnmarshalJSON(data []byte) error {
	m := make(map[string]uint64)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*vc = VectorClock(m)
	return nil
}
