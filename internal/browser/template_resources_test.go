package browser

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestTemplateContentDoesNotParticipateInResourcesOrStyle(t *testing.T) {
	document, err := html.Parse(strings.NewReader(`<html><head><style>.active { color: red }</style></head><body>
		<template><script id="hidden">throw new Error("hidden")</script><style>.active { color: blue }</style><p id="inside" class="active">inside</p></template>
		<p id="outside" class="active">outside</p><script id="visible">console.log("visible")</script></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	scripts := collectScriptsForEngine(document.Root, runtimemodel.EngineJavaScript)
	stylesheets := collectStylesheets(document.Root)
	if len(scripts) != 1 || len(stylesheets) != 1 {
		t.Fatalf("connected resources = scripts:%d styles:%d, want 1 and 1", len(scripts), len(stylesheets))
	}
	if _, exists := document.GetElementByID("inside"); exists {
		t.Fatal("template descendant entered the connected id index")
	}
	outside, ok := document.GetElementByID("outside")
	if !ok {
		t.Fatal("missing connected element")
	}
	computed := style.Compute(document, &css.Stylesheet{})
	if _, exists := computed[outside.ID]; !exists {
		t.Fatal("connected element was not styled")
	}
	for id := range computed {
		if node, ok := document.NodeByID(id); ok {
			if value, _ := node.Attribute("id"); value == "inside" {
				t.Fatal("template descendant participated in style computation")
			}
		}
	}
}
