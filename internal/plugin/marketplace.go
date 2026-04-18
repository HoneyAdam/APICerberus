package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Marketplace provides a registry for third-party plugins.
type Marketplace struct {
	storage MarketplaceStorage
	config  MarketplaceConfig

	// Cached plugin index
	index      *PluginIndex
	indexMu    sync.RWMutex
	indexStale bool

	// HTTP client with proper timeouts
	httpClient *http.Client
}

// MarketplaceConfig holds marketplace configuration.
type MarketplaceConfig struct {
	Enabled           bool              `json:"enabled" yaml:"enabled"`
	DataDir           string            `json:"data_dir" yaml:"data_dir"`
	RegistryURL       string            `json:"registry_url" yaml:"registry_url"`
	TrustedSigners    []string          `json:"trusted_signers" yaml:"trusted_signers"`
	TrustedSignerKeys map[string]string `json:"trusted_signer_keys" yaml:"trusted_signer_keys"`
	AutoUpdate        bool              `json:"auto_update" yaml:"auto_update"`
	UpdateInterval    time.Duration     `json:"update_interval" yaml:"update_interval"`
	VerifySignatures  bool              `json:"verify_signatures" yaml:"verify_signatures"`
	MaxPluginSize     int64             `json:"max_plugin_size" yaml:"max_plugin_size"`
	AllowedPhases     []string          `json:"allowed_phases" yaml:"allowed_phases"`
}

// DefaultMarketplaceConfig returns default marketplace configuration.
func DefaultMarketplaceConfig() MarketplaceConfig {
	return MarketplaceConfig{
		Enabled:          false,
		DataDir:          "./plugins",
		RegistryURL:      "https://plugins.apicerberus.io",
		AutoUpdate:       false,
		UpdateInterval:   24 * time.Hour,
		VerifySignatures: true,
		MaxPluginSize:    100 * 1024 * 1024, // 100MB
		AllowedPhases:    []string{"PRE_AUTH", "AUTH", "POST_AUTH", "PRE_PROXY", "PROXY", "POST_PROXY"},
	}
}

// PluginIndex represents the plugin registry index.
type PluginIndex struct {
	Version   string          `json:"version"`
	UpdatedAt time.Time       `json:"updated_at"`
	Plugins   []PluginListing `json:"plugins"`
}

