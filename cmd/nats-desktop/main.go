package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"

	"gioui.org/app"
	"gioui.org/op"

	appinternal "github.com/thedataflows/nats-desktop/internal/application"
)

// VERSION is the application version
// This can be overridden at build time with:
// go build -ldflags "-X main.VERSION=v1.0.0"
var VERSION = "dev"

func main() {
	profiler := os.Getenv("NATS_DESKTOP_PROFILER")
	if profiler != "" {
		go func() {
			log.Println(http.ListenAndServe(profiler, nil))
		}()
	}

	app.ID = "nats-desktop"
	go func() {
		w := new(app.Window)
		w.Option(app.Title("NATS Desktop"), app.Size(1280, 800))
		if err := loop(w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func loop(w *app.Window) error {
	var ops op.Ops
	myApp := appinternal.NewApp(w, VERSION)

	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			appinternal.Layout(gtx, myApp)

			e.Frame(gtx.Ops)
		}
	}
}
