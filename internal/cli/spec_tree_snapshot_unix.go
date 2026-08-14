//go:build darwin || linux

package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// Test seams for descriptor-relative traversal. The entry hook runs after a
// name was enumerated but before its descriptor is opened, which is precisely
// the historical pathname swap window. The reader receives the already-open
// descriptor and can never redirect traversal through a replacement path.
var (
	transactionBeforeSnapshotOpenat = func(directory, rel, name string) {}
	snapshotOpenRoot                = unix.Open
	snapshotOpenAt                  = unix.Openat
	snapshotFileStat                = func(file *os.File) (os.FileInfo, error) { return file.Stat() }
	snapshotReadDirNames            = func(file *os.File) ([]string, error) { return file.Readdirnames(-1) }
	snapshotSeek                    = func(file *os.File, offset int64, whence int) (int64, error) { return file.Seek(offset, whence) }
	snapshotClose                   = func(file *os.File) error { return file.Close() }
	snapshotFlistxattr              = unix.Flistxattr
	snapshotFgetxattr               = unix.Fgetxattr
	snapshotFsetxattr               = unix.Fsetxattr
	snapshotMetadataEntryTimes      = snapshotEntryTimes
)

func platformSupportsSecureLifecycleTransaction() bool { return true }

// snapshotSpecTreeNoFollow walks using directory descriptors and opens every
// child relative to its already-open parent with O_NOFOLLOW.  A pathname walk
// followed by ReadFile is not safe here: an untrusted writer can replace a
// component with a symlink between those two calls.  The resulting descriptor
// names a stable inode, so a replacement is either observed as a new directory
// entry on the next snapshot or is retained in the recovery tree at publish.
func snapshotSpecTreeNoFollow(specRoot string) (specTreeSnapshot, error) {
	fd, err := snapshotOpenRoot(specRoot, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return specTreeSnapshot{}, fmt.Errorf("opening spec tree without following links: %w", err)
	}
	root := os.NewFile(uintptr(fd), specRoot)
	defer func() { _ = snapshotClose(root) }()
	return snapshotSpecTreeNoFollowDirectory(root)
}

func snapshotStagedSpecTreeNoFollow(stage *stagedSpecTree) (specTreeSnapshot, error) {
	if stage == nil || stage.root == nil {
		return specTreeSnapshot{}, fmt.Errorf("staged spec tree descriptor is closed")
	}
	return snapshotSpecTreeNoFollowDirectory(stage.root)
}

func snapshotSpecTreeNoFollowDirectory(root *os.File) (specTreeSnapshot, error) {
	if _, err := snapshotSeek(root, 0, io.SeekStart); err != nil {
		return specTreeSnapshot{}, fmt.Errorf("rewinding spec tree descriptor: %w", err)
	}
	info, err := snapshotFileStat(root)
	if err != nil {
		return specTreeSnapshot{}, fmt.Errorf("statting spec tree descriptor: %w", err)
	}
	metadata, err := captureSnapshotEntryMetadata(root, info)
	if err != nil {
		return specTreeSnapshot{}, fmt.Errorf("validating spec tree root metadata: %w", err)
	}
	snapshot := specTreeSnapshot{
		files:       make(map[string]specTreeFile),
		directories: map[string]specTreeDirectory{".": {mode: info.Mode(), metadata: metadata}},
	}
	if err := snapshotDirectoryNoFollow(root, ".", &snapshot); err != nil {
		return specTreeSnapshot{}, fmt.Errorf("snapshotting spec tree: %w", err)
	}
	return snapshot, nil
}

func stagedSpecTreeMatchesPath(stage *stagedSpecTree) (bool, error) {
	fd, err := snapshotOpenRoot(stage.path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return false, err
	}
	candidate := os.NewFile(uintptr(fd), stage.path)
	defer func() { _ = snapshotClose(candidate) }()
	heldInfo, err := snapshotFileStat(stage.root)
	if err != nil {
		return false, err
	}
	candidateInfo, err := snapshotFileStat(candidate)
	if err != nil {
		return false, err
	}
	return os.SameFile(heldInfo, candidateInfo), nil
}

