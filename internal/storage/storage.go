package storage

import "context"

// TODO: maybe add metadata passing options

// IncludeEdge holds all relevant information to create meaningful
// edges inside of the storage system for querying.
type IncludeEdge struct {
	SourceProject string
	TargetProject string
	Ref           string
	Files         []string
}

type Storage interface {
	// CreateProjectNode takes a project name and takes care of
	// creating a node inside of the storage.
	CreateProjectNode(ctx context.Context, projectPath string) error
	// CreateIncludeEdge is responsible for creating the edges
	// inside of the storage, include edges should have the
	// `ref` and `files` fields set to allow for queries based
	// on the data.
	CreateIncludeEdge(ctx context.Context, include IncludeEdge) error
}
