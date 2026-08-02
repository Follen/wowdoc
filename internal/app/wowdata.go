package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/follenfang/wowdoc/internal/catalog"
	"github.com/follenfang/wowdoc/internal/gitinstall"
	"github.com/follenfang/wowdoc/internal/gitstore"
	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/indexer"
	"github.com/follenfang/wowdoc/internal/result"
	"github.com/follenfang/wowdoc/internal/store"
	"github.com/spf13/cobra"
)

func newWowdata() *cobra.Command {
	root := &cobra.Command{Use: "wowdata", Short: "Manage wowdoc data and installation", Version: Version}
	root.SetVersionTemplate("wowdata {{.Version}}\n")
	root.AddCommand(dataInitCommand(), dataUpdateCommand(), dataCleanCommand(), dataUninstallCommand())
	return root
}

func dataInitCommand() *cobra.Command {
	var sourceID, productID string
	var hotTags, workers int
	cmd := &cobra.Command{Use: "init", Short: "Initialize source mirrors and offline indexes", RunE: func(cmd *cobra.Command, args []string) error {
		layout, err := home.Resolve()
		if err != nil {
			return err
		}
		if err = layout.Ensure(); err != nil {
			return err
		}
		gitVersion, err := gitinstall.Ensure(cmd.Context(), cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		cat, err := store.OpenCatalog(layout)
		if err != nil {
			return err
		}
		defer cat.Close()
		sources := catalog.Sources()
		if err = cat.Seed(sources); err != nil {
			return err
		}
		manager := gitstore.Manager{Layout: layout}
		ctx, cancel := context.WithTimeout(cmd.Context(), 24*time.Hour)
		defer cancel()
		var built []indexer.Stats
		for _, source := range sources {
			if sourceID != "" && source.ID != sourceID {
				continue
			}
			if err = manager.Sync(ctx, source); err != nil {
				return err
			}
			for _, product := range source.Products {
				if productID != "" && product.ID != productID && product.Branch != productID {
					continue
				}
				head, e := manager.Head(ctx, source.ID, product.Branch)
				if e != nil {
					return e
				}
				tags, e := manager.ReachableTags(ctx, source.ID, product, hotTags)
				if e != nil {
					return e
				}
				if e = cat.PublishRef(source.ID, product, head, tags); e != nil {
					return e
				}
				refs := []struct{ commit, requested, tag string }{{head, "latest", ""}}
				seen := map[string]bool{head: true}
				for _, tag := range tags {
					if seen[tag.Commit] {
						continue
					}
					seen[tag.Commit] = true
					refs = append(refs, struct{ commit, requested, tag string }{tag.Commit, tag.Name, tag.Name})
				}
				for _, ref := range refs {
					if existing, ok, e := cat.Snapshot(source.ID, product.ID, ref.commit); e != nil {
						return e
					} else if ok && existing.Status == "ready" {
						continue
					}
					stats, e := indexer.Build(ctx, indexer.BuildOptions{Layout: layout, SourceID: source.ID, ProductID: product.ID, Commit: ref.commit, RequestedRef: ref.requested, Tag: ref.tag, Input: &indexer.GitInput{Manager: manager, SourceID: source.ID, ProductID: product.ID, Commit: ref.commit}, Workers: workers})
					if e != nil {
						return e
					}
					if e = cat.SaveSnapshot(store.SnapshotRecord{ID: stats.SnapshotID, SourceID: source.ID, ProductID: product.ID, Commit: ref.commit, RequestedRef: ref.requested, Tag: ref.tag, Status: "ready", DBPath: stats.DBPath, ManifestPath: stats.ManifestPath, IndexedAt: time.Now().UTC().Format(time.RFC3339)}); e != nil {
						return e
					}
					built = append(built, stats)
				}
			}
		}
		if len(built) == 0 && sourceID != "" {
			return result.E("source_not_found", "no source/product matched", 2)
		}
		ready := map[string]any{"schema": "wowdoc.ready.v1", "ready": true, "completedAt": time.Now().UTC().Format(time.RFC3339), "version": Version}
		raw, _ := json.MarshalIndent(ready, "", "  ")
		if err = os.WriteFile(filepath.Join(layout.State, "ready.json"), raw, 0o644); err != nil {
			return err
		}
		return result.Write(cmd.OutOrStdout(), map[string]any{"home": layout.Root, "ready": true, "gitVersion": gitVersion, "builtSnapshots": built})
	}}
	cmd.Flags().StringVar(&sourceID, "source", "", "initialize one source")
	cmd.Flags().StringVar(&productID, "product", "", "initialize one product")
	cmd.Flags().IntVar(&hotTags, "hot-tags", 50, "maximum reachable product Tags per branch")
	cmd.Flags().IntVar(&workers, "workers", 0, "parser workers (default 4-8)")
	return cmd
}

func dataUpdateCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "update", Short: "Update the npm package and bundled Skill", RunE: func(cmd *cobra.Command, args []string) error {
		npm, err := exec.LookPath("npm")
		if err != nil {
			return result.E("npm_not_found", "npm is required for wowdata update", 4)
		}
		argv := []string{"install", "-g", "@follenfang/wowdoc@latest"}
		if dryRun {
			return result.Write(cmd.OutOrStdout(), map[string]any{"command": append([]string{npm}, argv...), "changesSourceData": false})
		}
		process := exec.Command(npm, argv...)
		process.Stdout = cmd.ErrOrStderr()
		process.Stderr = cmd.ErrOrStderr()
		if err = process.Run(); err != nil {
			return result.E("npm_update_failed", err.Error(), 4)
		}
		return result.Write(cmd.OutOrStdout(), map[string]any{"updated": true, "package": "@follenfang/wowdoc", "changesSourceData": false})
	}}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the npm command without running it")
	return cmd
}

