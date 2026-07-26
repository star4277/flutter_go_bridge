package sample

type Value struct {
	Name string `json:"name"`
}

type Ref struct {
	value int
}

type State int32

const (
	StateUnknown State = iota
	StateReady
)

// flutter_go_bridge:dart_name=calculate
func Add(a, b int) int { return a + b }

func Echo(value Value) Value { return value }

func NewRef(value int) *Ref { return &Ref{value: value} }

func (ref *Ref) Value() int { return ref.value }

// fgb:ignore
func Ignored() {}
