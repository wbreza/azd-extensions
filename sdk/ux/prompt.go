package ux

import (
	"io"
	"os"

	"github.com/wbreza/azd-extensions/sdk/ux/internal"

	"dario.cat/mergo"
	"github.com/eiannone/keyboard"
	"github.com/fatih/color"
)

type PromptOptions struct {
	// The writer to use for output (default: os.Stdout)
	Writer io.Writer
	// The reader to use for input (default: os.Stdin)
	Reader io.Reader
	// The default value to use for the prompt (default: "")
	DefaultValue string
	// The message to display before the prompt
	Message string
	// The optional message to display when the user types ? (default: "")
	HelpMessage string
	// The optional hint text that display after the message (default: "[Type ? for hint]")
	Hint string
	// The optional placeholder text to display when the value is empty (default: "")
	PlaceHolder string
	// The optional validation function to use
	ValidationFn func(string) (bool, string)
	// The optional validation message to display when validation fails (default: "Invalid input")
	ValidationMessage string
	// The optional validation message to display when the value is empty and required (default: "This field is required")
	RequiredMessage string
	// Whether or not the prompt is required (default: false)
	Required bool
	// Whether or not to clear the prompt after completion (default: false)
	ClearOnCompletion bool
	// Whether or not to capture hint keys (default: true)
	IgnoreHintKeys bool
}

var DefaultPromptOptions PromptOptions = PromptOptions{
	Writer:            os.Stdout,
	Reader:            os.Stdin,
	Required:          false,
	ValidationMessage: "Invalid input",
	RequiredMessage:   "This field is required",
	Hint:              "[Type ? for hint]",
	ClearOnCompletion: false,
	IgnoreHintKeys:    false,
	ValidationFn: func(input string) (bool, string) {
		return true, ""
	},
}

type Prompt struct {
	input *internal.Input

	canvas             Canvas
	options            *PromptOptions
	hasValidationError bool
	value              string
	showHelp           bool
	complete           bool
	submitted          bool
	validationMessage  string
	cancelled          bool
	cursorPosition     *CursorPosition
}

func NewPrompt(options *PromptOptions) *Prompt {
	mergedOptions := PromptOptions{}
	if err := mergo.Merge(&mergedOptions, DefaultPromptOptions, mergo.WithoutDereference); err != nil {
		panic(err)
	}

	if err := mergo.Merge(&mergedOptions, options, mergo.WithoutDereference); err != nil {
		panic(err)
	}

	return &Prompt{
		input:   internal.NewInput(),
		options: &mergedOptions,
		value:   mergedOptions.DefaultValue,
	}
}

func (p *Prompt) validate() {
	p.hasValidationError = false
	p.validationMessage = p.options.ValidationMessage

	if p.options.Required && p.value == "" {
		p.hasValidationError = true
		p.validationMessage = p.options.RequiredMessage
		return
	}

	if p.options.ValidationFn != nil {
		ok, msg := p.options.ValidationFn(p.value)
		if !ok {
			p.hasValidationError = true
			if msg != "" {
				p.validationMessage = msg
			}
		}
	}
}

func (p *Prompt) WithCanvas(canvas Canvas) Visual {
	p.canvas = canvas
	return p
}

func (p *Prompt) Ask() (string, error) {
	if p.canvas == nil {
		p.canvas = NewCanvas(p).WithWriter(p.options.Writer)
	}

	if err := p.canvas.Run(); err != nil {
		return "", err
	}

	inputOptions := &internal.InputConfig{
		InitialValue:   p.options.DefaultValue,
		IgnoreHintKeys: p.options.IgnoreHintKeys,
	}
	input, done, err := p.input.ReadInput(inputOptions)
	if err != nil {
		return "", err
	}

	for {
		select {
		case <-p.input.SigChan:
			p.cancelled = true
			done()
			p.canvas.Update()
			return "", ErrCancelled

		case msg := <-input:
			p.showHelp = msg.Hint
			p.value = msg.Value

			p.validate()

			if msg.Key == keyboard.KeyEnter {
				p.submitted = true

				if !p.hasValidationError {
					p.complete = true
				}
			}

			p.canvas.Update()

			if p.complete {
				done()
				return p.value, nil
			}
		}
	}
}

func (p *Prompt) Render(printer Printer) error {
	if p.options.ClearOnCompletion && p.complete {
		return nil
	}

	printer.Fprintf(color.CyanString("? "))

	// Message
	printer.Fprintf(BoldString("%s: ", p.options.Message))

	// Cancelled
	if p.cancelled {
		printer.Fprintln(color.HiRedString("(Cancelled)"))
		return nil
	}

	// Hint (Only show when a help message has been defined)
	if !p.complete && p.options.Hint != "" && p.options.HelpMessage != "" {
		printer.Fprintf("%s ", color.CyanString(p.options.Hint))
	}

	// Placeholder
	if p.value == "" && p.options.PlaceHolder != "" {
		p.cursorPosition = Ptr(printer.CursorPosition())
		printer.Fprintf(color.HiBlackString(p.options.PlaceHolder))
	}

	// Value
	if p.value != "" {
		valueOutput := p.value

		if p.complete || p.value == p.options.DefaultValue {
			valueOutput = color.CyanString(p.value)
		}

		printer.Fprintf(valueOutput)
		p.cursorPosition = Ptr(printer.CursorPosition())
	}

	// Done
	if p.complete {
		printer.Fprintln()
		return nil
	}

	// Validation error
	if !p.showHelp && p.submitted && p.hasValidationError {
		printer.Fprintln()
		printer.Fprintln(color.YellowString(p.validationMessage))
	}

	// Hint
	if p.showHelp && p.options.HelpMessage != "" {
		printer.Fprintln()
		printer.Fprintf(
			color.HiMagentaString("%s %s\n",
				BoldString("Hint:"),
				p.options.HelpMessage,
			),
		)
	}

	// Only need to reset the cursor position when we are showing a message
	if p.cursorPosition != nil {
		printer.SetCursorPosition(*p.cursorPosition)
	}

	return nil
}