type cleanCandidate struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

func dataCleanCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{Use: "clean", Short: "Preview or remove disposable temporary data", RunE: func(cmd *cobra.Command, args []string) error {
		layout, err := home.Resolve()
		if err != nil {
			return err
		}
		var candidates []cleanCandidate
		var total int64
		for _, root := range []string{layout.Temp} {
			_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return nil
				}
				if !info.IsDir() {
					candidates = append(candidates, cleanCandidate{Path: path, Bytes: info.Size()})
					total += info.Size()
				}
				return nil
			})
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].Path < candidates[j].Path })
		if yes {
			for _, item := range candidates {
				_ = os.Remove(item.Path)
			}
		}
		return result.Write(cmd.OutOrStdout(), map[string]any{"executed": yes, "candidates": candidates, "estimatedBytes": total, "releasedBytes": map[bool]int64{true: total, false: 0}[yes], "preservedSnapshots": true})
	}}
	cmd.Flags().BoolVar(&yes, "yes", false, "delete the listed candidates")
	return cmd
}

func dataUninstallCommand() *cobra.Command {
	var yes, keepPackage bool
	cmd := &cobra.Command{Use: "uninstall", Short: "Remove wowdoc data and optionally the npm package", RunE: func(cmd *cobra.Command, args []string) error {
		layout, err := home.Resolve()
		if err != nil {
			return err
		}
		if !yes {
			return result.Write(cmd.OutOrStdout(), map[string]any{"executed": false, "willRemove": []string{layout.Root, "~/.agents/skills/wowdoc"}, "nextStep": "wowdata uninstall --yes"})
		}
		if err = os.RemoveAll(layout.Root); err != nil {
			return err
		}
		skillHome, homeErr := os.UserHomeDir()
		if homeErr == nil {
			_ = os.RemoveAll(filepath.Join(skillHome, ".agents", "skills", "wowdoc"))
		}
		packageRemoved := false
		if !keepPackage {
			if npm, e := exec.LookPath("npm"); e == nil {
				p := exec.Command(npm, "uninstall", "-g", "@follenfang/wowdoc")
				p.Stdout = cmd.ErrOrStderr()
				p.Stderr = cmd.ErrOrStderr()
				if e = p.Run(); e != nil {
					return result.E("npm_uninstall_failed", e.Error(), 4)
				}
				packageRemoved = true
			}
		}
		return result.Write(cmd.OutOrStdout(), map[string]any{"executed": true, "dataRemoved": true, "skillRemoved": true, "packageRemoved": packageRemoved})
	}}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm removal")
	cmd.Flags().BoolVar(&keepPackage, "keep-package", false, "keep the globally installed npm package")
	return cmd
}

var _ = fmt.Sprintf
