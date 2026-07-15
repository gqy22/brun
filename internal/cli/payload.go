package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

const payloadGateFD = 3

func payloadExecCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "__exec-payload <command...>",
		Hidden:             true,
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			gate := os.NewFile(payloadGateFD, "brun-payload-gate")
			if gate == nil {
				return fmt.Errorf("payload gate fd %d unavailable", payloadGateFD)
			}
			defer gate.Close()
			if err := waitForPayloadRelease(gate); err != nil {
				return err
			}
			path, err := exec.LookPath(args[0])
			if err != nil {
				return err
			}
			_ = gate.Close()
			return syscall.Exec(path, args, os.Environ())
		},
	}
}

func waitForPayloadRelease(reader io.Reader) error {
	var signal [1]byte
	if _, err := io.ReadFull(reader, signal[:]); err != nil {
		return fmt.Errorf("wait for payload release: %w", err)
	}
	if signal[0] != 1 {
		return fmt.Errorf("invalid payload release signal")
	}
	return nil
}
