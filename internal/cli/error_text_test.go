package cli

import (
	"strings"
)

func contains(err error, want string) bool {
	return err != nil && strings.Contains(err.Error(), want)
}
