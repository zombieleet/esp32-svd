package main

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// This file adjusts some of the patch files in esp-pacs/**
// It doesn't apply modifications twice, so it can be re-run safely.

var wifiFiles = []string{
	"esp-pacs/esp32/svd/patches/wifi.yaml",
	"esp-pacs/esp32c3/svd/patches/wifi.yaml",
	"esp-pacs/esp32s2/svd/patches/wifi.yaml",
}

// Clusters added without a dimIndex, which gen-device-svd needs to strip
// the %s from the cluster name.
var wifiClusters = []string{
	"FILTER_BANK%s",
	"TX_SLOT_CONFIG%s",
	"CRYPTO_KEY_SLOT%s",
}

func main() {
	exitCode := 0
	for _, filename := range wifiFiles {
		if err := addDimIndex(filename, wifiClusters); err != nil {
			fmt.Fprintln(os.Stderr, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

// addDimIndex adds "dimIndex: 0-<dim-1>" below each named cluster.
func addDimIndex(filename string, clusters []string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := map[string]bool{}
	var out []string
	for i, line := range lines {
		out = append(out, line)
		name, ok := strings.CutSuffix(strings.TrimSpace(line), ":")
		if !ok || !slices.Contains(clusters, name) {
			continue
		}
		found[name] = true
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))] + "    "
		dim := 0
		for _, field := range lines[i+1:] {
			if !strings.HasPrefix(field, indent) {
				break
			}
			field = strings.TrimSpace(field)
			if strings.HasPrefix(field, "dimIndex:") {
				dim = 0
				break
			}
			if v, ok := strings.CutPrefix(field, "dim:"); ok {
				if dim, err = strconv.Atoi(strings.TrimSpace(v)); err != nil {
					return fmt.Errorf("%s: %s: %w", filename, name, err)
				}
			}
		}
		if dim > 0 {
			out = append(out, fmt.Sprintf("%sdimIndex: 0-%d", indent, dim-1))
		}
	}
	for _, name := range clusters {
		if !found[name] {
			return fmt.Errorf("%s: cluster %s not found", filename, name)
		}
	}
	return os.WriteFile(filename, []byte(strings.Join(out, "\n")), 0o644)
}
