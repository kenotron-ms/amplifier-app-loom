package meeting

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

var (
	configBucket = []byte("meeting")
	configKey    = []byte("config")
)

// Config holds persisted meeting transcription settings.
type Config struct {
	Enabled   bool   `json:"enabled"`
	OutputDir string `json:"output_dir"`
	Model     string `json:"model"` // e.g. "whisper-1"
}

// DefaultConfig returns the default config (disabled, ~/meetings, whisper-1).
func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Enabled:   false,
		OutputDir: filepath.Join(home, "meetings"),
		Model:     "whisper-1",
	}
}

// ConfigStore reads and writes Config from bbolt.
type ConfigStore struct {
	db *bolt.DB
}

// NewConfigStore wraps an existing bbolt DB.
func NewConfigStore(db *bolt.DB) *ConfigStore {
	return &ConfigStore{db: db}
}

// Get returns the stored config, or the default if none has been saved.
func (s *ConfigStore) Get(ctx context.Context) (Config, error) {
	cfg := DefaultConfig()
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(configBucket)
		if b == nil {
			return nil
		}
		v := b.Get(configKey)
		if v == nil {
			return nil
		}
		return json.Unmarshal(v, &cfg)
	})
	return cfg, err
}

// Set persists cfg to bbolt.
func (s *ConfigStore) Set(ctx context.Context, cfg Config) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(configBucket)
		if err != nil {
			return err
		}
		v, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		return b.Put(configKey, v)
	})
}
