// Package dag implements a simple directed acyclic graph for stage dependency
// resolution and topological ordering with concurrency grouping.
package dag

import (
	"fmt"
	"sort"
)

// Graph is a directed acyclic graph of string-keyed nodes.
type Graph struct {
	nodes map[string][]string // node -> list of dependencies (edges TO this node)
}

// New creates a new empty Graph.
func New() *Graph {
	return &Graph{nodes: make(map[string][]string)}
}

// AddNode adds a node with its dependencies.
func (g *Graph) AddNode(name string, deps []string) {
	g.nodes[name] = deps
}

// Layer represents a group of stages that can run concurrently.
type Layer struct {
	Stages []string
}

// Resolve performs a topological sort and groups nodes into concurrent layers.
// Nodes in the same layer have no dependencies on each other and can run in parallel.
func (g *Graph) Resolve() ([]Layer, error) {
	// Kahn's algorithm
	inDegree := make(map[string]int)
	dependents := make(map[string][]string) // dep -> nodes that depend on it

	for node := range g.nodes {
		if _, ok := inDegree[node]; !ok {
			inDegree[node] = 0
		}
		for _, dep := range g.nodes[node] {
			if _, ok := g.nodes[dep]; !ok {
				return nil, fmt.Errorf("stage %q depends on unknown stage %q", node, dep)
			}
			dependents[dep] = append(dependents[dep], node)
			inDegree[node]++
		}
	}

	var layers []Layer
	visited := 0

	for {
		// Collect all nodes with in-degree 0
		var ready []string
		for node, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, node)
			}
		}
		if len(ready) == 0 {
			break
		}
		sort.Strings(ready) // deterministic ordering
		layers = append(layers, Layer{Stages: ready})
		visited += len(ready)

		for _, node := range ready {
			delete(inDegree, node)
			for _, dep := range dependents[node] {
				inDegree[dep]--
			}
		}
	}

	if visited != len(g.nodes) {
		return nil, fmt.Errorf("cycle detected in stage dependencies")
	}
	return layers, nil
}
