package obsidian

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Vault struct {
	rootDir string
	idx     *vaultIdx
}

type vaultIdx struct {
	notes    map[string]note
	dailyDir string
}

type note struct {
	relPath string
}

func LoadVault(rootDir string) (*Vault, error) {
	// Expand the rootDir in case it has a tilde prefix.
	rootDir, err := expandHomeDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to expand home directory: %w", err)
	}

	// Let's first make sure that rootDir is a valid path to an existing directory.
	info, err := os.Stat(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat root directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root directory is not a directory: %s", rootDir)
	}

	v := &Vault{
		rootDir: rootDir,
		idx: &vaultIdx{
			notes:    make(map[string]note),
			dailyDir: "",
		},
	}

	if err := v.RefreshIndex(); err != nil {
		return nil, fmt.Errorf("failed to create index: %w", err)
	}

	if v.idx.dailyDir == "" {
		return nil, fmt.Errorf("daily folder not found in vault")
	}

	return v, nil
}

func (v *Vault) RefreshIndex() error {
	// Let's walk through every dir/subdir in the vault, to save all notes into the index.

	handler := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// For whatever reason it could not access this path.
			// TODO(logging): log this
			return err
		}

		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				// Let's skip all hidden directories.
				return filepath.SkipDir
			}

			// Let's also try and spot the daily folder.
			if v.idx.dailyDir == "" && strings.Contains(strings.ToLower(d.Name()), "daily") {
				v.idx.dailyDir = path
			}

			return nil
		}

		if strings.HasSuffix(d.Name(), ".md") {
			// Let's add this note to the index.
			noteName := strings.TrimSuffix(filepath.Base(path), ".md")
			relPath, err := filepath.Rel(v.rootDir, path)
			if err != nil {
				// Since we're walking the rootDir and got here from that, this should /never/
				// happen.
				panic(fmt.Errorf("failed to get relative path for note %s: %w", noteName, err))
			}

			// If the note already exists, panic for now. We do need to be able to allow
			// disambiguating notes with the same name in separate directories like Obsidian does.
			// TODO(feature): Implement smarted disambiguation.
			if _, ok := v.idx.notes[noteName]; ok {
				panic(fmt.Errorf("there are multiple notes with the same name: %s", noteName))
			}

			v.idx.notes[noteName] = note{
				relPath: relPath,
			}
		}

		return nil
	}

	return filepath.WalkDir(v.rootDir, handler)
}

func (v *Vault) ReadNote(name string) (string, error) {
	note, ok := v.idx.notes[name]
	if !ok {
		return "", fmt.Errorf("note %s not found", name)
	}

	content, err := os.ReadFile(filepath.Join(v.rootDir, note.relPath))
	if err != nil {
		return "", fmt.Errorf("failed to read note %s: %w", name, err)
	}

	return fmt.Sprintf("<note>\n<note_name>%s</note_name>\n\n<content>%s</content></note>", name, content), nil
}

// ReadRecentDailies reads the `n` most recent dailies.
// The dailies are sorted by the last modified time, and the most recent is the first.
// The contents of the returned slice are defined by ReadNote.
func (v *Vault) ReadRecentDailies(n int) ([]string, error) {
	files, err := os.ReadDir(v.idx.dailyDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read daily directory: %w", err)
	}

	r := make([]string, 0, n)

	// We know that os.ReadDir returns all the directory entries /sorted/ by filename. Since daily
	// notes contain only the date in their name, we can search for the most recent notes simply by
	// traversing the result of os.ReadDir backwards.
	for i := len(files) - 1; i >= 0; i-- {
		name := files[i].Name()
		// TODO(feature): let's be smart and skip potential random files here like "template.md"
		if !strings.HasSuffix(name, ".md") {
			continue
		}

		noteName := strings.TrimSuffix(name, ".md")

		content, err := v.ReadNote(noteName)
		if err != nil {
			return nil, fmt.Errorf("failed to read note %s: %w", noteName, err)
		}
		r = append(r, content)

		if len(r) >= n {
			break
		}
	}

	return r, nil
}

func expandHomeDir(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
	}
	return path, nil
}
