package storage

import "context"

// TODO: maybe add metadata passing options

// Edge holds all relevant information to create meaningful
// edges inside the storage system for querying.
type Edge struct {
	SourceProject string
	TargetProject string
	Ref           string
	Files         []string
}

type Storage interface {
	// CreateProjectNode takes a project name and takes care of
	// creating a node inside the storage.
	CreateProjectNode(ctx context.Context, projectPath string) error

	// CreateIncludeEdge is responsible for creating the include edges
	// inside of the storage, include edges should have the
	// `ref` and `files` fields set to allow for queries based
	// on the data.
	CreateIncludeEdge(ctx context.Context, include Edge) error

	// CreateTriggerEdge is responsible for creating the edges for triggers
	// inside of the storage
	CreateTriggerEdge(ctx context.Context, include Edge) error

	// RemoveAll will delete all nodes & edges
	RemoveAll(ctx context.Context) error
}
