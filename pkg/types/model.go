package types

type Model interface {
	ID() (string, error)
	GGUFPath() (string, error)
	Config() (Config, error)
	Tags() []string
}
