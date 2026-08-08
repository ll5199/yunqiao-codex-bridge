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
	layout := calculateWindowLayout(520, 600)
	if layout.Footer.Y+layout.Footer.Height > 600 {
		t.Fatal("footer exceeds client area")
	}
	if layout.ModelsList.Height < 80 {
		t.Fatal("model list became unusably short")
	}
}
