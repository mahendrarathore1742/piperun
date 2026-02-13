package dag

import (
	"testing"
)

func TestResolveLinear(t *testing.T) {
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", []string{"a"})
	g.AddNode("c", []string{"b"})

	layers, err := g.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 3 {
		t.Fatalf("expected 3 layers, got %d", len(layers))
	}
	if layers[0].Stages[0] != "a" {
		t.Errorf("expected layer 0 = [a], got %v", layers[0].Stages)
	}
	if layers[1].Stages[0] != "b" {
		t.Errorf("expected layer 1 = [b], got %v", layers[1].Stages)
	}
	if layers[2].Stages[0] != "c" {
		t.Errorf("expected layer 2 = [c], got %v", layers[2].Stages)
	}
}

func TestResolveParallel(t *testing.T) {
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", nil)
	g.AddNode("c", []string{"a", "b"})

	layers, err := g.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(layers))
	}
	if len(layers[0].Stages) != 2 {
		t.Errorf("expected 2 stages in layer 0, got %d", len(layers[0].Stages))
	}
}

func TestResolveCycle(t *testing.T) {
	g := New()
	g.AddNode("a", []string{"b"})
	g.AddNode("b", []string{"a"})

	_, err := g.Resolve()
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func TestResolveUnknownDep(t *testing.T) {
	g := New()
	g.AddNode("a", []string{"nonexistent"})

	_, err := g.Resolve()
	if err == nil {
		t.Fatal("expected unknown dep error, got nil")
	}
}
