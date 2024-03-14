package test

import (
	"bytes"
	"io"

	"github.com/kubesaw/ksctl/pkg/ioutils"
)

// FakeTerminal a fake terminal, which can:
// - be configured to always return the same response when the command prompts the user for confirmation
// - capture the command output
type FakeTerminal struct {
	out *bytes.Buffer
	ioutils.Terminal
}

// NewFakeTerminal returns a new FakeTerminal which will
// only print messages in the console
func NewFakeTerminal() *FakeTerminal {
	out := bytes.NewBuffer(nil)
	term := &FakeTerminal{
		out: out,
		Terminal: ioutils.NewTerminal(nil, func() io.Writer {
			return out
		}),
	}
	return term
}

// Tee uses the given `out` as a secondary output.
// Usage: `Tee(os.Stdout)` to see in the console what's record in this terminal during the tests
// Note: it should be configured at the beginning of a test
func (t *FakeTerminal) Tee(out io.Writer) {
	t.Terminal = ioutils.NewTerminal(t.Terminal.InOrStdin, func() io.Writer {
		return io.MultiWriter(t.out, out)
	})
}

// NewFakeTerminalWithResponse returns a new FakeTerminal which will
// print messages in the console and respond to the questions/confirmations
// with the given response
func NewFakeTerminalWithResponse(response string) *FakeTerminal {
	out := bytes.NewBuffer(nil)
	term := &FakeTerminal{
		out: out,
		Terminal: ioutils.NewTerminal(
			func() io.Reader {
				in := bytes.NewBuffer(nil)
				in.WriteString(response)
				in.WriteByte('\n')
				return in
			},
			func() io.Writer {
				return out
			},
		),
	}
	return term
}

// Output return the content of the output buffer
func (t *FakeTerminal) Output() string {
	return t.out.String()
}
