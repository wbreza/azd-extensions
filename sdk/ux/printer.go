package ux

import (
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/wbreza/azd-extensions/sdk/ux/internal"
)

var (
	specialTextRegex = regexp.MustCompile("\x1b\\[[0-9;]*m")
)

type Printer interface {
	internal.Cursor
	Screen

	Fprintf(format string, a ...any)
	Fprintln(a ...any)

	ClearCanvas()

	CursorPosition() CanvasPosition
	SetCursorPosition(position CanvasPosition)
	Size() CanvasSize
}

func NewPrinter(writer io.Writer) Printer {
	if writer == nil {
		writer = os.Stdout
	}

	return &printer{
		Cursor: internal.NewCursor(writer),
		Screen: NewScreen(writer),

		writer:         writer,
		currentLine:    "",
		size:           newCanvasSize(),
		cursorPosition: nil,
	}
}

type printer struct {
	internal.Cursor
	Screen

	writer         io.Writer
	currentLine    string
	size           *CanvasSize
	cursorPosition *CanvasPosition
	clearLock      sync.Mutex
	writeLock      sync.Mutex
}

func (p *printer) Size() CanvasSize {
	return *p.size
}

func (p *printer) CursorPosition() CanvasPosition {
	cursorPosition := CanvasPosition{
		Row: p.size.Rows,
		Col: p.size.Cols,
	}

	log.Printf("Current cursor position: Row: %d, Col: %d\n", cursorPosition.Row, cursorPosition.Col)

	return cursorPosition
}

func (p *printer) MoveCursorToEnd() {
	p.SetCursorPosition(CanvasPosition{
		Row: p.size.Rows,
		Col: p.size.Cols,
	})
}

func (p *printer) SetCursorPosition(position CanvasPosition) {
	// If the cursor is already at the desired position, do nothing
	if p.cursorPosition != nil && *p.cursorPosition == position {
		return
	}

	// If cursorPosition is nil, assume we're already at the bottom-right of the screen
	if p.cursorPosition == nil {
		p.cursorPosition = &CanvasPosition{Row: p.size.Rows, Col: p.size.Cols}
	}

	// Calculate the row and column differences
	rowDiff := position.Row - p.cursorPosition.Row

	// Move vertically if needed
	if rowDiff > 0 {
		p.MoveCursorDown(rowDiff)
	} else if rowDiff < 0 {
		p.MoveCursorUp(int(math.Abs(float64(rowDiff))))
	}

	// Move horizontally if needed
	p.MoveCursorToStartOfLine()
	p.MoveCursorRight(position.Col)

	// Update the stored cursor position
	p.cursorPosition = &position
}

func (p *printer) Fprintf(format string, a ...any) {
	p.writeLock.Lock()
	defer p.writeLock.Unlock()

	content := fmt.Sprintf(format, a...)
	lineCount := strings.Count(content, "\n")

	var lastLine string

	if lineCount > 0 {
		lines := strings.Split(content, "\n")
		lastLine = lines[len(lines)-1]
		p.currentLine = lastLine
	} else {
		lastLine = content
		p.currentLine += lastLine
	}

	fmt.Fprint(p.writer, content)

	p.size.Cols = len(specialTextRegex.ReplaceAllString(p.currentLine, ""))
	p.size.Rows += lineCount

	log.Print(content)
}

func (p *printer) Fprintln(a ...any) {
	p.Fprintf(fmt.Sprintln(a...))
}

func (p *printer) ClearCanvas() {
	log.Println("Clearing canvas")

	p.clearLock.Lock()
	defer p.clearLock.Unlock()

	// 1. Move cursor to the bottom-right corner of the canvas
	p.MoveCursorToEnd()

	// 2. Clear each row from the bottom to the top
	for row := p.size.Rows; row > 0; row-- {
		p.ClearLine()
		if row > 1 { // Avoid moving up if we're on the top row
			p.MoveCursorUp(1)
		}
	}

	// 3. Reset the canvas size
	p.size = newCanvasSize()

	// 4. Clear cursor position
	p.cursorPosition = nil
}

func (p *printer) ClearLine() {
	fmt.Fprint(p.writer, "\033[2K\r")
}
