package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

func confirm(reader io.Reader, writer io.Writer, prompt string) (bool, error) {
	if _, err := fmt.Fprintf(writer, "%s (y/N): ", prompt); err != nil {
		return false, err
	}

	response, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || response == "") {
		return false, err
	}

	switch strings.ToLower(strings.TrimSpace(response)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
