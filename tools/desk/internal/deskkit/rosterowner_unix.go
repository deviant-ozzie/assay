//go:build unix

package deskkit

import (
	"fmt"
	"os"
	"syscall"
)

// checkFileOwner enforces the Unix ownership + mode form of the sshd rule.
func checkFileOwner(path string, fi os.FileInfo) error {
	if mode := fi.Mode().Perm(); mode&0o022 != 0 {
		kind, fix := "file", "0600"
		if fi.IsDir() {
			kind, fix = "directory", "0700"
		}
		return fmt.Errorf("roster config %s %s is group- or world-writable (mode %04o): "+
			"anything that can write it can name the accounts this tool trusts. "+
			"Fix with `chmod %s %s`", kind, path, mode, fix, path)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot determine the owner of %s — refusing to read a roster "+
			"whose ownership cannot be established", path)
	}
	if uid := os.Getuid(); int(st.Uid) != uid {
		return fmt.Errorf("roster config %s is owned by uid %d, not by the invoking user (uid %d) — "+
			"refusing to take the trusted-identity list from a file this user does not own",
			path, st.Uid, uid)
	}
	return nil
}
