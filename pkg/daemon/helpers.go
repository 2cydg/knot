package daemon

import "knot/pkg/config"

func isValidAlias(alias string) bool {
	return config.IsValidAlias(alias)
}
