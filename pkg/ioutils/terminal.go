package ioutils

import (
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/ghodss/yaml"
	errs "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

// AssumeYes automatically answers yes for all questions.
var AssumeYes bool

type Terminal struct {
	input  io.Reader
	output io.Writer
	*log.Logger
	defaultConfirm *bool
}

func NewTerminal(input io.Reader, output io.Writer, options ...TerminalOption) Terminal {
	logger := log.NewWithOptions(output, log.Options{
		TimeFormat: time.Kitchen,
		Level:      log.InfoLevel,
	})
	t := Terminal{
		input:  input,
		output: output,
		Logger: logger,
	}
	for _, apply := range options {
		apply(&t)
	}
	return t
}

type TerminalOption func(t *Terminal)

func WithDefaultConfirm(v bool) TerminalOption {
	return func(t *Terminal) {
		t.defaultConfirm = &v
	}
}

func WithVerbose(verbose bool) TerminalOption {
	return func(t *Terminal) {
		if verbose {
			t.Logger.SetLevel(log.DebugLevel)
		}
	}
}

func WithTee(w io.Writer) TerminalOption {
	return func(t *Terminal) {
		t.Logger.SetOutput(io.MultiWriter(t.output, w))

	}
}

func (t Terminal) Confirm(msg string, args ...any) (bool, error) {
	if t.defaultConfirm != nil {
		return *t.defaultConfirm, nil
	}
	var answer bool
	confirm := huh.NewConfirm().Title(fmt.Sprintf(msg, args...)).Value(&answer)
	if err := huh.NewForm(huh.NewGroup(confirm)).WithInput(t.input).Run(); err != nil {
		return false, err
	}
	return answer, nil
}

func (t Terminal) PrintObject(title string, object runtime.Object) error {
	t.Info(title)
	r, err := glamour.NewTermRenderer(
		// detect background color and pick either the default dark or light theme
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(120),
		glamour.WithStylesFromJSONBytes([]byte(`
		{ 
			"document": { 
				"margin": 0,
				"block_prefix": "",
				"block_suffix": "",
				"prefix": ""
			},
			"code_block": {
				"margin": 0, 
				"block_prefix": "",
				"block_suffix": "",
				"prefix": "",
				"suffix": ""
			} 
		}`)),
	)
	if err != nil {
		return err
	}
	obj := object.DeepCopyObject()
	m, err := meta.Accessor(obj)
	if err != nil {
		return errs.Wrapf(err, "cannot get metadata from %+v", object)
	}
	m.SetManagedFields(nil)
	result, err := yaml.Marshal(obj)
	if err != nil {
		return errs.Wrapf(err, "unable to unmarshal %+v", object)
	}
	o, err := r.Render(fmt.Sprintf("```yaml\n%s\n```", string(result)))
	if err != nil {
		return err
	}
	t.Print(o)
	return nil

}
