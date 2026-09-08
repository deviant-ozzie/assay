//go:build windows

package deskkit

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

const supportsPOSIXRosterModes = false

func setFixtureRosterHome(home string) (func(), error) {
	prevHome, hadHome := os.LookupEnv("HOME")
	prevProfile, hadProfile := os.LookupEnv("USERPROFILE")
	if err := os.Setenv("HOME", home); err != nil {
		return nil, err
	}
	if err := os.Setenv("USERPROFILE", home); err != nil {
		return nil, err
	}
	return func() {
		if hadHome {
			_ = os.Setenv("HOME", prevHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
		if hadProfile {
			_ = os.Setenv("USERPROFILE", prevProfile)
		} else {
			_ = os.Unsetenv("USERPROFILE")
		}
	}, nil
}

func secureTestRosterPaths(paths ...string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.SET_ACCESS,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, nil)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
			nil, nil, acl, nil); err != nil {
			return err
		}
	}
	return nil
}

func TestWindowsRosterACLRefusesOtherWriter(t *testing.T) {
	path := t.TempDir()
	if err := secureTestRosterPaths(path); err != nil {
		t.Fatal(err)
	}
	if err := checkFileOwner(path, mustStat(t, path)); err != nil {
		t.Fatalf("owner-only DACL refused: %v", err)
	}

	world, err := windows.StringToSid("S-1-1-0")
	if err != nil {
		t.Fatal(err)
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_WRITE,
		AccessMode:        windows.GRANT_ACCESS,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(world),
		},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if err := checkFileOwner(path, mustStat(t, path)); err == nil ||
		!strings.Contains(err.Error(), "write-capable Windows access") {
		t.Fatalf("world-writable DACL error = %v, want write-capable refusal", err)
	}
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi
}
