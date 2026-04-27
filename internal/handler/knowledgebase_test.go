package handler

import (
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestValidateExtractConfigAllowsEnabledDefaultSchema(t *testing.T) {
	config := &types.ExtractConfig{Enabled: true}

	if err := validateExtractConfig(config); err != nil {
		t.Fatalf("validateExtractConfig() error = %v", err)
	}
}

func TestValidateExtractConfigResetsDisabledConfig(t *testing.T) {
	config := &types.ExtractConfig{
		Enabled: false,
		Text:    "custom example",
		Tags:    []string{"Relation"},
		Nodes: []*types.GraphNode{
			{Name: "A"},
		},
	}

	if err := validateExtractConfig(config); err != nil {
		t.Fatalf("validateExtractConfig() error = %v", err)
	}
	if config.Enabled {
		t.Fatal("disabled extract config should stay disabled")
	}
	if config.Text != "" || len(config.Tags) != 0 || len(config.Nodes) != 0 || len(config.Relations) != 0 {
		t.Fatalf("disabled extract config was not reset: %+v", config)
	}
}

func TestValidateExtractConfigRejectsPartialCustomSchema(t *testing.T) {
	config := &types.ExtractConfig{
		Enabled: true,
		Text:    "custom example",
	}

	err := validateExtractConfig(config)
	if err == nil {
		t.Fatal("validateExtractConfig() expected an error")
	}
	if !strings.Contains(err.Error(), "tags cannot be empty") {
		t.Fatalf("validateExtractConfig() error = %v, want tags cannot be empty", err)
	}
}

func TestValidateExtractConfigTrimsAndAcceptsCustomSchema(t *testing.T) {
	config := &types.ExtractConfig{
		Enabled: true,
		Text:    " custom example ",
		Tags:    []string{" Relation "},
		Nodes: []*types.GraphNode{
			{Name: " Entity A "},
			{Name: " Entity B "},
		},
		Relations: []*types.GraphRelation{
			{Node1: " Entity A ", Node2: " Entity B ", Type: " Relation "},
		},
	}

	if err := validateExtractConfig(config); err != nil {
		t.Fatalf("validateExtractConfig() error = %v", err)
	}
	if config.Text != "custom example" ||
		config.Tags[0] != "Relation" ||
		config.Nodes[0].Name != "Entity A" ||
		config.Relations[0].Node1 != "Entity A" ||
		config.Relations[0].Node2 != "Entity B" ||
		config.Relations[0].Type != "Relation" {
		t.Fatalf("validateExtractConfig() did not trim custom schema: %+v", config)
	}
}
