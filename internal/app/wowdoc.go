package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/follenfang/wowdoc/internal/catalog"
	"github.com/follenfang/wowdoc/internal/gitstore"
	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/indexer"
	"github.com/follenfang/wowdoc/internal/query"
	"github.com/follenfang/wowdoc/internal/result"
	"github.com/follenfang/wowdoc/internal/store"
	"github.com/spf13/cobra"
	"github.com/yuin/gopher-lua/parse"
)

func newWowdoc() *cobra.Command {
	root := &cobra.Command{Use: "wowdoc", Short: "Versioned WoW UI source intelligence for agents", Version: Version}
	root.SetVersionTemplate("wowdoc {{.Version}}\n")
	root.AddCommand(dataInitCommand(), dataUpdateCommand(), dataCleanCommand(), dataUninstallCommand(), doctorCommand(), sourceCommand(), indexCommand(), searchCommand("query"), searchCommand("explore"), inspectCommand(), diffCommand(), validateCommand())
	return root
}

func doctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the local installation without changing it",
		RunE: func(cmd *cobra.Command, args []string) error {
			layout, err := home.Resolve()
			if err != nil {
				return err
			}
			checks := []map[string]any{}
			add := func(name string, ok bool, detail, next string) {
				row := map[string]any{"name": name, "ok": ok, "detail": detail}
				if !ok && next != "" {
					row["nextStep"] = next
				}
				checks = append(checks, row)
			}
			_, gitErr := exec.LookPath("git")
			add("git", gitErr == nil, fmt.Sprint(gitErr), "install git and ensure it is on PATH")
			probePath := layout.Root
			info, pathErr := os.Stat(probePath)
			if os.IsNotExist(pathErr) {
				probePath = filepath.Dir(probePath)
				info, pathErr = os.Stat(probePath)
			}
			writableHint := pathErr == nil && info.IsDir() && info.Mode().Perm()&0o200 != 0
			add("homeWritableHint", writableHint, probePath, "check permissions for "+probePath)
			add("initialized", layout.Initialized(), layout.State, "wowdoc init")
			healthy := gitErr == nil && writableHint
			return result.Write(cmd.OutOrStdout(), map[string]any{"version": Version, "commit": Commit, "go": runtime.Version(), "platform": runtime.GOOS + "/" + runtime.GOARCH, "home": layout.Root, "healthy": healthy, "checks": checks})
		},
	}
}

func sourceCommand() *cobra.Command {
	root := &cobra.Command{Use: "source", Short: "Inspect and synchronize source repositories"}
	var sourceID, productID string
	list := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, args []string) error {
		layout, err := home.Resolve()
		if err != nil {
			return err
		}
		data := map[string]any{"catalog": catalog.Sources(), "initialized": layout.Initialized()}
		if layout.Initialized() {
			cat, e := store.OpenCatalogRead(layout)
			if e != nil {
				return e
			}
			defer cat.Close()
			refs, e := cat.Refs(sourceID)
			if e != nil {
				return e
			}
			data["refs"] = refs
			if sourceID != "" && productID != "" {
				tags, e := cat.Tags(sourceID, productID)
				if e != nil {
					return e
				}
				data["tags"] = tags
			}
		}
		return result.Write(cmd.OutOrStdout(), data)
	}}
	list.Flags().StringVar(&sourceID, "source", "", "source id")
	list.Flags().StringVar(&productID, "product", "", "product id")
	var checkSource, checkProduct string
	check := &cobra.Command{Use: "check", RunE: func(cmd *cobra.Command, args []string) error {
		if err := require(checkSource, "source_required", "--source is required"); err != nil {
			return err
		}
		source, ok := catalog.FindSource(checkSource)
		if !ok {
			return result.E("source_not_found", "unknown source: "+checkSource, 2)
		}
		product, ok := catalog.FindProduct(source, checkProduct)
		if !ok {
			return result.E("unsupported_build", "unknown product: "+checkProduct, 3)
		}
		layout, err := home.Resolve()
		if err != nil {
			return err
		}
		ctx, cancel := gitstore.Context()
		defer cancel()
		value, err := (gitstore.Manager{Layout: layout}).Check(ctx, source, product)
		return writeResult(cmd, value, err)
	}}
	check.Flags().StringVar(&checkSource, "source", "", "source id")
	check.Flags().StringVar(&checkProduct, "product", "", "product id")
	var syncSource, syncProduct string
	sync := &cobra.Command{Use: "sync", RunE: func(cmd *cobra.Command, args []string) error {
		return syncSources(cmd.Context(), cmd, syncSource, syncProduct)
	}}
	sync.Flags().StringVar(&syncSource, "source", "", "source id")
	sync.Flags().StringVar(&syncProduct, "product", "", "product id")
	root.AddCommand(list, check, sync)
	return root
}

