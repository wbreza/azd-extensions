package project

type Project struct {
	Name string `yaml:"name"`
}

func Load() (*Project, error) {
	return &Project{
		Name: "My Project",
	}, nil
}
