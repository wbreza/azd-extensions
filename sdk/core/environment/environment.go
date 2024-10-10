package environment

type Environment struct {
	Name string `yaml:"name"`
}

func Load() (*Environment, error) {
	return &Environment{
		Name: "My Environment",
	}, nil
}
