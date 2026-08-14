//go:build darwin

package cli

// com.apple.provenance is synthesized by the local execution environment for
// newly-created files and directories. It is neither user-controlled SpecScore
// content nor stable clone metadata, so treating it as tree state would make a
// clean COW lint appear to change every entry. All user xattrs and ACL-backed
// xattrs are captured and restored by descriptor.
func isEphemeralSpecTreeXattr(name string) bool { return name == "com.apple.provenance" }
