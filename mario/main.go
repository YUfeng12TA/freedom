package main

import (
	_ "embed"
	"freedom"
)

//go:embed index.html
var indexHTML string

func main() {
	app := freedom.New(freedom.Config{
		Title:  "SUPER TIANSHU",
		Width:  790,
		Height: 770,
		Center: true,
		HTML:   func() (string, error) { return indexHTML, nil },
	})
	app.Run()
}
