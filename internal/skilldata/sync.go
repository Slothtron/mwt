package skilldata

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const SkillDirName = "mwt"

// Sync writes the embedded skill tree to destDir/mwt.
// If dest already exists and force is false, returns an error.
func Sync(skillsParent string, force bool) (dest string, err error) {
	dest = filepath.Join(skillsParent, SkillDirName)
	if info, statErr := os.Stat(dest); statErr == nil {
		if !force {
			return "", fmt.Errorf("%s already exists (use --force to overwrite)", dest)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%s exists and is not a directory", dest)
		}
		if err := os.RemoveAll(dest); err != nil {
			return "", fmt.Errorf("remove %s: %w", dest, err)
		}
	} else if !os.IsNotExist(statErr) {
		return "", fmt.Errorf("stat %s: %w", dest, statErr)
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dest, err)
	}

	err = fs.WalkDir(FS, SkillDirName, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(SkillDirName, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return "", fmt.Errorf("sync skill: %w", err)
	}
	return dest, nil
}