func syncSources(parent context.Context, cmd *cobra.Command, sourceID, productID string) error {
	layout, err := home.Resolve()
	if err != nil {
		return err
	}
	if err = layout.Ensure(); err != nil {
		return err
	}
	cat, err := store.OpenCatalog(layout)
	if err != nil {
		return err
	}
	defer cat.Close()
	if err = cat.Seed(catalog.Sources()); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
	defer cancel()
	manager := gitstore.Manager{Layout: layout, Progress: cmd.ErrOrStderr()}
	var selected []catalog.Source
	for _, source := range catalog.Sources() {
		if sourceID == "" || source.ID == sourceID {
			selected = append(selected, source)
		}
	}
	if len(selected) == 0 {
		return result.E("source_not_found", "no source/product matched", 2)
	}
	succeeded, failures := syncSourcesConcurrent(ctx, manager, selected, 3)
	var synced []map[string]any
	for _, source := range succeeded {
		for _, product := range source.Products {
			if productID != "" && product.ID != productID && product.Branch != productID {
				continue
			}
			head, e := manager.Head(ctx, source.ID, product.Branch)
			if e != nil {
				return e
			}
			tags, e := manager.ReachableTags(ctx, source.ID, product, 50)
			if e != nil {
				return e
			}
			if e = cat.PublishRef(source.ID, product, head, tags); e != nil {
				return e
			}
			synced = append(synced, map[string]any{"sourceId": source.ID, "product": product.ID, "branch": product.Branch, "resolvedCommit": head, "hotTags": len(tags)})
		}
	}
	if len(failures) > 0 {
		e := result.E("source_sync_failed", "one or more source mirrors failed to synchronize", 4)
		e.Details = map[string]any{"synced": synced, "failures": failures, "concurrency": sourceSyncWorkers(3, len(selected))}
		e.NextSteps = []string{"rerun wowdoc source sync to retry failed source mirrors"}
		return e
	}
	if len(synced) == 0 {
		return result.E("source_not_found", "no source/product matched", 2)
	}
	return result.Write(cmd.OutOrStdout(), map[string]any{"synced": synced})
}

func indexCommand() *cobra.Command {
	root := &cobra.Command{Use: "index", Short: "Build and inspect immutable source indexes"}
	root.AddCommand(indexBuildCommand("build"), indexBuildCommand("refresh"))
	var sourceID, productID string
	status := &cobra.Command{Use: "status", RunE: func(cmd *cobra.Command, args []string) error {
		if err := require(sourceID, "source_required", "--source is required"); err != nil {
			return err
		}
		if err := require(productID, "product_required", "--product is required"); err != nil {
			return err
		}
		layout, err := home.Resolve()
		if err != nil {
			return err
		}
		value, err := query.Status(layout, sourceID, productID)
		return writeResult(cmd, value, err)
	}}
	status.Flags().StringVar(&sourceID, "source", "", "source id")
	status.Flags().StringVar(&productID, "product", "", "product id")
	root.AddCommand(status)
	return root
}

func indexBuildCommand(name string) *cobra.Command {
	var sourceID, productID, ref, sourcePath string
	var workers int
	cmd := &cobra.Command{Use: name, RunE: func(cmd *cobra.Command, args []string) error {
		if err := require(sourceID, "source_required", "--source is required"); err != nil {
			return err
		}
		if err := require(productID, "product_required", "--product is required"); err != nil {
			return err
		}
		layout, err := home.Resolve()
		if err != nil {
			return err
		}
		if err = layout.Ensure(); err != nil {
			return err
		}
		cat, err := store.OpenCatalog(layout)
		if err != nil {
			return err
		}
		defer cat.Close()
		if err = cat.Seed(catalog.Sources()); err != nil {
			return err
		}
		source, ok := catalog.FindSource(sourceID)
		if !ok {
			return result.E("source_not_found", "unknown source: "+sourceID, 2)
		}
		product, ok := catalog.FindProduct(source, productID)
		if !ok {
			return result.E("unsupported_build", "unknown product: "+productID, 3)
		}
		var input indexer.Input
		var commit, tag string
		if sourcePath != "" {
			abs, e := filepath.Abs(sourcePath)
			if e != nil {
				return e
			}
			commit = shortHash(abs)
			input = indexer.DirectoryInput{Root: abs}
		} else {
			commit, tag, err = cat.Resolve(source, product, ref)
			if err != nil {
				return result.E("version_not_found", "sync the source or use an exact available Tag", 3)
			}
			manager := gitstore.Manager{Layout: layout}
			input = &indexer.GitInput{Manager: manager, SourceID: source.ID, ProductID: product.ID, Commit: commit}
		}
		stats, err := indexer.Build(cmd.Context(), indexer.BuildOptions{Layout: layout, SourceID: source.ID, ProductID: product.ID, Commit: commit, RequestedRef: ref, Tag: tag, Input: input, Workers: workers})
		if err != nil {
			return err
		}
		record := store.SnapshotRecord{ID: stats.SnapshotID, SourceID: source.ID, ProductID: product.ID, Commit: commit, RequestedRef: ref, Tag: tag, Status: "ready", DBPath: stats.DBPath, ManifestPath: stats.ManifestPath, IndexedAt: time.Now().UTC().Format(time.RFC3339)}
		if err = cat.SaveSnapshot(record); err != nil {
			return err
		}
		return result.Write(cmd.OutOrStdout(), stats)
	}}
	cmd.Flags().StringVar(&sourceID, "source", "", "source id")
	cmd.Flags().StringVar(&productID, "product", "", "product id")
	cmd.Flags().StringVar(&ref, "ref", "latest", "Tag, version, Commit, or latest")
	cmd.Flags().StringVar(&sourcePath, "source-path", "", "fixture source directory")
	cmd.Flags().IntVar(&workers, "workers", 0, "parser workers (default 4-8)")
	return cmd
}

