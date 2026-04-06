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

var flagGC bool
var flagDryRunPrune bool

func init() {
	storePruneCmd.Flags().BoolVar(&flagGC, "gc", false, "Full garbage collection with project awareness")
	storePruneCmd.Flags().BoolVar(&flagDryRunPrune, "dry-run", false, "Preview without deleting")
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

	// Phase 2: project registry info (§9.3)
	regPath := store.DefaultRegistryPath()
	reg, _ := store.ReadRegistry(regPath)
	if reg != nil && len(reg.Projects) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Registered projects: %d\n", len(reg.Projects))
	}

	return nil
}


func runStorePrune(cmd *cobra.Command, args []string) error {
	storePath := store.ResolvePath(flagStorePath, os.Getenv("ALLEGRO_STORE"))

	if flagGC {
		// Full GC with project awareness (§9.2)
		regPath := store.DefaultRegistryPath()
		staleDays := 90 // TODO: read from config
		result, err := store.GarbageCollect(storePath, regPath, staleDays, flagDryRunPrune)
		if err != nil {
			return err
		}
		prefix := ""
		if flagDryRunPrune {
			prefix = "[dry-run] "
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%sPruned %d manifests, %d files. %d stale projects warned. %d projects removed.\n",
			prefix, result.ManifestsPruned, result.FilesPruned, result.StaleWarned, result.ProjectsRemoved)
		return nil
	}

	// Phase 1 orphan-only prune
	s := store.New(storePath)
	packagesDir := filepath.Join(s.Root, "packages")
	referencedHashes := make(map[string]bool)

	filepath.Walk(packagesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		data, _ := os.ReadFile(path)
		var m store.Manifest
		if json.Unmarshal(data, &m) != nil {
			return nil
		}
		for _, f := range m.Files {
			hash := f.Hash
			if len(hash) > 7 && hash[:7] == "sha256:" {
				hash = hash[7:]
			}
			referencedHashes[hash] = true
		}
		return nil
	})

	filesDir := filepath.Join(s.Root, "files")
	var pruned int
	filepath.Walk(filesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !referencedHashes[filepath.Base(path)] {
			os.Remove(path)
			pruned++
		}
		return nil
	})

	fmt.Fprintf(cmd.OutOrStdout(), "Pruned %d orphaned files\n", pruned)
	return nil
}
