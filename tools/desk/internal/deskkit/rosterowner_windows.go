//go:build windows

package deskkit

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// checkFileOwner enforces the Windows owner + DACL form of the sshd rule.
// os.FileMode is deliberately not consulted: Go projects an NTFS directory as
// 0777 and a file as 0666 regardless of its ACL, so applying a POSIX mode mask
// would refuse every correctly secured Windows roster.
func checkFileOwner(path string, _ os.FileInfo) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		return fmt.Errorf("cannot read the Windows owner/DACL of %s — refusing to read a roster "+
			"whose permissions cannot be established: %w", path, err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return fmt.Errorf("cannot determine the Windows owner of %s — refusing to read a roster "+
			"whose ownership cannot be established: %w", path, err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return fmt.Errorf("cannot determine the invoking Windows user for %s — refusing to read "+
			"a roster whose ownership cannot be established: %w", path, err)
	}
	if !owner.Equals(user.User.Sid) {
		return fmt.Errorf("roster config %s is not owned by the invoking Windows user — "+
			"refusing to take the trusted-identity list from a path this user does not own", path)
	}

	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("cannot establish a restrictive Windows DACL for %s — refusing to "+
			"read a roster whose write access cannot be bounded: %w", path, err)
	}
	system, _ := windows.StringToSid("S-1-5-18")
	admins, _ := windows.StringToSid("S-1-5-32-544")
	writeMask := windows.ACCESS_MASK(windows.GENERIC_WRITE | windows.GENERIC_ALL |
		windows.WRITE_DAC | windows.WRITE_OWNER | windows.DELETE |
		windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA |
		windows.FILE_WRITE_EA | windows.FILE_WRITE_ATTRIBUTES |
		0x00000040) // FILE_DELETE_CHILD

	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil || ace == nil {
			return fmt.Errorf("cannot inspect Windows DACL entry %d for %s — refusing: %w", i, path, err)
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE ||
			ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return fmt.Errorf("Windows DACL for %s contains unsupported allow ACE type %d — "+
				"refusing rather than guessing its write scope", path, ace.Header.AceType)
		}
		if ace.Mask&writeMask == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid.Equals(owner) || sid.Equals(system) || sid.Equals(admins) {
			continue
		}
		return fmt.Errorf("roster config %s grants write-capable Windows access to SID %s — "+
			"anything that can write it can name the accounts this tool trusts", path, sid.String())
	}
	return nil
}