func searchCommand(name string) *cobra.Command {
	var sourceID, productID, ref, text, topic string
	var limit int
	cmd := &cobra.Command{Use: name, RunE: func(cmd *cobra.Command, args []string) error {
		if err := require(text, "query_required", "--text is required"); err != nil {
			return err
		}
		sel, err := selectSnapshot(sourceID, productID, ref)
		if err != nil {
			return err
		}
		defer sel.cat.Close()
		value, err := query.Search(sel.layout, sel.ctx, text, topic, limit)
		return writeResult(cmd, value, err)
	}}
	cmd.Flags().StringVar(&sourceID, "source", "", "source id")
	cmd.Flags().StringVar(&productID, "product", "", "product id")
	cmd.Flags().StringVar(&ref, "ref", "latest", "Tag, version, Commit, or latest")
	cmd.Flags().StringVar(&text, "text", "", "symbol or search text")
	cmd.Flags().StringVar(&topic, "topic", "", "api, lua, xml, toc, or asset")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum results")
	return cmd
}

func inspectCommand() *cobra.Command {
	var sourceID, productID, ref, symbol, path string
	cmd := &cobra.Command{Use: "inspect", RunE: func(cmd *cobra.Command, args []string) error {
		if symbol == "" && path == "" {
			return result.E("target_required", "--symbol or --path is required", 2)
		}
		sel, err := selectSnapshot(sourceID, productID, ref)
		if err != nil {
			return err
		}
		defer sel.cat.Close()
		value, err := query.Inspect(sel.layout, sel.ctx, symbol, path)
		return writeResult(cmd, value, err)
	}}
	cmd.Flags().StringVar(&sourceID, "source", "", "source id")
	cmd.Flags().StringVar(&productID, "product", "", "product id")
	cmd.Flags().StringVar(&ref, "ref", "latest", "Tag, version, Commit, or latest")
	cmd.Flags().StringVar(&symbol, "symbol", "", "qualified symbol")
	cmd.Flags().StringVar(&path, "path", "", "repository path")
	return cmd
}

func diffCommand() *cobra.Command {
	var sourceID, productID, from, to string
	cmd := &cobra.Command{Use: "diff", RunE: func(cmd *cobra.Command, args []string) error {
		a, err := selectSnapshot(sourceID, productID, from)
		if err != nil {
			return err
		}
		defer a.cat.Close()
		b, err := selectSnapshot(sourceID, productID, to)
		if err != nil {
			return err
		}
		defer b.cat.Close()
		value, err := query.Diff(a.layout, a.ctx, b.ctx)
		return writeResult(cmd, value, err)
	}}
	cmd.Flags().StringVar(&sourceID, "source", "", "source id")
	cmd.Flags().StringVar(&productID, "product", "", "product id")
	cmd.Flags().StringVar(&from, "from", "", "from ref")
	cmd.Flags().StringVar(&to, "to", "", "to ref")
	return cmd
}

func validateCommand() *cobra.Command {
	var path, sourceID, productID, ref string
	cmd := &cobra.Command{Use: "validate", RunE: func(cmd *cobra.Command, args []string) error {
		if err := require(path, "path_required", "--path is required"); err != nil {
			return err
		}
		var diagnostics []result.Diagnostic
		files := 0
		err := filepath.WalkDir(path, func(p string, e os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if e.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if ext == ".lua" {
				files++
				data, readErr := os.ReadFile(p)
				if readErr != nil {
					return readErr
				}
				if _, parseErr := parse.Parse(bytes.NewReader(data), p); parseErr != nil {
					diagnostics = append(diagnostics, result.Diagnostic{Code: "lua_parse_failed", Message: parseErr.Error(), Path: p})
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		return result.Write(cmd.OutOrStdout(), map[string]any{"path": path, "sourceId": sourceID, "product": productID, "ref": ref, "checkedLua": files, "valid": len(diagnostics) == 0, "diagnostics": diagnostics})
	}}
	cmd.Flags().StringVar(&path, "path", "", "AddOn directory")
	cmd.Flags().StringVar(&sourceID, "source", "", "target source id")
	cmd.Flags().StringVar(&productID, "product", "", "target product id")
	cmd.Flags().StringVar(&ref, "ref", "latest", "target ref")
	return cmd
}

func writeResult(cmd *cobra.Command, value any, err error) error {
	if err != nil {
		return err
	}
	return result.Write(cmd.OutOrStdout(), value)
}
func shortHash(v string) string {
	var x uint64 = 1469598103934665603
	for i := 0; i < len(v); i++ {
		x ^= uint64(v[i])
		x *= 1099511628211
	}
	return fmt.Sprintf("%040x", x)
}

var _ = json.Marshal
