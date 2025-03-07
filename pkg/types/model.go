package types

type Model interface {
	Info() ModelInfo
	GGUFPath() string
}
