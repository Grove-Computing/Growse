package ui

import (
	"context"

	"gioui.org/layout"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutengine "github.com/Grove-Computing/Growse/internal/layout"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

type documentRenderKey struct {
	page                                   *browser.Page
	revision                               uint64
	viewportWidth, viewportHeight, pxPerDp float32
	listFirst, listOffset                  int
}

type documentRenderJob struct {
	key         documentRenderKey
	page        *browser.Page
	nested      map[dom.NodeID]layoutengine.ScrollOffset
	build       func(*dom.Document, stylemodel.Map, float32, float32, float32, float32) *layoutengine.Tree
	buildImages func(*dom.Document, stylemodel.Map, map[dom.NodeID]layoutengine.ImageResource, float32, float32, float32, float32) *layoutengine.Tree
	buildFonts  func(*dom.Document, stylemodel.Map, map[dom.NodeID]layoutengine.ImageResource, *layoutengine.FontSet, float32, float32, float32, float32) *layoutengine.Tree
}

type documentRenderResult struct {
	key            documentRenderKey
	tree, baseTree *layoutengine.Tree
	displayList    *paintmodel.DisplayList
	layoutBuilds   int
	valid          bool
}

func (ui *BrowserUI) startRenderWorker() {
	ui.renderContext, ui.cancelRender = context.WithCancel(context.Background())
	ui.renderJobs = make(chan documentRenderJob, 1)
	ui.renderResults = make(chan documentRenderResult, 1)
	go func() {
		for {
			select {
			case <-ui.renderContext.Done():
				return
			case job := <-ui.renderJobs:
				result := buildDocumentRenderJob(job)
				select {
				case ui.renderResults <- result:
				default:
					select {
					case <-ui.renderResults:
					default:
					}
					select {
					case ui.renderResults <- result:
					default:
					}
				}
				ui.invalidate()
			}
		}
	}()
}

func buildDocumentRenderJob(job documentRenderJob) documentRenderResult {
	if job.page == nil {
		return documentRenderResult{key: job.key}
	}
	snapshot, ok := job.page.CaptureRenderSnapshot()
	if !ok {
		return documentRenderResult{key: job.key}
	}
	job.key.revision = snapshot.Revision
	buildTree := func(scrollY float32) *layoutengine.Tree {
		var tree *layoutengine.Tree
		switch {
		case job.buildFonts != nil && (snapshot.ImageResources != nil || snapshot.WebFonts != nil):
			tree = job.buildFonts(snapshot.Document, snapshot.ComputedStyles, snapshot.ImageResources, snapshot.WebFonts, job.key.viewportWidth, job.key.viewportHeight, 0, scrollY)
		case job.buildImages != nil && snapshot.ImageResources != nil:
			tree = job.buildImages(snapshot.Document, snapshot.ComputedStyles, snapshot.ImageResources, job.key.viewportWidth, job.key.viewportHeight, 0, scrollY)
		default:
			builder := job.build
			if builder == nil {
				builder = layoutengine.BuildWithScroll
			}
			tree = builder(snapshot.Document, snapshot.ComputedStyles, job.key.viewportWidth, job.key.viewportHeight, 0, scrollY)
		}
		if tree == nil {
			tree = &layoutengine.Tree{}
		}
		for nodeID, offset := range job.nested {
			layoutengine.ApplyScrollContainerOffset(tree, snapshot.ComputedStyles, nodeID, offset.X, offset.Y)
		}
		tree.Revision = job.key.revision
		return tree
	}
	tree := buildTree(0)
	builds := 1
	baseTree := layoutengine.Clone(tree)
	probe := paintmodel.Build(tree)
	position := layout.Position{First: job.key.listFirst, Offset: job.key.listOffset}
	if position.First >= 0 && position.First < len(probe.Commands) {
		if firstY, ok := commandDocumentY(probe.Commands[position.First]); ok {
			scrollY := max(firstY+float32(position.Offset)/max(job.key.pxPerDp, float32(1)), float32(0))
			if scrollY > 0 {
				tree = buildTree(scrollY)
				builds++
			}
		}
	}
	return documentRenderResult{key: job.key, tree: tree, baseTree: baseTree, displayList: paintmodel.Build(tree), layoutBuilds: builds, valid: true}
}

func sameDocumentRenderView(left, right documentRenderKey) bool {
	return left.page == right.page &&
		left.viewportWidth == right.viewportWidth &&
		left.viewportHeight == right.viewportHeight &&
		left.pxPerDp == right.pxPerDp &&
		left.listFirst == right.listFirst &&
		left.listOffset == right.listOffset
}

func (ui *BrowserUI) cachedDocumentFrameAsync(page *browser.Page, viewportWidth, viewportHeight, pxPerDp float32) (*layoutengine.Tree, *paintmodel.DisplayList, bool) {
	if !ui.asyncRender {
		return ui.cachedDocumentFrame(page, viewportWidth, viewportHeight, pxPerDp)
	}
	position := ui.pageList.Position
	key := documentRenderKey{page: page, revision: page.StyleRevision, viewportWidth: viewportWidth, viewportHeight: viewportHeight, pxPerDp: pxPerDp, listFirst: position.First, listOffset: position.Offset}
	for {
		select {
		case result := <-ui.renderResults:
			if !sameDocumentRenderView(result.key, key) {
				continue
			}
			if sameDocumentRenderView(ui.renderPending, result.key) && ui.renderPending.revision <= result.key.revision {
				ui.renderPending = documentRenderKey{}
			}
			if !result.valid || result.tree == nil {
				continue
			}
			if ui.layoutCache.page == page && ui.layoutCache.revision > result.key.revision {
				continue
			}
			for range result.layoutBuilds {
				page.RecordRenderEvent(browser.RenderLayoutBuild)
			}
			page.RecordRenderEvent(browser.RenderDisplayListBuild)
			page.SyncFrameViewports(result.tree)
			ui.layoutCache = documentLayoutCache{page: page, revision: result.key.revision, viewportWidth: result.key.viewportWidth, viewportHeight: result.key.viewportHeight, listFirst: result.key.listFirst, listOffset: result.key.listOffset, tree: result.tree, baseTree: result.baseTree, displayList: result.displayList}
		default:
			goto drained
		}
	}

drained:
	cache := &ui.layoutCache
	if cache.tree != nil && cache.page == page && cache.revision == key.revision && cache.viewportWidth == viewportWidth && cache.viewportHeight == viewportHeight && cache.listFirst == position.First && cache.listOffset == position.Offset {
		return layoutengine.Clone(cache.tree), cache.displayList, true
	}
	if ui.renderPending != key {
		nested := make(map[dom.NodeID]layoutengine.ScrollOffset, len(ui.nestedScroll))
		for nodeID, offset := range ui.nestedScroll {
			nested[nodeID] = offset
		}
		job := documentRenderJob{key: key, page: page, nested: nested, build: ui.layoutBuild, buildImages: ui.layoutBuildImages, buildFonts: ui.layoutBuildFonts}
		select {
		case ui.renderJobs <- job:
			ui.renderPending = key
			page.RecordRenderRebuild(browser.RenderRebuildInitial)
		default:
			select {
			case <-ui.renderJobs:
			default:
			}
			select {
			case ui.renderJobs <- job:
				ui.renderPending = key
			default:
			}
		}
	}
	if cache.tree != nil && cache.page == page {
		return layoutengine.Clone(cache.tree), cache.displayList, true
	}
	tree := &layoutengine.Tree{Revision: key.revision, Width: viewportWidth, ViewportHeight: viewportHeight, Height: viewportHeight, ScrollWidth: viewportWidth, ScrollHeight: viewportHeight, Parents: map[dom.NodeID]dom.NodeID{}, Bounds: map[dom.NodeID]layoutengine.Rect{}, ScrollContainers: map[dom.NodeID]layoutengine.ScrollContainer{}, ScrollOffsets: map[dom.NodeID]layoutengine.ScrollOffset{}}
	return tree, paintmodel.Build(tree), false
}
