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
	if !strings.Contains(rendererInjection, `__yunqiaoCodexBridgeInstalled = "`+rendererBridgeVersion+`"`) {
		t.Fatal("renderer bridge version does not match the CDP watchdog")
	}
	if strings.Contains(rendererInjection, "patchModelArray(value, true)") {
		t.Fatal("injection still patches arbitrary arrays")
	}
	for _, forbidden := range []string{"patchGraph", "patchReactState", "window.dispatchEvent =", "__reactFiber", "__reactProps"} {
		if strings.Contains(rendererInjection, forbidden) {
			t.Fatalf("injection still contains unsafe global patch %q", forbidden)
		}
	}
	for _, expected := range []string{"modelResponseLooksPatchable", `String(name) === "107580212"`, "requestIds.size === 0", "appServerPatchDisabled", "use-host-config-"} {
		if !strings.Contains(rendererInjection, expected) {
			t.Fatalf("injection is missing guarded model patch %q", expected)
		}
	}
	if strings.Contains(rendererInjection, `|| document.querySelector("main")`) {
		t.Fatal("image renderer still falls back to the whole main page")
	}
}

func TestRendererHealthExpressionChecksVersionAndHeartbeat(t *testing.T) {
	expression := rendererHealthExpression(3, `model-"quoted"`)
	for _, expected := range []string{rendererBridgeVersion, "s.models===3", "s.heartbeat", `model-\"quoted\"`} {
		if !strings.Contains(expression, expected) {
			t.Fatalf("health expression is missing %q: %s", expected, expression)
		}
	}
}

func TestNativeMenuInjectionUsesElectronMainProcess(t *testing.T) {
	for _, expected := range []string{"process.mainModule", "Menu.setApplicationMenu", `["File", "文件"]`} {
		if !strings.Contains(nativeMenuInjection, expected) {
			t.Fatalf("native menu injection is missing %q", expected)
		}
	}
}
