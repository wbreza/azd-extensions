package internal

// ExtensionMetadata represents the structure of extension.yaml
type ExtensionMetadata struct {
	Name        string                     `yaml:"name" json:"name"`
	DisplayName string                     `yaml:"displayName" json:"displayName"`
	Description string                     `yaml:"description" json:"description"`
	Usage       string                     `yaml:"usage" json:"usage"`
	Examples    []ExtensionMetadataExample `yaml:"examples" json:"examples"`
}

type ExtensionMetadataExample struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Usage       string `yaml:"usage" json:"usage"`
}

// Registry represents the registry.json structure
type Registry struct {
	Extensions []Extension `json:"extensions"`
	Signature  string      `json:"signature,omitempty"`
}

// Extension represents an extension in the registry
type Extension struct {
	Name        string             `json:"name"`
	DisplayName string             `json:"displayName"`
	Description string             `json:"description"`
	Versions    []ExtensionVersion `json:"versions"`
}

// ExtensionVersion represents a version of an extension
type ExtensionVersion struct {
	Version  string                     `json:"version"`
	Usage    string                     `json:"usage"`
	Examples []ExtensionMetadataExample `json:"examples"`
	Binaries map[string]ExtensionBinary `json:"binaries"`
}

// ExtensionBinary represents the binary information of an extension
type ExtensionBinary struct {
	URL      string `json:"url"`
	Checksum struct {
		Algorithm string `json:"algorithm"`
		Value     string `json:"value"`
	} `json:"checksum"`
}
