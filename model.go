package main

import (
	"fmt"
	"strings"
)

const defaultModelSize = "0.8b"
const defaultQuant = "Q4_K_M"

// Model-specific distribution details live here; all sizes share the PLaMo
// translation protocol and llama.cpp lifecycle.
type modelDefinition struct {
	Size       string
	Repo       string
	FilePrefix string
	Quants     []string
}

var modelDefinitions = []modelDefinition{
	{Size: "0.8b", Repo: "pfnet/plamo-3-translate-0.8b-GGUF", FilePrefix: "plamo-3-translate-0.8b", Quants: []string{"Q4_K_M", "Q4_0", "Q3_K_M", "Q2_K"}},
	{Size: "2b", Repo: "pfnet/plamo-3-translate-2b-GGUF", FilePrefix: "plamo-3-translate-2b", Quants: []string{"Q4_K_M", "Q4_0", "Q3_K_M", "Q2_K"}},
	{Size: "8b", Repo: "pfnet/plamo-3-translate-8b-GGUF", FilePrefix: "plamo-3-translate-8b", Quants: []string{"Q4_K_M", "Q3_K_M", "Q2_K"}},
	{Size: "31b", Repo: "pfnet/plamo-3-translate-31b-GGUF", FilePrefix: "plamo-3-translate-31b", Quants: []string{"Q4_K_M", "Q3_K_M", "Q2_K"}},
}

func modelSizes() []string {
	sizes := make([]string, 0, len(modelDefinitions))
	for _, m := range modelDefinitions {
		sizes = append(sizes, m.Size)
	}
	return sizes
}
func lookupModel(size string) (modelDefinition, error) {
	size = strings.ToLower(strings.TrimSpace(size))
	for _, m := range modelDefinitions {
		if m.Size == size {
			return m, nil
		}
	}
	return modelDefinition{}, fmt.Errorf("invalid --size %q (choose %s)", size, strings.Join(modelSizes(), ", "))
}
func (m modelDefinition) filename(quant string) (string, error) {
	for _, q := range m.Quants {
		if q == quant {
			return m.FilePrefix + "-" + quant + ".gguf", nil
		}
	}
	return "", fmt.Errorf("--quant %q is not available for --size %s (choose %s)", quant, m.Size, strings.Join(m.Quants, ", "))
}
func knownQuant(quant string) bool {
	for _, m := range modelDefinitions {
		for _, q := range m.Quants {
			if q == quant {
				return true
			}
		}
	}
	return false
}
