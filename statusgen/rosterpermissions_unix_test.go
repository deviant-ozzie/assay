//go:build unix

package main

import "testing"

const supportsPOSIXRosterModes = true

func secureTestRosterPaths(_ ...string) error { return nil }

func setTestRosterHome(t *testing.T, home string) { t.Setenv("HOME", home) }
