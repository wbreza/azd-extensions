package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/generator/internal"
	"gopkg.in/yaml.v3"
)

func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "build-registry",
		Short: "Build and update the extension registry",
		RunE:  buildRegistry,
	}

	paths := []string{
		"../extensions/ai",
		"../extensions/test",
		"../extensions/pack",
	}

	rootCmd.Flags().StringSliceP("paths", "p", paths, "Paths to the extensions to process")
	rootCmd.Flags().StringP("private-key", "k", "private_key.pem", "Path to the private key for signing the registry")
	rootCmd.Flags().StringP("public-key", "u", "public_key.pem", "Path to the public key for validation")
	rootCmd.Flags().StringP("registry", "r", "../registry/registry.json", "Path to the registry.json file")
	rootCmd.Flags().StringP("base-url", "b", "https://github.com/wbreza/azd-extensions/raw/main/registry/extensions", "Base URL for binary paths (e.g., https://github.com/user/repo/raw/main/registry/extensions)")

	return rootCmd
}

func buildRegistry(cmd *cobra.Command, args []string) error {
	paths, _ := cmd.Flags().GetStringSlice("paths")
	privateKeyPath, _ := cmd.Flags().GetString("private-key")
	publicKeyPath, _ := cmd.Flags().GetString("public-key")
	registryPath, _ := cmd.Flags().GetString("registry")
	baseURL, _ := cmd.Flags().GetString("base-url")

	if len(paths) == 0 {
		return fmt.Errorf("no paths provided")
	}

	if privateKeyPath == "" {
		return fmt.Errorf("private key path is required")
	}

	if publicKeyPath == "" {
		return fmt.Errorf("public key path is required")
	}

	if baseURL == "" {
		return fmt.Errorf("base URL is required")
	}

	// Load or create the registry
	var registry internal.Registry
	if _, err := os.Stat(registryPath); err == nil {
		data, err := os.ReadFile(registryPath)
		if err != nil {
			return fmt.Errorf("failed to read registry file: %v", err)
		}
		if err := json.Unmarshal(data, &registry); err != nil {
			return fmt.Errorf("failed to parse registry file: %v", err)
		}
	} else {
		registry = internal.Registry{}
	}

	// Process each extension
	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("failed to resolve absolute path for %s: %v", path, err)
		}

		if err := processExtension(absPath, baseURL, &registry); err != nil {
			return fmt.Errorf("failed to process extension at %s: %v", absPath, err)
		}
	}

	// Save the updated registry without a signature
	if err := saveRegistry(registryPath, &registry); err != nil {
		return fmt.Errorf("failed to save registry: %v", err)
	}

	signatureManager, err := internal.NewSignatureManagerFromFiles(privateKeyPath, publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create signature manager: %v", err)
	}

	// Sign the registry
	// Saves the registry with the signature
	if err := signRegistry(registryPath, signatureManager); err != nil {
		return fmt.Errorf("failed to sign registry: %v", err)
	}

	// Validate the registry using the public key
	if err := validateRegistrySignature(registryPath, signatureManager); err != nil {
		return fmt.Errorf("public key validation failed: %v", err)
	}

	fmt.Println("Registry updated, signed, and validated successfully.")
	return nil
}

func processExtension(path, baseURL string, registry *internal.Registry) error {
	// Load metadata
	metadataPath := filepath.Join(path, "extension.yaml")
	metadataData, err := os.ReadFile(metadataPath)
	if err != nil {
		return fmt.Errorf("failed to read metadata: %v", err)
	}

	var schema internal.ExtensionSchema
	if err := yaml.Unmarshal(metadataData, &schema); err != nil {
		return fmt.Errorf("failed to parse metadata: %v", err)
	}

	// Build the binaries
	buildScript := filepath.Join(path, "build.sh")
	if _, err := os.Stat(buildScript); err == nil {
		cmd := exec.Command("bash", "build.sh")
		cmd.Dir = path
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to build binaries: %s", string(output))
		}
	}

	// Prepare binaries for registry
	binariesPath := filepath.Join(path, "bin")
	binaries, err := os.ReadDir(binariesPath)
	binaryMap := map[string]internal.ExtensionBinary{}
	if err == nil {
		registryBasePath := "../registry/extensions"
		targetPath := filepath.Join(registryBasePath, schema.Id, schema.Version)

		// Ensure target directory exists
		if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create target directory: %v", err)
		}

		// Map and copy binaries
		for _, binary := range binaries {
			sourcePath := filepath.Join(binariesPath, binary.Name())
			targetFilePath := filepath.Join(targetPath, binary.Name())

			// Copy the binary to the registry folder
			if err := internal.CopyFile(sourcePath, targetFilePath); err != nil {
				return fmt.Errorf("failed to copy binary %s: %v", binary.Name(), err)
			}

			// Generate checksum
			checksum, err := internal.ComputeChecksum(targetFilePath)
			if err != nil {
				return fmt.Errorf("failed to compute checksum for %s: %v", targetFilePath, err)
			}

			// Parse binary filename to infer OS/ARCH
			osArch, err := inferOSArch(binary.Name())
			if err != nil {
				return fmt.Errorf("failed to infer OS/ARCH for binary %s: %v", binary.Name(), err)
			}

			// Generate URL for the binary using the base URL
			url := fmt.Sprintf("%s/%s/%s/%s", baseURL, schema.Id, schema.Version, binary.Name())

			// Add binary to the map with OS/ARCH key
			binaryMap[osArch] = internal.ExtensionBinary{
				URL: url,
				Checksum: struct {
					Algorithm string `json:"algorithm"`
					Value     string `json:"value"`
				}{
					Algorithm: "sha256",
					Value:     checksum,
				},
			}
		}
	}

	// Add or update the extension in the registry
	addOrUpdateExtension(schema, schema.Version, binaryMap, registry)
	return nil
}

