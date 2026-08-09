package main

import "testing"

func TestWindowLayoutExpandsMainContent(t *testing.T) {
	compact := calculateWindowLayout(620, 650)
	large := calculateWindowLayout(960, 900)
	if large.BaseEdit.Width <= compact.BaseEdit.Width {
		t.Fatal("API field did not expand with window width")
	}
	if large.ModelsList.Height <= compact.ModelsList.Height {
		t.Fatal("model list did not expand with window height")
	}
	if large.StatusEdit.Y <= compact.StatusEdit.Y {
		t.Fatal("bottom controls did not follow window height")
	}
}

func TestWindowLayoutUsesNarrowButtonRows(t *testing.T) {
	layout := calculateWindowLayout(560, 700)
	if layout.UpdateButton.Y <= layout.FetchButton.Y {
		t.Fatal("update button should move to a second row in a narrow window")
	}
	if layout.BaseEdit.X != layout.UpdateButton.X || layout.BaseEdit.Width != layout.UpdateButton.Width {
		t.Fatal("second-row update button should fill the content width")
	}
}

func TestWindowLayoutKeepsBottomControlsInsideClient(t *testing.T) {
	layout := calculateWindowLayout(520, 680)
	if layout.Footer.Y+layout.Footer.Height > 680 {
		t.Fatal("footer exceeds client area")
	}
	if layout.ModelsList.Height < 80 {
		t.Fatal("model list became unusably short")
	}
}

func TestWindowLayoutKeepsAdvertisementUsable(t *testing.T) {
	for _, width := range []int{520, 760, 960} {
		layout := calculateWindowLayout(width, 720)
		if layout.Advertisement.Width < 300 {
			t.Fatalf("advertisement is too narrow at width %d", width)
		}
		if layout.Advertisement.X+layout.Advertisement.Width >= layout.AdvertisementButton.X {
			t.Fatalf("advertisement overlaps detail button at width %d", width)
		}
		if layout.AdvertisementButton.Width < 96 {
			t.Fatalf("advertisement detail button is unusable at width %d", width)
		}
	}
}

func TestWindowLayoutReclaimsSpaceWhenAdvertisementDisabled(t *testing.T) {
	enabled := calculateWindowLayoutWithAdvertisement(760, 720, true)
	disabled := calculateWindowLayoutWithAdvertisement(760, 720, false)
	if disabled.BaseLabel.Y >= enabled.BaseLabel.Y {
		t.Fatal("API controls did not move up when advertisement was disabled")
	}
	if disabled.ModelsList.Height <= enabled.ModelsList.Height {
		t.Fatal("model list did not reclaim disabled advertisement space")
	}
}
