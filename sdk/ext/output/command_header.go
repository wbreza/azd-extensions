package output

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

type CommandHeader struct {
	Title       string
	Description string
}

func (ch CommandHeader) Print() {
	color.White(ux.BoldString(ch.Title))
	if ch.Description != "" {
		color.HiBlack(ch.Description)
	}
	fmt.Println()
}
