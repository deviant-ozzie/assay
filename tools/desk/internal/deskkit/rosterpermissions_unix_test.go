//go:build unix

package deskkit

import "os"

const supportsPOSIXRosterModes = true

func secureTestRosterPaths(_ ...string) error { return nil }

func setFixtureRosterHome(home string) (func(), error) {
	prev, had := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		return nil, err
	}
	return func() {
		if had {
			_ = os.Setenv("HOME", prev)
		} else {
			_ = os.Unsetenv("HOME")
		}
	}, nil
}
