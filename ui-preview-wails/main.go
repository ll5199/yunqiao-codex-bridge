package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var frontend embed.FS

func main() {
	err := wails.Run(&options.App{
		Title:            "云桥 Codex 客户端 · 界面预览",
		Width:            1120,
		Height:           760,
		MinWidth:         820,
		MinHeight:        620,
		BackgroundColour: &options.RGBA{R: 246, G: 248, B: 245, A: 1},
		AssetServer:      &assetserver.Options{Assets: frontend},
	})
	if err != nil {
		log.Fatal(err)
	}
}
