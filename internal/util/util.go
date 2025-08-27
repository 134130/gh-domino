package util

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// AskForConfirmation asks the user for confirmation.
func AskForConfirmation(s string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("%s (y/N): ", s)

	response, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	response = strings.ToLower(strings.TrimSpace(response))

	if response == "y" || response == "yes" {
		return true, nil
	}

	return false, nil
}