// PluginListing represents a plugin in the registry.
type PluginListing struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Author            string            `json:"author"`
	Version           string            `json:"version"`
	LatestVersion     string            `json:"latest_version"`
	Phases            []string          `json:"phases"`
	Downloads         int               `json:"downloads"`
	Rating            float64           `json:"rating"`
	PublishedAt       time.Time         `json:"published_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	Tags              []string          `json:"tags"`
	Homepage          string            `json:"homepage"`
	Repository        string            `json:"repository"`
	License           string            `json:"license"`
	Signatures        map[string]string `json:"signatures"` // version -> signature
	Checksums         map[string]string `json:"checksums"`  // version -> sha256
	Dependencies      []string          `json:"dependencies"`
	MinGatewayVersion string            `json:"min_gateway_version"`
}

// InstalledPlugin represents an installed plugin.
type InstalledPlugin struct {
	PluginListing
	InstallPath      string         `json:"install_path"`
	InstalledAt      time.Time      `json:"installed_at"`
	InstalledVersion string         `json:"installed_version"`
	Enabled          bool           `json:"enabled"`
	Config           map[string]any `json:"config,omitempty"`
}

// MarketplaceStorage defines the storage interface for marketplace.
type MarketplaceStorage interface {
	SavePlugin(id string, version string, data io.Reader) (string, error)
	LoadPlugin(path string) (io.ReadCloser, error)
	DeletePlugin(path string) error
	ListInstalled() ([]InstalledPlugin, error)
	SaveMetadata(plugin InstalledPlugin) error
}

// FileSystemStorage implements MarketplaceStorage using the filesystem.
type FileSystemStorage struct {
	basePath string
}

// NewFileSystemStorage creates a new filesystem storage.
func NewFileSystemStorage(basePath string) (*FileSystemStorage, error) {
	if err := os.MkdirAll(basePath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create plugin directory: %w", err)
	}

	return &FileSystemStorage{basePath: basePath}, nil
}

// SavePlugin saves a plugin to storage.
func (fs *FileSystemStorage) SavePlugin(id, version string, data io.Reader) (string, error) {
	pluginDir := filepath.Join(fs.basePath, "installed", sanitizeID(id))
	if err := os.MkdirAll(pluginDir, 0750); err != nil {
		return "", err
	}

	pluginPath := filepath.Join(pluginDir, fmt.Sprintf("%s.tar.gz", version))
	// #nosec G304 -- pluginPath is constructed under controlled basePath with sanitized ID.
	file, err := os.Create(pluginPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, data); err != nil {
		return "", err
	}

	return pluginPath, nil
}

// LoadPlugin loads a plugin from storage.
func (fs *FileSystemStorage) LoadPlugin(path string) (io.ReadCloser, error) {
	// #nosec G304 -- path is controlled by the storage layer under basePath.
	return os.Open(path)
}

// DeletePlugin deletes a plugin from storage.
func (fs *FileSystemStorage) DeletePlugin(path string) error {
	return os.Remove(path)
}

// ListInstalled lists all installed plugins.
func (fs *FileSystemStorage) ListInstalled() ([]InstalledPlugin, error) {
	installedDir := filepath.Join(fs.basePath, "installed")
	entries, err := os.ReadDir(installedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var plugins []InstalledPlugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metadataPath := filepath.Join(installedDir, entry.Name(), "metadata.json")
		// #nosec G304 -- metadataPath is constructed under controlled installedDir from directory listing.
		data, err := os.ReadFile(metadataPath)
		if err != nil {
			continue
		}

		var plugin InstalledPlugin
		if err := json.Unmarshal(data, &plugin); err != nil {
			continue
		}

		plugins = append(plugins, plugin)
	}

	return plugins, nil
}

// SaveMetadata saves plugin metadata.
func (fs *FileSystemStorage) SaveMetadata(plugin InstalledPlugin) error {
	pluginDir := filepath.Join(fs.basePath, "installed", sanitizeID(plugin.ID))
	if err := os.MkdirAll(pluginDir, 0750); err != nil {
		return err
	}

	metadataPath := filepath.Join(pluginDir, "metadata.json")
	data, err := json.MarshalIndent(plugin, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metadataPath, data, 0600)
}

// NewMarketplace creates a new plugin marketplace.
func NewMarketplace(config MarketplaceConfig) (*Marketplace, error) {
	if !config.Enabled {
		return &Marketplace{
			config: config,
		}, nil
	}

	storage, err := NewFileSystemStorage(config.DataDir)
	if err != nil {
		return nil, err
	}

	mp := &Marketplace{
		config:     config,
		storage:    storage,
		indexStale: true,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          10,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
			},
		},
	}

	// Load cached index
	if err := mp.loadCachedIndex(); err != nil {
		// Cache miss is okay, will fetch from registry
		mp.index = &PluginIndex{
			Version:   "1.0.0",
			UpdatedAt: time.Now(),
			Plugins:   []PluginListing{},
		}
	}

	return mp, nil
}

// IsEnabled returns true if marketplace is enabled.
func (mp *Marketplace) IsEnabled() bool {
	return mp != nil && mp.config.Enabled
}

// Search searches for plugins matching the query.
func (mp *Marketplace) Search(query string, tags []string) []PluginListing {
	mp.indexMu.RLock()
	defer mp.indexMu.RUnlock()

	if mp.index == nil {
		return nil
	}

	var results []PluginListing
	query = strings.ToLower(query)

	for _, plugin := range mp.index.Plugins {
		// Filter by query
		if query != "" {
			match := strings.Contains(strings.ToLower(plugin.Name), query) ||
				strings.Contains(strings.ToLower(plugin.Description), query) ||
				strings.Contains(strings.ToLower(plugin.Author), query)
			if !match {
				continue
			}
		}

		// Filter by tags
		if len(tags) > 0 {
			hasTag := false
			for _, tag := range tags {
				for _, pluginTag := range plugin.Tags {
					if strings.EqualFold(tag, pluginTag) {
						hasTag = true
						break
					}
				}
			}
			if !hasTag {
				continue
			}
		}

		results = append(results, plugin)
	}

	// Sort by rating (descending), then by downloads
	sort.Slice(results, func(i, j int) bool {
		if results[i].Rating != results[j].Rating {
			return results[i].Rating > results[j].Rating
		}
		return results[i].Downloads > results[j].Downloads
	})

	return results
}

// GetPlugin returns a plugin by ID.
func (mp *Marketplace) GetPlugin(id string) (*PluginListing, error) {
	mp.indexMu.RLock()
	defer mp.indexMu.RUnlock()

	if mp.index == nil {
		return nil, fmt.Errorf("plugin index not available")
	}

	for _, plugin := range mp.index.Plugins {
		if plugin.ID == id {
			return &plugin, nil
		}
	}

	return nil, fmt.Errorf("plugin not found: %s", id)
}

// Install installs a plugin from the marketplace.
func (mp *Marketplace) Install(ctx context.Context, id, version string) (*InstalledPlugin, error) {
	if !mp.IsEnabled() {
		return nil, fmt.Errorf("marketplace is disabled")
	}

	// Get plugin info
	listing, err := mp.GetPlugin(id)
	if err != nil {
		return nil, err
	}

	// Check version
	if version == "" {
		version = listing.LatestVersion
	}

	// Check if already installed
	installed, err := mp.GetInstalled(id)
	if err == nil && installed != nil {
		if installed.InstalledVersion == version {
			return nil, fmt.Errorf("plugin %s version %s is already installed", id, version)
		}
	}

	// Download plugin
	downloadURL := fmt.Sprintf("%s/plugins/%s/%s/download", mp.config.RegistryURL, id, version)
	data, checksum, err := mp.downloadPlugin(ctx, downloadURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download plugin: %w", err)
	}

	// Verify checksum
	expectedChecksum := listing.Checksums[version]
	if expectedChecksum != "" && checksum != expectedChecksum {
		return nil, fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, checksum)
	}

	// Verify signature if required
	if mp.config.VerifySignatures {
		signature := listing.Signatures[version]
		if signature == "" {
			return nil, fmt.Errorf("plugin is not signed")
		}
		if err := mp.verifySignature(data, signature, listing.Author); err != nil {
			return nil, fmt.Errorf("signature verification failed: %w", err)
		}
	}

	// Save plugin
	installPath, err := mp.storage.SavePlugin(id, version, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to save plugin: %w", err)
	}

	// Create installed plugin record
	installedPlugin := InstalledPlugin{
		PluginListing:    *listing,
		InstallPath:      installPath,
		InstalledAt:      time.Now(),
		InstalledVersion: version,
		Enabled:          false,
		Config:           make(map[string]any),
	}

	// Extract and install
	if err := mp.extractAndInstall(installPath); err != nil {
		if delErr := mp.storage.DeletePlugin(installPath); delErr != nil {
			log.Printf("[WARN] marketplace: failed to clean up plugin after install error: %v", delErr)
		}
		return nil, fmt.Errorf("failed to install plugin: %w", err)
	}

	// Save metadata
	if err := mp.storage.SaveMetadata(installedPlugin); err != nil {
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}

	return &installedPlugin, nil
}

// Uninstall removes an installed plugin.
func (mp *Marketplace) Uninstall(id string) error {
	if !mp.IsEnabled() {
		return fmt.Errorf("marketplace is disabled")
	}

	_, err := mp.GetInstalled(id)
	if err != nil {
		return err
	}

	// Unregister from plugin system
	mp.Unregister(id)

	// Delete files
	pluginDir := filepath.Join(mp.config.DataDir, "installed", sanitizeID(id))
	return os.RemoveAll(pluginDir)
}

// Unregister invalidates the cached index entry for a plugin.
// The installed plugin files remain on disk; a gateway config reload
// is required to deactivate the plugin in the pipeline.
func (mp *Marketplace) Unregister(id string) {
	mp.indexMu.Lock()
	defer mp.indexMu.Unlock()
	mp.indexStale = true
}

// GetInstalled returns an installed plugin by ID.
func (mp *Marketplace) GetInstalled(id string) (*InstalledPlugin, error) {
	installed, err := mp.storage.ListInstalled()
	if err != nil {
		return nil, err
	}

	for _, plugin := range installed {
		if plugin.ID == id {
			return &plugin, nil
		}
	}

	return nil, fmt.Errorf("plugin not installed: %s", id)
}

// ListInstalled lists all installed plugins.
func (mp *Marketplace) ListInstalled() ([]InstalledPlugin, error) {
	if !mp.IsEnabled() {
		return nil, nil
	}

	return mp.storage.ListInstalled()
}

// Enable enables an installed plugin.
func (mp *Marketplace) Enable(id string) error {
	installed, err := mp.GetInstalled(id)
	if err != nil {
		return err
	}

	installed.Enabled = true
	return mp.storage.SaveMetadata(*installed)
}

// Disable disables an installed plugin.
func (mp *Marketplace) Disable(id string) error {
	installed, err := mp.GetInstalled(id)
	if err != nil {
		return err
	}

	installed.Enabled = false
	return mp.storage.SaveMetadata(*installed)
}

// UpdateIndex fetches the latest plugin index from the registry.
func (mp *Marketplace) UpdateIndex(ctx context.Context) error {
	if !mp.IsEnabled() {
		return nil
	}

	indexURL := fmt.Sprintf("%s/index.json", mp.config.RegistryURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return err
	}

	resp, err := mp.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch index: %s", resp.Status)
	}

	var index PluginIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return fmt.Errorf("failed to decode index: %w", err)
	}

	mp.indexMu.Lock()
	mp.index = &index
	mp.indexStale = false
	mp.indexMu.Unlock()

	// Cache the index
	if err := mp.cacheIndex(&index); err != nil {
		log.Printf("[WARN] marketplace: failed to cache index: %v", err)
	}

	return nil
}

// CheckForUpdates checks for available updates for installed plugins.
func (mp *Marketplace) CheckForUpdates() ([]PluginUpdate, error) {
	if !mp.IsEnabled() {
		return nil, nil
	}

	installed, err := mp.storage.ListInstalled()
	if err != nil {
		return nil, err
	}

	var updates []PluginUpdate
	for _, plugin := range installed {
		listing, err := mp.GetPlugin(plugin.ID)
		if err != nil {
			continue
		}

		if listing.LatestVersion != plugin.InstalledVersion {
			updates = append(updates, PluginUpdate{
				PluginID:         plugin.ID,
				CurrentVersion:   plugin.InstalledVersion,
				AvailableVersion: listing.LatestVersion,
			})
		}
	}

	return updates, nil
}

// PluginUpdate represents an available plugin update.
type PluginUpdate struct {
	PluginID         string `json:"plugin_id"`
	CurrentVersion   string `json:"current_version"`
	AvailableVersion string `json:"available_version"`
}

// downloadPlugin downloads a plugin from the given URL.
func (mp *Marketplace) downloadPlugin(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := mp.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download failed: %s", resp.Status)
	}

	// Check size
	if resp.ContentLength > mp.config.MaxPluginSize {
		return nil, "", fmt.Errorf("plugin exceeds maximum size of %d bytes", mp.config.MaxPluginSize)
	}

	// Read data and compute checksum
	data, err := io.ReadAll(io.LimitReader(resp.Body, mp.config.MaxPluginSize))
	if err != nil {
		return nil, "", err
	}

	hash := sha256.Sum256(data)
	checksum := hex.EncodeToString(hash[:])

	return data, checksum, nil
}

// verifySignature verifies a plugin Ed25519 signature.
func (mp *Marketplace) verifySignature(data []byte, signature, author string) error {
	pubKeyB64, ok := mp.config.TrustedSignerKeys[author]
	if !ok {
		return fmt.Errorf("author %s is not a trusted signer", author)
	}

	pubKey, err := base64.StdEncoding.DecodeString(pubKeyB64)
	if err != nil {
		return fmt.Errorf("invalid public key for author %s: %w", author, err)
	}

	if len(pubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size for author %s: expected %d, got %d", author, ed25519.PublicKeySize, len(pubKey))
	}

	var sig []byte
	// Try base64 first (common for JSON transport), then hex
	sig, err = base64.StdEncoding.DecodeString(signature)
	if err != nil {
		sig, err = hex.DecodeString(signature)
		if err != nil {
			return fmt.Errorf("invalid signature encoding: %w", err)
		}
	}

	if !ed25519.Verify(pubKey, data, sig) {
		return fmt.Errorf("signature verification failed for author %s", author)
	}

	return nil
}

// extractAndInstall extracts and installs a plugin package.
func (mp *Marketplace) extractAndInstall(installPath string) error {
	// Open the tar.gz file
	// #nosec G304 -- installPath is constructed by SavePlugin under controlled basePath with sanitized ID.
	file, err := os.Open(installPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	// Extract files
	pluginDir := filepath.Dir(installPath)
	maxExtractSize := mp.config.MaxPluginSize
	if maxExtractSize <= 0 {
		maxExtractSize = 100 * 1024 * 1024 // 100MB default
	}
	var extractedSize int64
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Sanitize path
		targetPath := filepath.Join(pluginDir, filepath.Clean("/"+header.Name))
		rel, err := filepath.Rel(pluginDir, targetPath)
		if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
			return fmt.Errorf("invalid path in archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return err
			}
		case tar.TypeReg:
			// M-012: Check BEFORE creating the file if it would exceed the limit.
			// A file that itself is larger than remaining budget can't be fully extracted.
			remaining := maxExtractSize - extractedSize
			if header.Size > remaining {
				return fmt.Errorf("extracted plugin exceeds maximum size of %d bytes", maxExtractSize)
			}
			// #nosec G304 -- targetPath is sanitized via filepath.Clean/Rel checks above.
			outFile, err := os.Create(targetPath)
			if err != nil {
				return err
			}
			written, err := io.CopyN(outFile, tarReader, remaining)
			extractedSize += written
			if err != nil && !errors.Is(err, io.EOF) {
				_ = outFile.Close() // #nosec G104 // Best-effort cleanup; returning copy error.
				return err
			}
			if extractedSize > maxExtractSize {
				_ = outFile.Close()
				return fmt.Errorf("extracted plugin exceeds maximum size of %d bytes", maxExtractSize)
			}
			if err := outFile.Close(); err != nil {
				return fmt.Errorf("failed to close extracted file: %w", err)
			}
		}
	}

	return nil
}

// loadCachedIndex loads the cached plugin index.
func (mp *Marketplace) loadCachedIndex() error {
	cachePath := filepath.Join(mp.config.DataDir, "cache", "index.json")
	// #nosec G304 -- cachePath is constructed under controlled DataDir.
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return err
	}

	var index PluginIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return err
	}

	mp.index = &index
	return nil
}

// cacheIndex saves the plugin index to cache.
func (mp *Marketplace) cacheIndex(index *PluginIndex) error {
	cacheDir := filepath.Join(mp.config.DataDir, "cache")
	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		return err
	}

	cachePath := filepath.Join(cacheDir, "index.json")
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cachePath, data, 0600)
}

// sanitizeID sanitizes a plugin ID for use in filesystem paths.
func sanitizeID(id string) string {
	// Replace any non-alphanumeric characters (except dash and underscore) with dash
	re := regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
	return re.ReplaceAllString(id, "-")
}
