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
		`__yunqiaoChineseLocaleStatus`,
		`fallbackInstalled`,
		`process-restart.v1`,
		`uiTranslations`,
		`[data-testid='conversation-turn']`,
		`__YUNQIAO_LOCALIZATION_ONLY__`,
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
	if strings.Contains(rendererInjection, "window.location.reload()") {
		t.Fatal("locale injection still uses a renderer reload instead of requesting a full Codex process restart")
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
	for _, expected := range []string{rendererBridgeVersion, "s.models===3", "s.heartbeat", "l?.installed===true", "l?.active===true", `model-\"quoted\"`} {
		if !strings.Contains(expression, expected) {
			t.Fatalf("health expression is missing %q: %s", expected, expression)
		}
	}
}

func TestNativeMenuInjectionUsesElectronMainProcess(t *testing.T) {
	for _, expected := range []string{"process.mainModule", "globalThis.require", "process.getBuiltinModule", "Menu.setApplicationMenu", `status: "pending"`, `["File", "文件"]`} {
		if !strings.Contains(nativeMenuInjection, expected) {
			t.Fatalf("native menu injection is missing %q", expected)
		}
	}
}

func TestLocalizationOnlyExpressionDoesNotInjectModels(t *testing.T) {
	expression := localizationExpression()
	for _, expected := range []string{"__YUNQIAO_LOCALIZATION_ONLY__=true", "__YUNQIAO_INJECT_MODELS__=[]", rendererBridgeVersion} {
		if !strings.Contains(expression, expected) {
			t.Fatalf("localization-only expression is missing %q", expected)
		}
	}
}
