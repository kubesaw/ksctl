package client

import "os/exec"

type CommandCreator func(name string, arg ...string) *exec.Cmd
