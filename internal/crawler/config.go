package crawler

import (
	"errors"
	"fmt"
	"time"

	"github.com/ardanlabs/conf/v3"
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
	GitlabHost             string        `conf:"required,short:g,env:GITLAB_HOST"`
	GitlabToken            string        `conf:"required,short:t,env:GITLAB_TOKEN"`
	GitlabMaxRPS           int           `conf:"default:1,short:r,env:GITLAB_MAX_RPS"`
	Storage                string        `conf:"required,short:s,env:STORAGE_BACKEND"`
	StorageCleanup         bool          `conf:"default:false,short:c,env:STORAGE_CLEANUP"`
	DefaultRefName         string        `conf:"default:HEAD,short:d,env:DEFAULT_REF_NAME"`
	HTTPClientTimeout      time.Duration `conf:"default:5s,short:x,env:HTTP_CLIENT_TIMEOUT"`
	HTTPClientMaxRetry     int           `conf:"default:2,short,m,env:HTTP_CLIENT_MAX_RETRY"`
	HTTPClientMaxRetryWait time.Duration `conf:"default:30s,short:w,env:HTTP_CLIENT_MAX_RETRY_WAIT"`
	HTTPClientMinRetryWait time.Duration `conf:"default:5s,short:n,env:HTTP_CLIENT_MIN_RETRY_WAIT"`
	NumberOfWorkers        int           `conf:"default:20,short:c,env:NUMBER_OF_WORKERS"`
	// There should be global config composition maybe? For not this lives here
	// though this is the global log level
	LogLevel  int    `conf:"default:1,env:LOG_LEVEL"`
	LogFormat string `conf:"default:json,env:LOG_FORMAT"`
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
