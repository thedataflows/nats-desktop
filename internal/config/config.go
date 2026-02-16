package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Preferences     Preferences    `toml:"preferences"`
	Contexts        []*ContextInfo `toml:"contexts"`
	PreviousContext string         `toml:"previous_context,omitempty"`
}

type Preferences struct {
	Theme          string        `toml:"theme"`
	FontSize       int           `toml:"fontSize"`
	AutoRefresh    bool          `toml:"autoRefresh"`
	RefreshPeriod  time.Duration `toml:"refreshPeriod"`
	BackupLocation string        `toml:"backupLocation"`
}

type ContextInfo struct {
	Name        string `toml:"name"`
	Description string `toml:"description,omitempty"`
	URL         string `toml:"url"`
	Username    string `toml:"username,omitempty"`
	Password    string `toml:"password,omitempty"`
	Token       string `toml:"token,omitempty"`
	NKeyFile    string `toml:"nkey_file,omitempty"`
	Credentials string `toml:"credentials,omitempty"`
	Active      bool   `toml:"active,omitempty"`
	Connected   bool   `toml:"-"`
}

// NatsCLIContext represents the natscli JSON format for compatibility
type NatsCLIContext struct {
	Description string `json:"description"`
	URL         string `json:"url"`
	User        string `json:"user,omitempty"`
	Password    string `json:"password,omitempty"`
	Token       string `json:"token,omitempty"`
	NKey        string `json:"nkey,omitempty"`
	Creds       string `json:"creds,omitempty"`
	TLSCert     string `json:"tls_cert,omitempty"`
	TLSKey      string `json:"tls_key,omitempty"`
	TLSCA       string `json:"tls_ca,omitempty"`
}

type KeyBinding struct {
	Key      string `toml:"key"`
	Modifier string `toml:"modifier"`
	Action   string `toml:"action"`
}

func NewConfig() *Config {
	return &Config{
		Preferences: Preferences{
			Theme:          "light",
			FontSize:       14,
			AutoRefresh:    false,
			RefreshPeriod:  30 * time.Second,
			BackupLocation: "./jetstream-backups",
		},
		Contexts: []*ContextInfo{},
	}
}

func (c *Config) Save() error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := toml.NewEncoder(f).Encode(c); err != nil {
		return err
	}
	return nil
}

func (c *Config) Load() error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if _, err := toml.Decode(string(data), c); err != nil {
		return err
	}

	return nil
}

func GetConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "nats-desktop.toml", nil
	}
	return filepath.Join(configDir, "nats-desktop.toml"), nil
}

func DefaultKeyBindings() map[string]KeyBinding {
	return map[string]KeyBinding{
		"global-search": {"k", "CmdOrCtrl", "Open Search"},
		"new-tab":       {"t", "CmdOrCtrl", "New Tab"},
		"close-tab":     {"w", "CmdOrCtrl", "Close Tab"},
		"refresh":       {"r", "CmdOrCtrl", "Refresh"},
		"connect":       {"c", "CmdOrCtrl", "Connect"},
		"disconnect":    {"d", "CmdOrCtrl", "Disconnect"},
	}
}

func (p *Preferences) GetRefreshInterval() time.Duration {
	if !p.AutoRefresh {
		return 0
	}
	return p.RefreshPeriod
}

// GetBackupLocation returns the backup directory path
func (p *Preferences) GetBackupLocation() string {
	if p.BackupLocation == "" {
		return "./jetstream-backups"
	}
	return p.BackupLocation
}

// GetNatsCLIContextPath returns the path to natscli context directory
func GetNatsCLIContextPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "nats", "context"), nil
}

// ImportFromNatsCLI imports contexts from natscli format
func (c *Config) ImportFromNatsCLI() error {
	contextDir, err := GetNatsCLIContextPath()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(contextDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		contextName := entry.Name()[:len(entry.Name())-5] // Remove .json
		contextPath := filepath.Join(contextDir, entry.Name())

		data, err := os.ReadFile(contextPath)
		if err != nil {
			continue
		}

		var natsCtx NatsCLIContext
		if err := json.Unmarshal(data, &natsCtx); err != nil {
			continue
		}

		// Check if context already exists
		exists := false
		for _, ctx := range c.Contexts {
			if ctx.Name == contextName {
				exists = true
				break
			}
		}

		if !exists {
			c.Contexts = append(c.Contexts, &ContextInfo{
				Name:        contextName,
				Description: natsCtx.Description,
				URL:         natsCtx.URL,
				Username:    natsCtx.User,
				Password:    natsCtx.Password,
				Token:       natsCtx.Token,
				NKeyFile:    natsCtx.NKey,
				Credentials: natsCtx.Creds,
			})
		}
	}

	return nil
}

// ExportToNatsCLI exports contexts to natscli format
func (c *Config) ExportToNatsCLI() error {
	contextDir, err := GetNatsCLIContextPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(contextDir, 0755); err != nil {
		return err
	}

	for _, ctx := range c.Contexts {
		natsCtx := NatsCLIContext{
			Description: ctx.Description,
			URL:         ctx.URL,
			User:        ctx.Username,
			Password:    ctx.Password,
			Token:       ctx.Token,
			NKey:        ctx.NKeyFile,
			Creds:       ctx.Credentials,
		}

		data, err := json.MarshalIndent(natsCtx, "", "  ")
		if err != nil {
			continue
		}

		contextPath := filepath.Join(contextDir, ctx.Name+".json")
		if err := os.WriteFile(contextPath, data, 0644); err != nil {
			continue
		}
	}

	return nil
}

// GetActiveContext returns the currently active context
func (c *Config) GetActiveContext() *ContextInfo {
	for _, ctx := range c.Contexts {
		if ctx.Active {
			return ctx
		}
	}
	return nil
}

// SetActiveContext sets the active context and tracks previous
func (c *Config) SetActiveContext(name string) {
	currentActive := c.GetActiveContext()
	if currentActive != nil {
		c.PreviousContext = currentActive.Name
		currentActive.Active = false
	}

	for _, ctx := range c.Contexts {
		if ctx.Name == name {
			ctx.Active = true
			break
		}
	}
}

// SwitchToPreviousContext switches to the previously active context
func (c *Config) SwitchToPreviousContext() bool {
	if c.PreviousContext == "" {
		return false
	}

	for _, ctx := range c.Contexts {
		if ctx.Name == c.PreviousContext {
			c.SetActiveContext(ctx.Name)
			return true
		}
	}
	return false
}
