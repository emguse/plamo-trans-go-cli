package main

import (
	"reflect"
	"strings"
	"testing"
)

func modelForTest(t *testing.T, size string) modelDefinition {
	t.Helper()
	m, err := lookupModel(size)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func TestModelDefinitions(t *testing.T) {
	cases := []struct {
		size, repo, prefix string
		quants             []string
	}{
		{"0.8b", "pfnet/plamo-3-translate-0.8b-GGUF", "plamo-3-translate-0.8b", []string{"Q4_K_M", "Q4_0", "Q3_K_M", "Q2_K"}},
		{"2b", "pfnet/plamo-3-translate-2b-GGUF", "plamo-3-translate-2b", []string{"Q4_K_M", "Q4_0", "Q3_K_M", "Q2_K"}},
		{"8b", "pfnet/plamo-3-translate-8b-GGUF", "plamo-3-translate-8b", []string{"Q4_K_M", "Q3_K_M", "Q2_K"}},
		{"31b", "pfnet/plamo-3-translate-31b-GGUF", "plamo-3-translate-31b", []string{"Q4_K_M", "Q3_K_M", "Q2_K"}},
	}
	for _, tc := range cases {
		t.Run(tc.size, func(t *testing.T) {
			m := modelForTest(t, tc.size)
			if m.Repo != tc.repo || m.FilePrefix != tc.prefix || !reflect.DeepEqual(m.Quants, tc.quants) {
				t.Fatalf("unexpected definition: %+v", m)
			}
			for _, q := range tc.quants {
				name, err := m.filename(q)
				if err != nil || name != tc.prefix+"-"+q+".gguf" {
					t.Fatal(name, err)
				}
			}
			if _, err := m.filename("unknown"); err == nil {
				t.Fatal("accepted unsupported quantization")
			}
			if modelForTest(t, strings.ToUpper(tc.size)).Size != tc.size {
				t.Fatal("size is not normalized")
			}
		})
	}
	if !reflect.DeepEqual(modelSizes(), []string{"0.8b", "2b", "8b", "31b"}) {
		t.Fatal(modelSizes())
	}
	if _, err := lookupModel("unknown"); err == nil {
		t.Fatal("accepted unknown size")
	}
	for _, size := range []string{"8b", "31b"} {
		if _, err := modelForTest(t, size).filename("Q4_0"); err == nil {
			t.Fatal("accepted unavailable Q4_0", size)
		}
	}
}
