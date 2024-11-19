package cmd

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
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

	rootCmd.Flags().StringSliceP("paths", "p", []string{"../extensions/ai", "../extensions/test"}, "Paths to the extensions to process")
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

	// Save the updated registry
	registry.Signature = ""
	if err := saveRegistry(registryPath, &registry); err != nil {
		return fmt.Errorf("failed to save registry: %v", err)
	}

	// Sign the registry
	if err := signRegistry(registryPath, privateKeyPath); err != nil {
		return fmt.Errorf("failed to sign registry: %v", err)
	}

	// Validate the registry using the public key
	if err := validateRegistrySignature(registryPath, publicKeyPath); err != nil {
		return fmt.Errorf("public key validation failed: %v", err)
	}

	fmt.Println("Registry updated, signed, and validated successfully.")
	return nil
}

func processExtension(path, baseURL string, registry *internal.Registry) error {
	// Ensure required files exist
	requiredFiles := []string{"version.txt", "build.sh", "extension.yaml"}
	for _, file := range requiredFiles {
		fullFilePath := filepath.Join(path, file)
		if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
			return fmt.Errorf("missing required file: %s", fullFilePath)
		}
	}

	// Build the binaries
	buildScript := filepath.Join(path, "build.sh")
	cmd := exec.Command("bash", buildScript)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build binaries: %s", string(output))
	}

	// Load metadata
	metadataPath := filepath.Join(path, "extension.yaml")
	metadataData, err := os.ReadFile(metadataPath)
	if err != nil {
		return fmt.Errorf("failed to read metadata: %v", err)
	}

	var metadata internal.ExtensionMetadata
	if err := yaml.Unmarshal(metadataData, &metadata); err != nil {
		return fmt.Errorf("failed to parse metadata: %v", err)
	}

	// Load version
	versionPath := filepath.Join(path, "version.txt")
	versionData, err := os.ReadFile(versionPath)
	if err != nil {
		return fmt.Errorf("failed to read version: %v", err)
	}
	version := string(versionData)

	// Prepare binaries for registry
	binariesPath := filepath.Join(path, "bin")
	binaries, err := os.ReadDir(binariesPath)
	if err != nil {
		return fmt.Errorf("failed to list binaries: %v", err)
	}

	registryBasePath := "../registry/extensions"
	targetPath := filepath.Join(registryBasePath, metadata.Name, version)

	// Ensure target directory exists
	if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create target directory: %v", err)
	}

	// Map and copy binaries
	binaryMap := map[string]internal.ExtensionBinary{}
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
		url := fmt.Sprintf("%s/%s/%s/%s", baseURL, metadata.Name, version, binary.Name())

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

	// Add or update the extension in the registry
	addOrUpdateExtension(metadata, version, binaryMap, registry)
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

func addOrUpdateExtension(metadata internal.ExtensionMetadata, version string, binaries map[string]internal.ExtensionBinary, registry *internal.Registry) {
	// Find or create the extension in the registry
	var ext *internal.Extension
	for i := range registry.Extensions {
		if registry.Extensions[i].Name == metadata.Name {
			ext = &registry.Extensions[i]
			break
		}
	}

	// If the extension doesn't exist, add it
	if ext == nil {
		registry.Extensions = append(registry.Extensions, internal.Extension{
			Name:        metadata.Name,
			DisplayName: metadata.DisplayName,
			Description: metadata.Description,
			Versions:    []internal.ExtensionVersion{},
		})
		ext = &registry.Extensions[len(registry.Extensions)-1]
	}

	// Check if the version already exists and update it if found
	for i, v := range ext.Versions {
		if v.Version == version {
			ext.Versions[i] = internal.ExtensionVersion{
				Version:  version,
				Usage:    metadata.Usage,
				Examples: metadata.Examples,
				Binaries: binaries,
			}
			fmt.Printf("Updated version %s for extension %s\n", version, metadata.Name)
			return
		}
	}

	// If the version does not exist, add it as a new entry
	ext.Versions = append(ext.Versions, internal.ExtensionVersion{
		Version:  version,
		Usage:    metadata.Usage,
		Examples: metadata.Examples,
		Binaries: binaries,
	})
	fmt.Printf("Added new version %s for extension %s\n", version, metadata.Name)
}

func saveRegistry(path string, registry *internal.Registry) error {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing private key")
	}

	// Try parsing as PKCS#1
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Try parsing as PKCS#8
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Ensure it's an RSA private key
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA private key")
	}

	return rsaKey, nil
}

func signRegistry(registryPath, privateKeyPath string) error {
	// Read the registry file
	registryData, err := os.ReadFile(registryPath)
	if err != nil {
		return fmt.Errorf("failed to read registry file: %v", err)
	}

	// Read the private key
	privateKey, err := loadPrivateKey(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %v", err)
	}

	// Compute the hash
	hasher := sha256.New()
	hasher.Write(registryData)
	hash := hasher.Sum(nil)

	// Sign the hash
	signature, err := rsa.SignPKCS1v15(nil, privateKey, crypto.SHA256, hash)
	if err != nil {
		return fmt.Errorf("failed to sign registry: %v", err)
	}

	// Update the registry with the signature
	registry := &internal.Registry{}
	if err := json.Unmarshal(registryData, registry); err != nil {
		return fmt.Errorf("failed to parse registry for signing: %v", err)
	}

	registry.Signature = fmt.Sprintf("%x", signature)

	return saveRegistry(registryPath, registry)
}

func validateRegistrySignature(registryPath, publicKeyPath string) error {
	// Load the registry
	registryData, err := os.ReadFile(registryPath)
	if err != nil {
		return fmt.Errorf("failed to read registry: %w", err)
	}

	// Unmarshal to extract signature and registry content
	var rawRegistry map[string]json.RawMessage
	if err := json.Unmarshal(registryData, &rawRegistry); err != nil {
		return fmt.Errorf("failed to parse registry: %w", err)
	}

	var signature string
	if err := json.Unmarshal(rawRegistry["signature"], &signature); err != nil {
		return fmt.Errorf("failed to extract signature: %w", err)
	}
	delete(rawRegistry, "signature")

	// Marshal the registry content back to JSON
	registryContent, err := json.Marshal(rawRegistry)
	if err != nil {
		return fmt.Errorf("failed to marshal registry content: %w", err)
	}

	// Load the public key
	publicKeyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	block, _ := pem.Decode(publicKeyData)
	if block == nil {
		return fmt.Errorf("failed to decode public key PEM")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaPubKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not RSA")
	}

	// Verify the signature
	hash := crypto.SHA256.New()
	hash.Write(registryContent)
	computedHash := hash.Sum(nil)

	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	err = rsa.VerifyPKCS1v15(rsaPubKey, crypto.SHA256, computedHash, sigBytes)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	return nil
}
