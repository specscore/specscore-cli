//go:build !linux && !darwin

package lifecycle

import "os"

func artifactIdentityOpenFlags() int { return os.O_RDONLY }
