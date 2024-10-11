package ux

import (
	"errors"
	"fmt"
	"os"

	"github.com/fatih/color"
)

var ErrCancelled = errors.New("cancelled by user")

func init() {
	forceColorVal, has := os.LookupEnv("FORCE_COLOR")
	if has && forceColorVal == "1" {
		color.NoColor = false
	}
}

func Hyperlink(url string, text ...string) string {
	if len(text) == 0 {
		text = []string{url}
	}

	return fmt.Sprintf("\033]8;;%s\007%s\033]8;;\007", url, text[0])
}

var BoldString = color.New(color.Bold).SprintfFunc()

func Ptr[T any](value T) *T {
	return &value
}

func Render(renderFn RenderFn) Visual {
	return NewVisualElement(renderFn)
}

type RenderFn func(printer Printer) error
type visualElement struct {
	canvas   Canvas
	renderFn func(printer Printer) error
}

func NewVisualElement(renderFn RenderFn) *visualElement {
	return &visualElement{
		renderFn: renderFn,
	}
}

func (v *visualElement) WithCanvas(canvas Canvas) Visual {
	v.canvas = canvas
	return v
}

func (v *visualElement) Render(printer Printer) error {
	return v.renderFn(printer)
}
