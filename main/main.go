package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	sshService := NewSSHService()

	app := application.New(application.Options{
		Name:        "LinuxSafeTools",
		Description: "SSH PTY desktop client built with Wails v3",
		Services: []application.Service{
			application.NewService(sshService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	sshService.SetApp(app)

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "LinuxSafeTools",
		Width:            1380,
		Height:           920,
		MinWidth:         1100,
		MinHeight:        760,
		Frameless:        true,
		BackgroundColour: application.NewRGB(13, 19, 33),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