func stagedSpecTreePublishedAt(stage *stagedSpecTree, path string) (bool, error) {
	fd, err := snapshotOpenRoot(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return false, err
	}
	published := os.NewFile(uintptr(fd), path)
	defer func() { _ = snapshotClose(published) }()
	heldInfo, err := snapshotFileStat(stage.root)
	if err != nil {
		return false, err
	}
	publishedInfo, err := snapshotFileStat(published)
	if err != nil {
		return false, err
	}
	return os.SameFile(heldInfo, publishedInfo), nil
}

func snapshotDirectoryNoFollow(directory *os.File, rel string, snapshot *specTreeSnapshot) error {
	names, err := snapshotReadDirNames(directory)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", rel, err)
	}
	for _, name := range names {
		// Readdirnames never returns these, but reject them defensively before
		// passing the name to openat.
		if name == "." || name == ".." || filepath.Base(name) != name {
			return fmt.Errorf("invalid directory entry %q under %s", name, rel)
		}
		transactionBeforeSnapshotOpenat(directory.Name(), rel, name)
		fd, err := snapshotOpenAt(int(directory.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			if err == unix.ELOOP {
				return fmt.Errorf("refusing symbolic link %s in lifecycle transaction", filepath.Join(rel, name))
			}
			return fmt.Errorf("opening %s without following links: %w", filepath.Join(rel, name), err)
		}
		child := os.NewFile(uintptr(fd), name)
		childInfo, statErr := snapshotFileStat(child)
		if statErr != nil {
			_ = snapshotClose(child)
			return fmt.Errorf("statting %s: %w", filepath.Join(rel, name), statErr)
		}
		childRel := filepath.Join(rel, name)
		if rel == "." {
			childRel = name
		}
		metadata, err := captureSnapshotEntryMetadata(child, childInfo)
		if err != nil {
			_ = snapshotClose(child)
			return fmt.Errorf("validating %s metadata: %w", childRel, err)
		}
		if childInfo.IsDir() {
			snapshot.directories[childRel] = specTreeDirectory{mode: childInfo.Mode(), metadata: metadata}
			if err := snapshotDirectoryNoFollow(child, childRel, snapshot); err != nil {
				_ = snapshotClose(child)
				return err
			}
			if err := snapshotClose(child); err != nil {
				return fmt.Errorf("closing directory %s: %w", childRel, err)
			}
			continue
		}
		if !childInfo.Mode().IsRegular() {
			_ = snapshotClose(child)
			return fmt.Errorf("refusing non-regular file %s in lifecycle transaction", childRel)
		}
		content, readErr := transactionReadSnapshotFile(child)
		if readErr != nil {
			_ = snapshotClose(child)
			return fmt.Errorf("reading %s through stable descriptor: %w", childRel, readErr)
		}
		// Re-stat the same descriptor. A write that changes its observable
		// identity while we read is a concurrent mutation; fail rather than
		// applying a lint result derived from a torn input.
		afterRead, statErr := snapshotFileStat(child)
		closeErr := snapshotClose(child)
		if statErr != nil {
			return fmt.Errorf("re-statting %s: %w", childRel, statErr)
		}
		if closeErr != nil {
			return fmt.Errorf("closing %s: %w", childRel, closeErr)
		}
		if childInfo.Mode() != afterRead.Mode() || childInfo.Size() != afterRead.Size() || !childInfo.ModTime().Equal(afterRead.ModTime()) {
			return fmt.Errorf("concurrent modification while reading %s", childRel)
		}
		snapshot.files[childRel] = specTreeFile{content: content, mode: childInfo.Mode(), metadata: metadata}
	}
	return nil
}

