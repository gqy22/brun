package cmd

import (
	"errors"
	"os"
	"os/exec"
)

type PayloadCommand struct {
	Command   *exec.Cmd
	gateRead  *os.File
	gateWrite *os.File
}

func NewPayloadCommand(args []string, gated bool) (*PayloadCommand, error) {
	if len(args) == 0 {
		return nil, errors.New("payload command is empty")
	}
	if !gated {
		return &PayloadCommand{Command: exec.Command(args[0], args[1:]...)}, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	gateRead, gateWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	helperArgs := append([]string{"__exec-payload"}, args...)
	command := exec.Command(executable, helperArgs...)
	command.ExtraFiles = []*os.File{gateRead}
	return &PayloadCommand{Command: command, gateRead: gateRead, gateWrite: gateWrite}, nil
}

func (p *PayloadCommand) Started() error {
	if p == nil || p.gateRead == nil {
		return nil
	}
	err := p.gateRead.Close()
	p.gateRead = nil
	return err
}

func (p *PayloadCommand) Release() error {
	if p == nil || p.gateWrite == nil {
		return nil
	}
	_, writeErr := p.gateWrite.Write([]byte{1})
	closeErr := p.gateWrite.Close()
	p.gateWrite = nil
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func (p *PayloadCommand) Abort() {
	if p == nil {
		return
	}
	if p.gateRead != nil {
		_ = p.gateRead.Close()
		p.gateRead = nil
	}
	if p.gateWrite != nil {
		_ = p.gateWrite.Close()
		p.gateWrite = nil
	}
}
