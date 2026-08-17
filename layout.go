package main

type controlRect struct {
	X, Y, Width, Height int
}

type windowLayout struct {
	Brand, Subtitle, ModeAccount, ModeExternal                  controlRect
	Advertisement, AdvertisementButton                          controlRect
	BaseLabel, BaseEdit, KeyLabel, KeyEdit                      controlRect
	FetchButton, LaunchButton, NativeLaunchButton, UpdateButton controlRect
	ModelsLabel, ModelsList                                     controlRect
	AccountEdit, PasswordEdit, LoginButton, LogoutButton        controlRect
	MemberInfo, ProviderLabel, ProviderCombo                    controlRect
	AccountModelLabel, AccountModelCombo                        controlRect
	UsageTitle, UsageList                                       controlRect
	StatusTitle, StatusEdit, ProgressLabel, ProgressBar         controlRect
	Footer                                                      controlRect
}

func calculateWindowLayout(width, height int) windowLayout {
	return calculateWindowLayoutForMode(width, height, true, false)
}

func calculateWindowLayoutWithAdvertisement(width, height int, advertisementEnabled bool) windowLayout {
	return calculateWindowLayoutForMode(width, height, advertisementEnabled, false)
}

func calculateWindowLayoutForMode(width, height int, advertisementEnabled, accountMode bool) windowLayout {
	if width < 520 {
		width = 520
	}
	if height < 720 {
		height = 720
	}
	margin := 24
	contentWidth := width - margin*2
	gap := 12
	buttonWidth := (contentWidth - gap*2) / 3
	if buttonWidth > 170 {
		buttonWidth = 170
	}
	buttonRowWidth := buttonWidth*3 + gap*2
	buttonX := margin + (contentWidth-buttonRowWidth)/2

	statusTitleY := height - 144
	statusEditY := height - 120
	progressLabelY := height - 76
	progressBarY := height - 52
	footerY := height - 26

	layout := windowLayout{
		Brand:         controlRect{margin, 18, contentWidth, 26},
		Subtitle:      controlRect{margin, 46, contentWidth, 22},
		ModeAccount:   controlRect{margin, 78, 112, 30},
		ModeExternal:  controlRect{margin + 126, 78, 112, 30},
		StatusTitle:   controlRect{margin, statusTitleY, contentWidth, 22},
		StatusEdit:    controlRect{margin, statusEditY, contentWidth, 32},
		ProgressLabel: controlRect{margin, progressLabelY, contentWidth, 20},
		ProgressBar:   controlRect{margin, progressBarY, contentWidth, 18},
		Footer:        controlRect{margin, footerY, contentWidth, 20},
	}

	adButtonWidth := 142
	adGap := 10
	if accountMode {
		loginWidth := contentWidth
		if loginWidth > 650 {
			loginWidth = 650
		}
		loginX := margin + (contentWidth-loginWidth)/2
		inputGap := 10
		inputWidth := (loginWidth - inputGap) / 2
		loginButtonWidth := 126
		loginButtonsWidth := loginButtonWidth*2 + inputGap
		loginButtonX := margin + (contentWidth-loginButtonsWidth)/2
		comboWidth := (contentWidth - gap) / 2
		layout.AccountEdit = controlRect{loginX, 120, inputWidth, 34}
		layout.PasswordEdit = controlRect{loginX + inputWidth + inputGap, 120, loginWidth - inputWidth - inputGap, 34}
		layout.LoginButton = controlRect{loginButtonX, 164, loginButtonWidth, 34}
		layout.LogoutButton = controlRect{loginButtonX + loginButtonWidth + inputGap, 164, loginButtonWidth, 34}
		layout.MemberInfo = controlRect{margin, 208, contentWidth, 28}
		layout.ProviderLabel = controlRect{margin, 246, comboWidth, 20}
		layout.AccountModelLabel = controlRect{margin + comboWidth + gap, 246, comboWidth, 20}
		layout.ProviderCombo = controlRect{margin, 268, comboWidth, 34}
		layout.AccountModelCombo = controlRect{margin + comboWidth + gap, 268, comboWidth, 34}
		adY := 318
		layout.Advertisement = controlRect{margin, adY, contentWidth - adButtonWidth - adGap, 48}
		layout.AdvertisementButton = controlRect{width - margin - adButtonWidth, adY + 6, adButtonWidth, 36}
		layout.LaunchButton = controlRect{buttonX, 382, buttonWidth, 38}
		layout.NativeLaunchButton = controlRect{buttonX + buttonWidth + gap, 382, buttonWidth, 38}
		layout.UpdateButton = controlRect{buttonX + (buttonWidth+gap)*2, 382, buttonWidth, 38}
		layout.UsageTitle = controlRect{margin, 440, contentWidth, 22}
		// A single list owns all three rows. Windows can resize this control
		// without independently wrapping or repositioning row text.
		layout.UsageList = controlRect{margin, 466, contentWidth, 82}
	} else {
		layout.BaseLabel = controlRect{margin, 120, contentWidth, 22}
		layout.BaseEdit = controlRect{margin, 144, contentWidth, 34}
		layout.KeyLabel = controlRect{margin, 190, contentWidth, 22}
		fetchWidth := 142
		layout.KeyEdit = controlRect{margin, 214, contentWidth - fetchWidth - gap, 34}
		layout.FetchButton = controlRect{width - margin - fetchWidth, 214, fetchWidth, 34}
		layout.ModelsLabel = controlRect{margin, 262, contentWidth, 22}
		layout.ModelsList = controlRect{margin, 286, contentWidth, 116}
		adY := 418
		layout.Advertisement = controlRect{margin, adY, contentWidth - adButtonWidth - adGap, 48}
		layout.AdvertisementButton = controlRect{width - margin - adButtonWidth, adY + 6, adButtonWidth, 36}
		layout.LaunchButton = controlRect{buttonX, 482, buttonWidth, 38}
		layout.NativeLaunchButton = controlRect{buttonX + buttonWidth + gap, 482, buttonWidth, 38}
		layout.UpdateButton = controlRect{buttonX + (buttonWidth+gap)*2, 482, buttonWidth, 38}
	}
	if !advertisementEnabled {
		layout.Advertisement = controlRect{}
		layout.AdvertisementButton = controlRect{}
	}
	return layout
}
