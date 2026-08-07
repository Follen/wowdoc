package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
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

const defaultHotTags = 10

type initRef struct{ commit, requested, tag string }
type initJob struct {
	source  catalog.Source
	product catalog.Product
	refs    []initRef
}
type initSource struct {
	source   catalog.Source
	products []catalog.Product
}
type initFailure struct {
	SourceID string `json:"sourceId"`
	Product  string `json:"product"`
	Ref      string `json:"ref"`
	Code     string `json:"code"`
	Message  string `json:"message"`
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
		manager := gitstore.Manager{Layout: layout, Progress: cmd.ErrOrStderr()}
		ctx, cancel := context.WithTimeout(cmd.Context(), 24*time.Hour)
		defer cancel()
		workerBudget := initWorkerBudget(workers)
		var selected []initSource
		matchedProducts := 0
		for _, source := range sources {
			if sourceID != "" && source.ID != sourceID {
				continue
			}
			var products []catalog.Product
			for _, product := range source.Products {
				if productID == "" || product.ID == productID || product.Branch == productID {
					products = append(products, product)
				}
			}
			if len(products) > 0 {
				matchedProducts += len(products)
				selected = append(selected, initSource{source: source, products: products})
			}
		}
		if matchedProducts == 0 {
			return result.E("source_not_found", "no source/product matched", 2)
		}
		if err = syncInitSources(ctx, manager, selected, workerBudget); err != nil {
			return err
		}
		var jobs []initJob
		for _, selectedSource := range selected {
			for _, product := range selectedSource.products {
				head, e := manager.Head(ctx, selectedSource.source.ID, product.Branch)
				if e != nil {
					return e
				}
				tags, e := manager.ReachableTags(ctx, selectedSource.source.ID, product, hotTags)
				if e != nil {
					return e
				}
				if e = cat.PublishRef(selectedSource.source.ID, product, head, tags); e != nil {
					return e
				}
				refs := []initRef{{head, "latest", ""}}
				seen := map[string]bool{head: true}
				for _, tag := range tags {
					if seen[tag.Commit] {
						continue
					}
					seen[tag.Commit] = true
					refs = append(refs, initRef{tag.Commit, tag.Name, tag.Name})
				}
				jobs = append(jobs, initJob{source: selectedSource.source, product: product, refs: refs})
			}
		}
		built, failures, branchWorkers := runInitJobs(ctx, jobs, workerBudget, func(job initJob, ref initRef, buildWorkers int) (indexer.Stats, error) {
			stats, buildErr := indexer.Build(ctx, indexer.BuildOptions{Layout: layout, SourceID: job.source.ID, ProductID: job.product.ID, Commit: ref.commit, RequestedRef: ref.requested, Tag: ref.tag, Input: &indexer.GitInput{Manager: manager, SourceID: job.source.ID, ProductID: job.product.ID, Commit: ref.commit}, Workers: buildWorkers})
			if buildErr == nil {
				buildErr = cat.SaveSnapshot(store.SnapshotRecord{ID: stats.SnapshotID, SourceID: job.source.ID, ProductID: job.product.ID, Commit: ref.commit, RequestedRef: ref.requested, Tag: ref.tag, Status: "ready", DBPath: stats.DBPath, ManifestPath: stats.ManifestPath, IndexedAt: time.Now().UTC().Format(time.RFC3339)})
			}
			return stats, buildErr
		})
		if len(failures) > 0 {
			e := result.E("initialization_failed", "one or more product branches failed to initialize", 4)
			e.Details = map[string]any{"completedSnapshots": built, "failures": failures, "workerBudget": workerBudget, "branchWorkers": branchWorkers}
			e.NextSteps = []string{"rerun wowdoc init to continue incomplete branches"}
			return e
		}
		ready := map[string]any{"schema": "wowdoc.ready.v1", "ready": true, "completedAt": time.Now().UTC().Format(time.RFC3339), "version": Version}
		raw, _ := json.MarshalIndent(ready, "", "  ")
		if err = os.WriteFile(filepath.Join(layout.State, "ready.json"), raw, 0o644); err != nil {
			return err
		}
		return result.Write(cmd.OutOrStdout(), map[string]any{"home": layout.Root, "ready": true, "gitVersion": gitVersion, "builtSnapshots": built, "workerBudget": workerBudget, "branchWorkers": branchWorkers})
	}}
	cmd.Flags().StringVar(&sourceID, "source", "", "initialize one source")
	cmd.Flags().StringVar(&productID, "product", "", "initialize one product")
	cmd.Flags().IntVar(&hotTags, "hot-tags", defaultHotTags, "maximum reachable product Tags per branch")
	cmd.Flags().IntVar(&workers, "workers", 0, "parser workers (default 4-8)")
	return cmd
}

func syncInitSources(ctx context.Context, manager gitstore.Manager, sources []initSource, workerBudget int) error {
	selected := make([]catalog.Source, 0, len(sources))
	for _, source := range sources {
		selected = append(selected, source.source)
	}
	_, failures := syncSourcesConcurrent(ctx, manager, selected, workerBudget)
	if len(failures) > 0 {
		e := result.E("source_sync_failed", "one or more source mirrors failed to synchronize", 4)
		e.Details = map[string]any{"failures": failures, "concurrency": sourceSyncWorkers(workerBudget, len(selected))}
		e.NextSteps = []string{"rerun wowdoc init to retry failed source mirrors"}
		return e
	}
	return nil
}