// validateSnapshotEntryMetadata rejects filesystem states that the staged
// content/mode materialiser cannot prove it preserves.  In particular, a
// symlink, device, FIFO, socket, hard-linked file, privileged mode, foreign
// owner/group, ACL, capability, or extended attribute must never disappear as
// an incidental consequence of a lifecycle lint pass.  Refusing before
// publication is safer than publishing a superficially-valid but lossy tree.
func captureSnapshotEntryMetadata(file *os.File, info os.FileInfo) (specTreeEntryMetadata, error) {
	mode := info.Mode()
	if mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return specTreeEntryMetadata{}, fmt.Errorf("unsupported privileged mode %v", mode)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return specTreeEntryMetadata{}, fmt.Errorf("unsupported stat metadata type %T", info.Sys())
	}
	if stat.Uid != uint32(os.Getuid()) || stat.Gid != uint32(os.Getgid()) {
		return specTreeEntryMetadata{}, fmt.Errorf("owner %d:%d cannot be preserved by isolated lint", stat.Uid, stat.Gid)
	}
	if !info.IsDir() && stat.Nlink != 1 {
		return specTreeEntryMetadata{}, fmt.Errorf("hard-linked file with %d links cannot be preserved by isolated lint", stat.Nlink)
	}
	platformFlags, err := snapshotPlatformFlags(int(file.Fd()), info)
	if err != nil {
		return specTreeEntryMetadata{}, err
	}
	attributes, err := readSnapshotExtendedAttributes(int(file.Fd()))
	if err != nil {
		return specTreeEntryMetadata{}, fmt.Errorf("inspecting extended attributes: %w", err)
	}
	accessTime, modificationTime, err := snapshotMetadataEntryTimes(info)
	if err != nil {
		return specTreeEntryMetadata{}, err
	}
	return specTreeEntryMetadata{
		extendedAttributes: attributes,
		accessTime:         accessTime,
		modificationTime:   modificationTime,
		platformFlags:      platformFlags,
	}, nil
}

func readSnapshotExtendedAttributes(fd int) (map[string][]byte, error) {
	size, err := snapshotFlistxattr(fd, nil)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}
	namesBuffer := make([]byte, size)
	n, err := snapshotFlistxattr(fd, namesBuffer)
	if err != nil {
		return nil, err
	}
	if n != size {
		return nil, fmt.Errorf("extended attribute list changed while reading")
	}
	names := strings.Split(string(namesBuffer), "\x00")
	attributes := make(map[string][]byte, len(names))
	for _, name := range names {
		if name == "" || isEphemeralSpecTreeXattr(name) {
			continue
		}
		if isUnpreservableSpecTreeXattr(name) {
			return nil, fmt.Errorf("cannot preserve ACL, capability, or security xattr %s", name)
		}
		valueSize, err := snapshotFgetxattr(fd, name, nil)
		if err != nil {
			return nil, fmt.Errorf("reading %s size: %w", name, err)
		}
		value := make([]byte, valueSize)
		n, err := snapshotFgetxattr(fd, name, value)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		if n != valueSize {
			return nil, fmt.Errorf("extended attribute %s changed while reading", name)
		}
		attributes[name] = value
	}
	if len(attributes) == 0 {
		return nil, nil
	}
	return attributes, nil
}

func isUnpreservableSpecTreeXattr(name string) bool {
	switch name {
	case "system.posix_acl_access", "system.posix_acl_default", "security.capability", "com.apple.macl":
		return true
	}
	return false
}

func applyStagedEntryMetadata(fd int, metadata specTreeEntryMetadata) error {
	names := make([]string, 0, len(metadata.extendedAttributes))
	for name := range metadata.extendedAttributes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := snapshotFsetxattr(fd, name, metadata.extendedAttributes[name], 0); err != nil {
			return fmt.Errorf("setting %s: %w", name, err)
		}
	}
	if !metadata.accessTime.IsZero() || !metadata.modificationTime.IsZero() {
		if err := setStagedEntryTimes(fd, metadata.accessTime, metadata.modificationTime); err != nil {
			return fmt.Errorf("setting access/modification times: %w", err)
		}
	}
	return applyStagedPlatformFlags(fd, metadata.platformFlags)
}
