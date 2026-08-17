package main

import "testing"

func TestExternalLayoutKeepsKeyFetchButtonVisible(t *testing.T) {
	for _, size := range [][2]int{{520, 720}, {760, 810}, {1200, 1000}} {
		layout := calculateWindowLayoutForMode(size[0], size[1], true, false)
		if layout.FetchButton.Width < 120 || layout.FetchButton.Height < 32 {
			t.Fatalf("fetch button is unusable at %dx%d: %#v", size[0], size[1], layout.FetchButton)
		}
		if overlaps(layout.KeyEdit, layout.FetchButton) {
			t.Fatalf("key field overlaps fetch button at %dx%d", size[0], size[1])
		}
		assertInside(t, layout.FetchButton, size[0], size[1], "fetch button")
	}
}

func TestAccountLayoutUsesOneStableUsageList(t *testing.T) {
	for _, size := range [][2]int{{520, 720}, {760, 810}, {1200, 1000}} {
		layout := calculateWindowLayoutForMode(size[0], size[1], true, true)
		if layout.UsageList.Height < 72 || layout.UsageList.Width != size[0]-48 {
			t.Fatalf("usage list is not a stable full-width control at %dx%d: %#v", size[0], size[1], layout.UsageList)
		}
		if overlaps(layout.UsageList, layout.StatusTitle) {
			t.Fatalf("usage list overlaps status at %dx%d", size[0], size[1])
		}
		assertInside(t, layout.UsageList, size[0], size[1], "usage list")
	}
}

func TestAccountLoginRowsStayCenteredAndSeparate(t *testing.T) {
	for _, width := range []int{520, 760, 1200} {
		layout := calculateWindowLayoutForMode(width, 810, true, true)
		inputRowLeft := layout.AccountEdit.X
		inputRowRight := layout.PasswordEdit.X + layout.PasswordEdit.Width
		if inputRowLeft != width-inputRowRight {
			t.Fatalf("input row is not centered at width %d", width)
		}
		if overlaps(layout.AccountEdit, layout.PasswordEdit) || overlaps(layout.AccountEdit, layout.LoginButton) || overlaps(layout.PasswordEdit, layout.LogoutButton) {
			t.Fatalf("login rows overlap at width %d", width)
		}
	}
}

func TestVisibleLayoutsDoNotOverlapOrLeaveClient(t *testing.T) {
	for _, accountMode := range []bool{false, true} {
		for _, size := range [][2]int{{520, 720}, {620, 780}, {760, 810}, {960, 900}, {1200, 1000}} {
			layout := calculateWindowLayoutForMode(size[0], size[1], true, accountMode)
			common := []namedRect{
				{"brand", layout.Brand}, {"subtitle", layout.Subtitle},
				{"mode account", layout.ModeAccount}, {"mode external", layout.ModeExternal},
				{"advertisement", layout.Advertisement}, {"consult button", layout.AdvertisementButton},
				{"launch", layout.LaunchButton}, {"native launch", layout.NativeLaunchButton}, {"update", layout.UpdateButton},
				{"status title", layout.StatusTitle}, {"status", layout.StatusEdit},
				{"progress label", layout.ProgressLabel}, {"progress", layout.ProgressBar}, {"footer", layout.Footer},
			}
			modeControls := []namedRect{}
			if accountMode {
				modeControls = []namedRect{
					{"user", layout.AccountEdit}, {"password", layout.PasswordEdit},
					{"login", layout.LoginButton}, {"logout", layout.LogoutButton},
					{"member", layout.MemberInfo}, {"provider label", layout.ProviderLabel},
					{"provider", layout.ProviderCombo}, {"model label", layout.AccountModelLabel},
					{"model", layout.AccountModelCombo}, {"usage title", layout.UsageTitle}, {"usage", layout.UsageList},
				}
			} else {
				modeControls = []namedRect{
					{"base label", layout.BaseLabel}, {"base", layout.BaseEdit},
					{"key label", layout.KeyLabel}, {"key", layout.KeyEdit}, {"fetch", layout.FetchButton},
					{"models label", layout.ModelsLabel}, {"models", layout.ModelsList},
				}
			}
			all := append(common, modeControls...)
			for _, item := range all {
				assertInside(t, item.rect, size[0], size[1], item.name)
			}
			for left := 0; left < len(all); left++ {
				for right := left + 1; right < len(all); right++ {
					if permittedHorizontalPair(all[left].name, all[right].name) {
						continue
					}
					if overlaps(all[left].rect, all[right].rect) {
						t.Fatalf("%s overlaps %s in account=%v at %dx%d", all[left].name, all[right].name, accountMode, size[0], size[1])
					}
				}
			}
		}
	}
}

type namedRect struct {
	name string
	rect controlRect
}

func overlaps(left, right controlRect) bool {
	return left.X < right.X+right.Width && left.X+left.Width > right.X && left.Y < right.Y+right.Height && left.Y+left.Height > right.Y
}

func assertInside(t *testing.T, rect controlRect, width, height int, name string) {
	t.Helper()
	if rect.Width <= 0 || rect.Height <= 0 || rect.X < 0 || rect.Y < 0 || rect.X+rect.Width > width || rect.Y+rect.Height > height {
		t.Fatalf("%s is outside %dx%d: %#v", name, width, height, rect)
	}
}

func permittedHorizontalPair(left, right string) bool {
	// These controls intentionally share a row but their rectangles must still
	// remain separate; no special overlap exception is currently needed.
	return false
}
