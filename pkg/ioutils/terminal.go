package ioutils

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/ghodss/yaml"
	errs "github.com/pkg/errors"
	"golang.org/x/term"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

// AssumeYes automatically answers yes for all questions.
var AssumeYes bool

// Terminal a wrapper around a Cobra command, with extra methods
// to display messages.
type Terminal interface {
	InOrStdin() io.Reader
	OutOrStdout() io.Writer
	AskForConfirmation(msg ConfirmationMessage) bool
	Println(msg string)
	Printlnf(msg string, args ...interface{})
	PrintContextSeparatorf(context string, args ...interface{})
	PrintContextSeparatorWithBodyf(body, context string, args ...interface{})
	PrintObject(object runtime.Object, title string) error
}

// NewTerminal returns a new terminal with the given funcs to
// access the `in` reader and `out` writer
func NewTerminal(in func() io.Reader, out func() io.Writer) Terminal {
	return &DefaultTerminal{
		in:  in,
		out: out,
	}
}

// DefaultTerminal a wrapper around a Cobra command, with extra methods
// to display messages.
type DefaultTerminal struct {
	in  func() io.Reader
	out func() io.Writer
}

// InOrStdin returns an `io.Reader` to read the user's input
func (t *DefaultTerminal) InOrStdin() io.Reader {
	return t.in()
}

// OutOrStdout returns an `io.Writer` to write messages in the console
func (t *DefaultTerminal) OutOrStdout() io.Writer {
	return t.out()
}

// Println prints the given message and appends a line feed
func (t *DefaultTerminal) Println(msg string) {
	fmt.Fprintln(t.OutOrStdout(), msg)
}

// Printf prints the given message with arguments
func (t *DefaultTerminal) Printf(format string, args ...interface{}) {
	fmt.Fprintf(t.OutOrStdout(), format, args...)
}

// Printlnf prints the given message with arguments and appends a line feed
func (t *DefaultTerminal) Printlnf(format string, args ...interface{}) {
	fmt.Fprintf(t.OutOrStdout(), format+"\n", args...)
}

// PrintContextSeparatorf prints the context separator (only)
func (t *DefaultTerminal) PrintContextSeparatorf(context string, args ...interface{}) {
	t.PrintContextSeparatorWithBodyf("", context, args...)
}

// PrintContextSeparatorWithBodyf prints the context separator and a message
func (t *DefaultTerminal) PrintContextSeparatorWithBodyf(body, context string, args ...interface{}) {
	width, _, err := term.GetSize(0)
	if err != nil {
		width = 60
	}
	line := strings.Repeat("-", width)
	fmt.Fprintln(t.OutOrStdout(), "\n"+line)
	fmt.Fprintln(t.OutOrStdout(), " "+fmt.Sprintf(context, args...))
	if body != "" {
		fmt.Fprintln(t.OutOrStdout(), line)
		fmt.Fprintln(t.OutOrStdout(), body)
	}
	fmt.Fprintln(t.OutOrStdout(), line)
}

// PrintObject prints the given object
func (t *DefaultTerminal) PrintObject(object runtime.Object, title string) error {
	toPrint := object.DeepCopyObject()
	toPrintMeta, err := meta.Accessor(toPrint)
	if err != nil {
		return errs.Wrapf(err, "cannot get metadata from %+v", object)
	}
	toPrintMeta.SetManagedFields(nil)
	result, err := yaml.Marshal(toPrint)
	if err != nil {
		return errs.Wrapf(err, "unable to unmarshal %+v", object)
	}
	t.PrintContextSeparatorWithBodyf(string(result), "%s", title)
	return nil
}

func WithDangerZoneMessagef(consequence, action string, args ...interface{}) ConfirmationMessage {
	return ConfirmationMessage(fmt.Sprintf(`
###################################
####                           ####
####   !!!  DANGER ZONE  !!!   ####
####                           ####
###################################

THIS COMMAND WILL CAUSE %s
%s`, strings.ToUpper(consequence), WithMessagef(action, args...)))
}

func WithMessagef(action string, args ...interface{}) ConfirmationMessage {
	return ConfirmationMessage(fmt.Sprintf(`
Are you sure that you want to %s`, fmt.Sprintf(action, args...)))
}

type ConfirmationMessage string

func (t *DefaultTerminal) AskForConfirmation(msg ConfirmationMessage) bool {
	reader := bufio.NewReader(t.InOrStdin())
	t.Printlnf(string(msg))
	t.Printlnf("===============================")
	t.Printf("[y/n] -> ")
	text := ""
	var err error
	if AssumeYes {
		text = "y"
	} else {
		text, err = reader.ReadString('\n')
		if err != nil {
			log.Fatal("unable to read from input: ", err)
		}
	}
	text = strings.ReplaceAll(text, "\n", "")
	t.Printlnf("response: '%s'", text)
	switch text {
	case "y", "Y":
		return true
	case "n", "N":
		return false
	default:
		return t.AskForConfirmation("answer y or n")
	}
}