func inferOSArch(filename string) (string, error) {
	// Example filename: azd-ext-ai-windows-amd64.exe
	parts := strings.Split(filename, "-")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid binary filename format: %s", filename)
	}

	// Extract OS and ARCH from the filename
	osPart := parts[len(parts)-2]                                   // Second-to-last part is the OS
	archPart := parts[len(parts)-1]                                 // Last part is the ARCH (with optional extension)
	archPart = strings.TrimSuffix(archPart, filepath.Ext(archPart)) // Remove extension

	return fmt.Sprintf("%s/%s", osPart, archPart), nil
}

func addOrUpdateExtension(schema internal.ExtensionSchema, version string, binaries map[string]internal.ExtensionBinary, registry *internal.Registry) {
	// Find or create the extension in the registry
	var ext *internal.ExtensionMetadata
	for i := range registry.Extensions {
		if registry.Extensions[i].Id == schema.Id {
			ext = registry.Extensions[i]
			break
		}
	}

	// If the extension doesn't exist, add it
	if ext == nil {
		ext = &internal.ExtensionMetadata{
			Versions: []internal.ExtensionVersion{},
		}

		registry.Extensions = append(registry.Extensions, ext)
	}

	ext.Id = schema.Id
	ext.Namespace = schema.Namespace
	ext.DisplayName = schema.DisplayName
	ext.Description = schema.Description
	ext.Tags = schema.Tags

	// Check if the version already exists and update it if found
	for i, v := range ext.Versions {
		if v.Version == version {
			ext.Versions[i] = internal.ExtensionVersion{
				Version:      version,
				Usage:        schema.Usage,
				Examples:     schema.Examples,
				Dependencies: schema.Dependencies,
				Binaries:     binaries,
			}
			fmt.Printf("Updated version %s for extension %s\n", version, schema.Id)
			return
		}
	}

	// If the version does not exist, add it as a new entry
	ext.Versions = append(ext.Versions, internal.ExtensionVersion{
		Version:      version,
		Usage:        schema.Usage,
		Examples:     schema.Examples,
		Dependencies: schema.Dependencies,
		Binaries:     binaries,
	})
	fmt.Printf("Added new version %s for extension %s\n", version, schema.Id)
}

func saveRegistry(path string, registry *internal.Registry) error {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func signRegistry(registryPath string, signatureManager *internal.SignatureManager) error {
	// Load the registry
	registryBytes, err := os.ReadFile(registryPath)
	if err != nil {
		return fmt.Errorf("failed to read registry: %v", err)
	}

	// Unmarshal to extract signature and registry content
	var registry *internal.Registry
	if err := json.Unmarshal(registryBytes, &registry); err != nil {
		return fmt.Errorf("failed to parse registry: %v", err)
	}

	// Clear any present signature
	registry.Signature = ""
	rawRegistryBytes, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal registry: %v", err)
	}

	// Sign the registry content
	signature, err := signatureManager.Sign(rawRegistryBytes)
	if err != nil {
		return fmt.Errorf("failed to sign registry: %v", err)
	}

	// Update the registry with the signature
	registry.Signature = signature
	if err := saveRegistry(registryPath, registry); err != nil {
		return fmt.Errorf("failed to save registry: %v", err)
	}

	return nil
}

func validateRegistrySignature(registryPath string, signatureManager *internal.SignatureManager) error {
	// Load the registry
	registryBytes, err := os.ReadFile(registryPath)
	if err != nil {
		return fmt.Errorf("failed to read registry: %w", err)
	}

	// Unmarshal to extract signature and registry content
	var registry *internal.Registry
	if err := json.Unmarshal(registryBytes, &registry); err != nil {
		return fmt.Errorf("failed to parse registry: %w", err)
	}

	// Store the signature and remove it from the registry
	signature := registry.Signature
	registry.Signature = ""

	// Marshal the registry content to verify the signature
	rawRegistry, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal registry: %w", err)
	}

	if err := signatureManager.Verify(rawRegistry, signature); err != nil {
		return fmt.Errorf("failed to verify registry signature: %w", err)
	}

	return nil
}
