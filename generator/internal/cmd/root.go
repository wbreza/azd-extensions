package cmd

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dario.cat/mergo"
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
	rootCmd.Flags().StringP("base-url", "b", "https://github.com/wbreza/azd-extensions/raw/main/registry/extensions", "Base URL for artifact paths (e.g., https://github.com/user/repo/raw/main/registry/extensions)")

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

	// Build the artifacts
	buildScript := filepath.Join(path, "build.sh")
	if _, err := os.Stat(buildScript); err == nil {
		cmd := exec.Command("bash", "build.sh")
		cmd.Dir = path
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to build artifacts: %s", string(output))
		}
	}

	// Prepare artifacts for registry
	artifactsPath := filepath.Join(path, "bin")
	artifacts, err := os.ReadDir(artifactsPath)
	artifactMap := map[string]internal.ExtensionArtifact{}
	if err == nil {
		registryBasePath := "../registry/extensions"
		targetPath := filepath.Join(registryBasePath, schema.Id, schema.Version)

		// Ensure target directory exists
		if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create target directory: %v", err)
		}

		// Map and copy artifacts
		for _, artifact := range artifacts {
			sourcePath := filepath.Join(artifactsPath, artifact.Name())

			fileWithoutExt := getFileNameWithoutExt(artifact.Name())
			zipFileName := fmt.Sprintf("%s.zip", fileWithoutExt)
			targetFilePath := filepath.Join(targetPath, zipFileName)

			// Create a ZIP archive for the artifact
			if err := zipSource(sourcePath, targetFilePath); err != nil {
				return fmt.Errorf("failed to create archive for %s: %v", artifact.Name(), err)
			}

			// Generate checksum
			checksum, err := internal.ComputeChecksum(targetFilePath)
			if err != nil {
				return fmt.Errorf("failed to compute checksum for %s: %v", targetFilePath, err)
			}

			// Parse artifact filename to infer OS/ARCH
			osArch, err := inferOSArch(artifact.Name())
			if err != nil {
				return fmt.Errorf("failed to infer OS/ARCH for artifact %s: %v", artifact.Name(), err)
			}

			// Generate URL for the artifact using the base URL
			url := fmt.Sprintf("%s/%s/%s/%s", baseURL, schema.Id, schema.Version, filepath.Base(targetFilePath))

			platformMetadata := map[string]any{}
			operatingSystems := []string{"windows", "linux", "darwin"}
			architectures := []string{"amd64", "arm64"}

			for _, os := range operatingSystems {
				if err := mergo.Merge(&platformMetadata, schema.Platforms[os]); err != nil {
					return fmt.Errorf("failed to merge os metadata: %v", err)
				}
			}

			for _, arch := range architectures {
				if err := mergo.Merge(&platformMetadata, schema.Platforms[arch]); err != nil {
					return fmt.Errorf("failed to merge architecture metadata: %v", err)
				}
			}

			if err := mergo.Merge(&platformMetadata, schema.Platforms[osArch]); err != nil {
				return fmt.Errorf("failed to merge os/arch metadata: %v", err)
			}

			// Add artifact to the map with OS/ARCH key
			artifactMap[osArch] = internal.ExtensionArtifact{
				URL: url,
				Checksum: struct {
					Algorithm string `json:"algorithm"`
					Value     string `json:"value"`
				}{
					Algorithm: "sha256",
					Value:     checksum,
				},
				AdditionalMetadata: platformMetadata,
			}
		}
	}

	// Add or update the extension in the registry
	addOrUpdateExtension(schema, schema.Version, artifactMap, registry)
	return nil
}

func inferOSArch(filename string) (string, error) {
	// Example filename: azd-ext-ai-windows-amd64.exe
	parts := strings.Split(filename, "-")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid artifact filename format: %s", filename)
	}

	// Extract OS and ARCH from the filename
	osPart := parts[len(parts)-2]                                   // Second-to-last part is the OS
	archPart := parts[len(parts)-1]                                 // Last part is the ARCH (with optional extension)
	archPart = strings.TrimSuffix(archPart, filepath.Ext(archPart)) // Remove extension

	return fmt.Sprintf("%s/%s", osPart, archPart), nil
}

func addOrUpdateExtension(schema internal.ExtensionSchema, version string, artifacts map[string]internal.ExtensionArtifact, registry *internal.Registry) {
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
				EntryPoint:   schema.EntryPoint,
				Usage:        schema.Usage,
				Examples:     schema.Examples,
				Dependencies: schema.Dependencies,
				Artifacts:    artifacts,
			}
			fmt.Printf("Updated version %s for extension %s\n", version, schema.Id)
			return
		}
	}

	// If the version does not exist, add it as a new entry
	ext.Versions = append(ext.Versions, internal.ExtensionVersion{
		Version:      version,
		EntryPoint:   schema.EntryPoint,
		Usage:        schema.Usage,
		Examples:     schema.Examples,
		Dependencies: schema.Dependencies,
		Artifacts:    artifacts,
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

func zipSource(source, target string) error {
	outputFile, err := os.Create(target)
	if err != nil {
		return err
	}

	defer outputFile.Close()

	fileInfo, err := os.Stat(source)
	if err != nil {
		return err
	}

	zipWriter := zip.NewWriter(outputFile)
	defer zipWriter.Close()

	header := &zip.FileHeader{
		Name:     filepath.Base(source),
		Modified: fileInfo.ModTime(),
		Method:   zip.Deflate,
	}

	headerWriter, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}

	file, err := os.Open(source)
	if err != nil {
		return err
	}

	_, err = io.Copy(headerWriter, file)
	if err != nil {
		return err
	}

	return nil
}

// getFileNameWithoutExt extracts the filename without its extension
func getFileNameWithoutExt(filePath string) string {
	// Get the base filename
	fileName := filepath.Base(filePath)

	// Remove the extension
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}
