package neo4j

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ardanlabs/conf/v2"

	"github.com/deichindianer/gitlab-ci-crawler/internal/storage"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	neo4jDriver "github.com/neo4j/neo4j-go-driver/v4/neo4j"
)

type Storage struct {
	Connection neo4j.Driver
	Session    neo4j.Session
}

type Config struct {
	Host     string `conf:"default:bolt://127.0.0.1:7687,flag:neo4j-host,short:n,env:NEO4J_HOST"`
	Username string `conf:"default:neo4j,flag:neo4j-username,short:u,env:NEO4J_USERNAME"`
	Password string `conf:"required,flag:neo4j-password,short:w,env:NEO4J_PASSWORD"`
	Realm    string
}

func New(cfg *Config) (*Storage, error) {
	help, err := conf.Parse("", cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			return nil, errors.New(help)
		}
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	driver, err := neo4j.NewDriver(cfg.Host, neo4jDriver.BasicAuth(cfg.Username, cfg.Password, cfg.Realm))
	if err != nil {
		return nil, fmt.Errorf("failed to create neo4j driver: %w", err)
	}

	session := driver.NewSession(neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeWrite,
	})

	return &Storage{
		Connection: driver,
		Session:    session,
	}, nil
}

func (s *Storage) CreateProjectNode(_ context.Context, projectPath string) error {
	cypher := "MERGE (p:Project {name: $projectPath})"
	parameters := map[string]interface{}{
		"projectPath": projectPath,
	}
	_, err := s.Session.WriteTransaction(func(transaction neo4j.Transaction) (interface{}, error) {
		result, err := transaction.Run(cypher, parameters)
		if err != nil {
			return nil, err
		}

		if result.Next() {
			return nil, nil
		}

		return nil, result.Err()
	})
	return err
}

func (s *Storage) CreateIncludeEdge(_ context.Context, include storage.IncludeEdge) error {
	cypher := "MATCH (p:Project {name: $sourceProject})\nMATCH (p2:Project {name: $targetProject})\nMERGE (p)-[rel:INCLUDES {ref: $ref, files:$files}]->(p2)"
	parameters := map[string]interface{}{
		"sourceProject": include.SourceProject,
		"targetProject": include.TargetProject,
		"ref":           include.Ref,
		"files":         strings.Join(include.Files, ","),
	}
	_, err := s.Session.WriteTransaction(func(transaction neo4j.Transaction) (interface{}, error) {
		result, err := transaction.Run(cypher, parameters)
		if err != nil {
			return nil, err
		}

		if result.Next() {
			return nil, nil
		}

		return nil, result.Err()
	})
	return err
}
