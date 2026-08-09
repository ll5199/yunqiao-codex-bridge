package main

type controlRect struct {
	X, Y, Width, Height int
}

type windowLayout struct {
	Brand, Subtitle, Advertisement, AdvertisementButton controlRect
	BaseLabel, BaseEdit, KeyLabel, KeyEdit              controlRect
	FetchButton, LaunchButton, UpdateButton             controlRect
	ModelsLabel, ModelsList, StatusTitle, StatusEdit    controlRect
	ProgressLabel, ProgressBar, Footer                  controlRect
}

func calculateWindowLayout(width, height int) windowLayout {
	if width < 520 {
		width = 520
	}
	if height < 600 {
		height = 600
	}
	margin := 28
	if width < 660 {
		margin = 20
	}
	contentWidth := width - margin*2

	advertisementY := 82
	advertisementHeight := 50
	advertisementButtonWidth := 104
	advertisementGap := 10
	buttonsY := 304
	buttonHeight := 38
	modelsLabelY := 366
	if contentWidth >= 610 {
		gap := 14
		fetchWidth := 132
		launchWidth := 192
		updateWidth := 176
		returnWidth := fetchWidth + launchWidth + updateWidth + gap*2
		if returnWidth > contentWidth {
			updateWidth -= returnWidth - contentWidth
		}
		// Buttons remain left aligned so extra horizontal space goes to the
		// fields and model list, where it is more useful.
		_ = updateWidth
	} else {
		buttonHeight = 34
		modelsLabelY = 326
	}

	statusTitleY := height - 146
	statusEditY := height - 120
	progressLabelY := height - 76
	progressBarY := height - 52
	footerY := height - 26
	modelsTop := modelsLabelY + 26
	modelsHeight := statusTitleY - modelsTop - 18
	if modelsHeight < 86 {
		modelsHeight = 86
	}

	layout := windowLayout{
		Brand:               controlRect{margin, 20, contentWidth, 26},
		Subtitle:            controlRect{margin, 48, contentWidth, 22},
		Advertisement:       controlRect{margin, advertisementY, contentWidth - advertisementButtonWidth - advertisementGap, advertisementHeight},
		AdvertisementButton: controlRect{width - margin - advertisementButtonWidth, advertisementY + 7, advertisementButtonWidth, advertisementHeight - 14},
		BaseLabel:           controlRect{margin, 150, contentWidth, 24},
		BaseEdit:            controlRect{margin, 176, contentWidth, 32},
		KeyLabel:            controlRect{margin, 224, contentWidth, 24},
		KeyEdit:             controlRect{margin, 250, contentWidth, 32},
		ModelsLabel:         controlRect{margin, modelsLabelY, contentWidth, 24},
		ModelsList:          controlRect{margin, modelsTop, contentWidth, modelsHeight},
		StatusTitle:         controlRect{margin, statusTitleY, contentWidth, 22},
		StatusEdit:          controlRect{margin, statusEditY, contentWidth, 32},
		ProgressLabel:       controlRect{margin, progressLabelY, contentWidth, 20},
		ProgressBar:         controlRect{margin, progressBarY, contentWidth, 18},
		Footer:              controlRect{margin, footerY, contentWidth, 20},
	}
	if contentWidth >= 610 {
		gap := 14
		layout.FetchButton = controlRect{margin, buttonsY, 132, buttonHeight}
		layout.LaunchButton = controlRect{margin + 132 + gap, buttonsY, 192, buttonHeight}
		layout.UpdateButton = controlRect{margin + 132 + gap + 192 + gap, buttonsY, 176, buttonHeight}
	} else {
		gap := 10
		firstWidth := (contentWidth - gap) / 2
		layout.FetchButton = controlRect{margin, buttonsY, firstWidth, buttonHeight}
		layout.LaunchButton = controlRect{margin + firstWidth + gap, buttonsY, contentWidth - firstWidth - gap, buttonHeight}
		layout.UpdateButton = controlRect{margin, buttonsY + buttonHeight + 8, contentWidth, buttonHeight}
	}
	return layout
}