type sourceSyncResult struct {
	source catalog.Source
	err    error
}

func sourceSyncWorkers(requested, sources int) int {
	workers := requested
	if workers < 1 {
		workers = 1
	}
	if workers > 3 {
		workers = 3
	}
	if workers > sources {
		workers = sources
	}
	return workers
}

func syncSourcesConcurrent(ctx context.Context, manager gitstore.Manager, sources []catalog.Source, workerBudget int) ([]catalog.Source, []map[string]any) {
	return runSourceSyncJobs(ctx, sources, workerBudget, manager.Sync)
}

func runSourceSyncJobs(ctx context.Context, sources []catalog.Source, workerBudget int, syncSource func(context.Context, catalog.Source) error) ([]catalog.Source, []map[string]any) {
	workers := sourceSyncWorkers(workerBudget, len(sources))
	if workers < 1 {
		return nil, nil
	}
	jobs := make(chan catalog.Source)
	results := make(chan sourceSyncResult, len(sources))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for source := range jobs {
				results <- sourceSyncResult{source: source, err: syncSource(ctx, source)}
			}
		}()
	}
	for _, source := range sources {
		jobs <- source
	}
	close(jobs)
	wg.Wait()
	close(results)
	var succeeded []catalog.Source
	var failures []map[string]any
	for item := range results {
		if item.err == nil {
			succeeded = append(succeeded, item.source)
			continue
		}
		code := "internal_error"
		var appErr *result.Error
		if result.As(item.err, &appErr) {
			code = appErr.Code
		}
		failures = append(failures, map[string]any{"sourceId": item.source.ID, "code": code, "message": item.err.Error()})
	}
	sort.Slice(succeeded, func(i, j int) bool { return succeeded[i].ID < succeeded[j].ID })
	sort.Slice(failures, func(i, j int) bool { return fmt.Sprint(failures[i]["sourceId"]) < fmt.Sprint(failures[j]["sourceId"]) })
	return succeeded, failures
}

func initWorkerBudget(requested int) int {
	if requested > 0 {
		if requested > 8 {
			return 8
		}
		return requested
	}
	workers := runtime.NumCPU()
	if workers < 4 {
		workers = 4
	}
	if workers > 8 {
		workers = 8
	}
	return workers
}

func runInitJobs(ctx context.Context, jobs []initJob, workerBudget int, build func(initJob, initRef, int) (indexer.Stats, error)) ([]indexer.Stats, []initFailure, int) {
	branchWorkers := workerBudget / 2
	if branchWorkers < 1 {
		branchWorkers = 1
	}
	if branchWorkers > 4 {
		branchWorkers = 4
	}
	if branchWorkers > len(jobs) {
		branchWorkers = len(jobs)
	}
	if branchWorkers == 0 {
		return nil, nil, 0
	}
	workersPerBuild := workerBudget / branchWorkers
	if workersPerBuild < 1 {
		workersPerBuild = 1
	}
	jobCh := make(chan initJob)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var built []indexer.Stats
	var failures []initFailure
	for i := 0; i < branchWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				for _, ref := range job.refs {
					stats, buildErr := build(job, ref, workersPerBuild)
					mu.Lock()
					if buildErr != nil {
						code := "internal_error"
						var appErr *result.Error
						if result.As(buildErr, &appErr) {
							code = appErr.Code
						}
						failures = append(failures, initFailure{SourceID: job.source.ID, Product: job.product.ID, Ref: ref.requested, Code: code, Message: buildErr.Error()})
						mu.Unlock()
						break
					}
					built = append(built, stats)
					mu.Unlock()
				}
			}
		}()
	}
	for _, job := range jobs {
		select {
		case jobCh <- job:
		case <-ctx.Done():
			close(jobCh)
			wg.Wait()
			return built, append(failures, initFailure{Code: "operation_cancelled", Message: ctx.Err().Error()}), branchWorkers
		}
	}
	close(jobCh)
	wg.Wait()
	sort.Slice(built, func(i, j int) bool { return built[i].SnapshotID < built[j].SnapshotID })
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].SourceID != failures[j].SourceID {
			return failures[i].SourceID < failures[j].SourceID
		}
		if failures[i].Product != failures[j].Product {
			return failures[i].Product < failures[j].Product
		}
		return failures[i].Ref < failures[j].Ref
	})
	return built, failures, branchWorkers
}

func dataUpdateCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "update", Short: "Update the npm package and bundled Skill", RunE: func(cmd *cobra.Command, args []string) error {
		npm, err := exec.LookPath("npm")
		if err != nil {
			return result.E("npm_not_found", "npm is required for wowdoc update", 4)
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
			return result.Write(cmd.OutOrStdout(), map[string]any{"executed": false, "willRemove": []string{layout.Root, "~/.agents/skills/wowdoc"}, "nextStep": "wowdoc uninstall --yes"})
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
