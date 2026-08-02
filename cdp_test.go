package main

import (
	"strings"
	"testing"
)

func TestInjectionContainsIndependentSentinelAndModelPatch(t *testing.T) {
	for _, expected := range []string{
		"__yunqiaoCodexBridgeInstalled",
		"list-models-for-host",
		"model/list",
		"Response.prototype.json",
		`text === "yunqiao"`,
		`localeOverride`,
		`installStatsigSetter`,
		`localeSyncStarted`,
		`image_generation`,
		`生成的图片`,
		`__yunqiaoAcceptProxyImages`,
		`127.0.0.1:9230/yunqiao/images`,
		`data-testid='conversation-turn'`,
		`.composer-footer`,
		`mountImageCard`,
		`继续对话`,
		`renderedImageKeys`,
	} {
		if !strings.Contains(rendererInjection, expected) {
			t.Fatalf("injection is missing %q", expected)
		}
	}
	if strings.Contains(rendererInjection, "patchModelArray(value, true)") {
		t.Fatal("injection still patches arbitrary arrays")
	}
	if strings.Contains(rendererInjection, `|| document.querySelector("main")`) {
		t.Fatal("image renderer still falls back to the whole main page")
	}
}

func TestNativeMenuInjectionUsesElectronMainProcess(t *testing.T) {
	for _, expected := range []string{"process.mainModule", "Menu.setApplicationMenu", `["File", "文件"]`} {
		if !strings.Contains(nativeMenuInjection, expected) {
			t.Fatalf("native menu injection is missing %q", expected)
		}
	}
}
