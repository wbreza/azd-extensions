package internal

// ExtensionMetadata represents the structure of extension.yaml
type ExtensionSchema struct {
	Id           string                `yaml:"id" json:"id"`
	Namespace    string                `yaml:"namespace" json:"namespace,omitempty"`
	Version      string                `yaml:"version" json:"version"`
	DisplayName  string                `yaml:"displayName" json:"displayName"`
	Description  string                `yaml:"description" json:"description"`
	Usage        string                `yaml:"usage" json:"usage"`
	Examples     []ExtensionExample    `yaml:"examples" json:"examples"`
	Tags         []string              `yaml:"tags" json:"tags,omitempty"`
	Dependencies []ExtensionDependency `yaml:"dependencies" json:"dependencies,omitempty"`
}

type ExtensionExample struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Usage       string `json:"usage"`
}

// Registry represents the registry.json structure
type Registry struct {
	Extensions []*ExtensionMetadata `json:"extensions"`
	Signature  string               `json:"signature,omitempty"`
}

// Extension represents an extension in the registry
type ExtensionMetadata struct {
	Id          string             `json:"id"`
	Namespace   string             `json:"namespace,omitempty"`
	DisplayName string             `json:"displayName"`
	Description string             `json:"description"`
	Versions    []ExtensionVersion `json:"versions"`
	Source      string             `json:"source,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
}

// ExtensionDependency represents a dependency of an extension
type ExtensionDependency struct {
	Id      string `json:"id"`
	Version string `json:"version,omitempty"`
}

// ExtensionVersion represents a version of an extension
type ExtensionVersion struct {
	Version      string                     `json:"version"`
	Usage        string                     `json:"usage"`
	Examples     []ExtensionExample         `json:"examples"`
	Binaries     map[string]ExtensionBinary `json:"binaries,omitempty"`
	Dependencies []ExtensionDependency      `json:"dependencies,omitempty"`
}

// ExtensionBinary represents the binary information of an extension
type ExtensionBinary struct {
	URL      string            `json:"url"`
	Checksum ExtensionChecksum `json:"checksum"`
}

type ExtensionChecksum struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}
