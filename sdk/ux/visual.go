package ux

type Visual interface {
	Render(printer Printer) error
	WithCanvas(canvas Canvas) Visual
}

type VisualContext struct {
	// The size of the visual
	Size CanvasSize
	// The relative row position of the visual within the canvas
	Top int
}
