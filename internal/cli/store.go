package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"github.com/allegro-php/allegro/internal/store"
	"github.com/spf13/cobra"
)

var storeCmd = &cobra.Command{
	Use:   "store",
	Short: "Store management commands",
}

var storeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show store statistics",
	RunE:  runStoreStatus,
}

var storePathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print store directory path",
	Run: func(cmd *cobra.Command, args []string) {
		storePath := store.ResolvePath(flagStorePath, os.Getenv("ALLEGRO_STORE"))
		fmt.Fprintln(cmd.OutOrStdout(), storePath)
	},
}

var storePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove orphaned CAS files",
	RunE:  runStorePrune,
}

func init() {
	storeCmd.AddCommand(storeStatusCmd, storePathCmd, storePruneCmd)
	rootCmd.AddCommand(storeCmd)
}

func runStoreStatus(cmd *cobra.Command, args []string) error {
	storePath := store.ResolvePath(flagStorePath, os.Getenv("ALLEGRO_STORE"))
	s := store.New(storePath)

	filesDir := filepath.Join(s.Root, "files")
	var fileCount int
	var totalSize int64

	filepath.Walk(filesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		fileCount++
		totalSize += info.Size()
		return nil
	})

	packagesDir := filepath.Join(s.Root, "packages")
	var manifestCount int
	filepath.Walk(packagesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		manifestCount++
		return nil
	})

	fmt.Fprintf(cmd.OutOrStdout(), "Store: %s\n", storePath)
	fmt.Fprintf(cmd.OutOrStdout(), "  Files: %s (%s)\n", FormatCommaThousands(fileCount), FormatBinarySize(totalSize))
	fmt.Fprintf(cmd.OutOrStdout(), "  Package manifests: %d\n", manifestCount)
	return nil
}

func runStorePrune(cmd *cobra.Command, args []string) error {
	storePath := store.ResolvePath(flagStorePath, os.Getenv("ALLEGRO_STORE"))
	s := store.New(storePath)

	// Step 1: Enumerate all manifests and collect referenced hashes
	packagesDir := filepath.Join(s.Root, "packages")
	referencedHashes := make(map[string]bool)

	filepath.Walk(packagesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var m store.Manifest
		if err := parseManifestJSON(data, &m); err != nil {
			return nil
		}
		for _, f := range m.Files {
			// Strip "sha256:" prefix if present
			hash := f.Hash
			if len(hash) > 7 && hash[:7] == "sha256:" {
				hash = hash[7:]
			}
			referencedHashes[hash] = true
		}
		return nil
	})

	// Step 2: Enumerate CAS files and delete unreferenced
	filesDir := filepath.Join(s.Root, "files")
	var pruned int

	filepath.Walk(filesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		hash := filepath.Base(path)
		if !referencedHashes[hash] {
			os.Remove(path)
			pruned++
		}
		return nil
	})

	fmt.Fprintf(cmd.OutOrStdout(), "Pruned %d orphaned files\n", pruned)
	return nil
}


func parseManifestJSON(data []byte, m *store.Manifest) error {
	return json.Unmarshal(data, m)
}
