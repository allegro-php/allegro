package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/allegro-php/allegro/internal/autoloader"
	"github.com/allegro-php/allegro/internal/linker"
	"github.com/allegro-php/allegro/internal/orchestrator"
	"github.com/allegro-php/allegro/internal/store"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify vendor directory integrity",
	RunE:  runVerify,
}

var flagFix bool

func init() {
	verifyCmd.Flags().BoolVar(&flagFix, "fix", false, "Repair issues found")
	rootCmd.AddCommand(verifyCmd)
}

func runVerify(cmd *cobra.Command, args []string) error {
	projectDir, _ := os.Getwd()
	vendorDir := filepath.Join(projectDir, "vendor")
	storePath := store.ResolvePath(flagStorePath, os.Getenv("ALLEGRO_STORE"))
	s := store.New(storePath)

	state, err := linker.ReadVendorState(vendorDir)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "cannot read vendor state: %v\n", err)
		os.Exit(ExitGeneralError)
		return nil
	}

	result, err := orchestrator.VerifyVendor(vendorDir, s, state, ResolveWorkers())
	if err != nil {
		return err
	}

	// Print results
	fmt.Fprintf(cmd.OutOrStdout(), "Checked %d packages (%d files)\n\n", result.TotalPackages, result.TotalFiles)

	if len(result.Issues) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Summary: %d OK, 0 failed\n", result.OKPackages)
		return nil
	}

	for _, issue := range result.Issues {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s/%s — %s\n", issue.Type, issue.Package, issue.File, issue.Detail)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nSummary: %d OK, %d failed\n", result.OKPackages, result.FailPackages)

	if !flagFix {
		fmt.Fprintln(cmd.OutOrStdout(), "Run `allegro verify --fix` to repair")
		os.Exit(ExitGeneralError)
	}

	// --fix mode: repair issues (§5.5)
	fixedCount := 0
	unfixable := 0
	lnk := &linker.CopyLinker{} // fix always uses copy for safety

	for _, issue := range result.Issues {
		switch issue.Type {
		case "missing", "modified":
			// Re-link from CAS
			hash := "" // need to find hash from manifest
			for pkgName, pkgVersion := range state.Packages {
				if pkgName == issue.Package {
					m, err := s.ReadManifest(pkgName, pkgVersion)
					if err != nil {
						unfixable++
						break
					}
					for _, f := range m.Files {
						if f.Path == issue.File {
							h := f.Hash
							if len(h) > 7 && h[:7] == "sha256:" {
								h = h[7:]
							}
							hash = h
							break
						}
					}
					break
				}
			}
			if hash == "" {
				unfixable++
				continue
			}
			srcPath := s.FilePath(hash)
			if _, err := os.Stat(srcPath); os.IsNotExist(err) {
				fmt.Fprintf(cmd.ErrOrStderr(), "  CAS file missing for %s/%s — re-download needed\n", issue.Package, issue.File)
				unfixable++
				continue
			}
			dstPath := filepath.Join(vendorDir, issue.Package, issue.File)
			os.Remove(dstPath)
			os.MkdirAll(filepath.Dir(dstPath), 0755)
			if err := lnk.LinkFile(srcPath, dstPath); err != nil {
				unfixable++
				continue
			}
			os.Chmod(dstPath, 0644)
			fixedCount++

		case "permission":
			dstPath := filepath.Join(vendorDir, issue.Package, issue.File)
			os.Chmod(dstPath, 0644)
			fixedCount++
		}
	}

	// Run dumpautoload after fixes
	noDev := !state.EffectiveDev()
	composerBin, findErr := autoloader.FindComposer(projectDir)
	if findErr == nil {
		autoloader.RunDumpautoload(composerBin, projectDir, false, noDev)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nFixed %d issues, %d unfixable\n", fixedCount, unfixable)
	if unfixable > 0 {
		os.Exit(ExitGeneralError)
	}
	return nil
}
