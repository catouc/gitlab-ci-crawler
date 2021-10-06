package main

import (
	"log"
	"os"

	"github.com/deichindianer/dependency-seeker/internal/projects"
)

func main() {
	if os.Getenv("GITLAB_TOKEN") == "" {
		log.Fatal("missing gitlab token\n")
	}

	if os.Getenv("DEPENDENCY_LOGGING") == "DEBUG" {
		projects.Debug = true
	}

	cfg := projects.Config{
		GitlabHost:    "https://source.tui",
		GitlabToken:   os.Getenv("GITLAB_TOKEN"),
		Neo4jHost:     "bolt://localhost:7687",
		Neo4jUsername: os.Getenv("NEO4J_USERNAME"),
		Neo4jPassword: os.Getenv("NEO4J_PASSWORD"),
	}

	s, err := projects.New(cfg)
	if err != nil {
		log.Fatalf("failed to setup source: %s\n", err)
	}

	err = s.PlotDependencyTree(5620)
	//err = s.GatherProjectData(projects.GatherProjectDataInput{
	//	OutPath: "out",
	//	GroupID: 129,
	//})
	if err != nil {
		log.Fatalf("failed to gather project data: %s", err)
	}
}
