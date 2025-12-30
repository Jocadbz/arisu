package main

import (
	"testing"
)

func TestParseBlocks(t *testing.T) {
	content := `line1
line2

line3
line4

line5`
	blocks := parseBlocks(content)

	if len(blocks) != 3 {
		t.Errorf("Expected 3 blocks, got %d", len(blocks))
	}

	if len(blocks[0].Lines) != 2 {
		t.Errorf("Expected 2 lines in block 0, got %d", len(blocks[0].Lines))
	}
	if blocks[0].Lines[0] != "line1" {
		t.Errorf("Expected line1, got %s", blocks[0].Lines[0])
	}

	if len(blocks[1].Lines) != 2 {
		t.Errorf("Expected 2 lines in block 1, got %d", len(blocks[1].Lines))
	}
	if blocks[1].Lines[0] != "line3" {
		t.Errorf("Expected line3, got %s", blocks[1].Lines[0])
	}

	if len(blocks[2].Lines) != 1 {
		t.Errorf("Expected 1 line in block 2, got %d", len(blocks[2].Lines))
	}
	if blocks[2].Lines[0] != "line5" {
		t.Errorf("Expected line5, got %s", blocks[2].Lines[0])
	}
}

func TestBlocksToString(t *testing.T) {
	blocks := []Block{
		{ID: 0, Lines: []string{"line1", "line2"}},
		{ID: 1, Lines: []string{"line3"}},
	}

	expected := "line1\nline2\n\nline3\n"
	result := blocksToString(blocks)

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}
