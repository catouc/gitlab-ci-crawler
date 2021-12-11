package crawler

import (
	"errors"
	"fmt"
	"github.com/ardanlabs/conf/v2"
)

const (
	StorageUnknown = -1
	StorageNeo4j   = 0
)

type Storage int

func (sb Storage) String() (string, error) {
	switch sb {
	case StorageNeo4j:
		return "neo4j", nil
	default:
		return "", fmt.Errorf("unknown storage: %d", sb)
	}
}

func StorageFromString(s string) (Storage, error) {
	switch s {
	case "neo4j":
		return StorageNeo4j, nil
	default:
		return StorageUnknown, fmt.Errorf("unknown storage: %s", s)
	}
}

type Config struct {
	GitlabHost   string `conf:"required,short:g,env:GITLAB_HOST"`
	GitlabToken  string `conf:"required,short:t,env:GITLAB_TOKEN"`
	GitlabMaxRPS int    `conf:"default:1,short:r,env:GITLAB_MAX_RPS"`
	Storage      string `conf:"required,short:s,env:STORAGE_BACKEND"`
}

func ParseConfig(cfg *Config) error {
	help, err := conf.Parse("", cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			return errors.New(help)
		}
		return fmt.Errorf("failed to parse config: %w", err)
	}

	_, err = StorageFromString(cfg.Storage)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	return nil
}
