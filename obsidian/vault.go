package obsidian

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Vault struct {
	rootDir string
	idx     *vaultIdx
	cfg     Cfg
}

type Cfg struct {
	ComputeEmbeddings bool
}

type vaultIdx struct {
	notes     map[string]note
	dailyDir  string
	weeklyDir string

	// Only used if cfg.computeEmbeddings is set in the vault.
	embeds *embedIdx
}

type note struct {
	relPath     string
	contentHash string // SHA-256 hex digest of note content
}

// LoadVault loads a vault from a given root directory.
// The root directory is the directory that contains the vault.
// It will be expanded to the home directory if it has a tilde prefix.
// It will return an error if the root directory is not a valid directory.
// It will also return an error if the daily folder is not found.
func LoadVault(rootDir string, cfg Cfg) (*Vault, error) {
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
		cfg: cfg,
	}

	if err := v.RefreshIndex(); err != nil {
		return nil, fmt.Errorf("failed to create index: %w", err)
	}

	if v.idx.dailyDir == "" {
		return nil, fmt.Errorf("daily folder not found in vault")
	}

	// NOTE: We're okay with the weeklyDir not being found.

	return v, nil
}

// RefreshIndex refreshes the index of the vault.
// It walks through every dir/subdir in the vault, to save all notes into the index.
// It also spots the daily folder, which is used to read the most recent dailies.
// It skips all subdirectories that start with a ".".
//
// Returns an error if the daily folder is not found.
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
			// Let's do the same to try and get the weekly folder
			if v.idx.weeklyDir == "" && strings.Contains(strings.ToLower(d.Name()), "weekly") {
				v.idx.weeklyDir = path
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

			// Compute content hash for change detection (used by embeddings cache).
			content, err := os.ReadFile(path)
			if err != nil {
				log.Printf("warning: failed to read note %s for hashing: %v", noteName, err)
				return nil
			}
			hash := sha256.Sum256(content)
			contentHash := hex.EncodeToString(hash[:])

			v.idx.notes[noteName] = note{
				relPath:     relPath,
				contentHash: contentHash,
			}
		}

		return nil
	}

	if err := filepath.WalkDir(v.rootDir, handler); err != nil {
		return fmt.Errorf("failed to walk the vault: %w", err)
	}

	return nil
}

// RefreshEmbeddingsAsync starts embeddings refresh in background.
// Returns a channel that receives nil on success or an error.
// The channel is closed after sending.
func (v *Vault) RefreshEmbeddingsAsync() <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		if err := v.RefreshEmbeddings(); err != nil {
			done <- err
			return
		}
		done <- nil
	}()
	return done
}

// EmbeddingsReady returns true if embeddings are available for semantic search.
func (v *Vault) EmbeddingsReady() bool {
	return v.idx.embeds != nil
}

// ReadNote reads the contents of a note from the vault.
// The name of the note is "pure", without directories and without exensions.
// E.g.: to read a note in `<rooDir>/dailies/2025-10-11.md`, the name is `2025-10-11` only.
// Returns the contents of the note wrapped in a `<note>` tag, with the note name and content.
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

// ListDir lists the items for a given relative directory in the vault.
// E.g.: "." will list the root directory of the vault.
//
// The items are sorted by filename. Items that start with a `.` are omitted.
// Directories will have a `/` suffix whereas regular files will not.
// Returns them as a list of strings.
func (v *Vault) ListDir(relPath string) ([]string, error) {
	dir, err := os.ReadDir(filepath.Join(v.rootDir, relPath))
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", relPath, err)
	}

	r := make([]string, 0, len(dir))
	for _, d := range dir {
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		if d.IsDir() {
			r = append(r, fmt.Sprintf("%s/", d.Name()))
		} else {
			r = append(r, d.Name())
		}
	}

	return r, nil
}

// Match represents a ripgrep search result for a single note.
type Match struct {
	NoteName     string
	MatchedLines []string
}

// RipGrep searches markdown notes under subFolder for pattern using ripgrep.
// Returns a slice of matches, where each match contains the note name (basename without .md)
// and the matched lines from that note.
// subFolder is joined with the vault root; hidden vault internals are excluded.
func (v *Vault) RipGrep(pattern, subFolder string, caseSensitive bool) ([]Match, error) {
	if pattern == "" {
		return nil, fmt.Errorf("pattern cannot be empty")
	}

	fullPath := filepath.Join(v.rootDir, subFolder)
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat search path %s: %w", fullPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("search path is not a directory: %s", fullPath)
	}

	args := []string{
		"--json",
		"-g", "*.md",
		"-g", "!.trash/",
		"-g", "!.obsidian/",
	}
	if caseSensitive {
		args = append(args, "--case-sensitive")
	} else {
		args = append(args, "--ignore-case")
	}
	args = append(args, pattern, fullPath)

	cmd := exec.Command("rg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to pipe ripgrep stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ripgrep: %w", err)
	}

	// Build matches directly in a slice, using noteIndex to track where each note is.
	//
	// Why we need noteIndex:
	// Ripgrep is multi-threaded by default, so output from different files can be
	// interleaved. For example:
	//   match from note1.md line 5
	//   match from note2.md line 10
	//   match from note1.md line 15  <- same note as first match!
	//
	// Without noteIndex, we'd create duplicate Match entries for the same note.
	// The map provides O(1) lookup to find which index in the matches slice
	// corresponds to each note name, allowing us to append to the correct Match.
	//
	// Memory overhead: ~8 bytes per unique matched note (just the integer index).
	// This is negligible compared to the actual match data (strings).
	matches := make([]Match, 0)
	noteIndex := make(map[string]int)

	type rgEvent struct {
		Type string `json:"type"`
		Data struct {
			Path struct {
				Text string `json:"text"`
			} `json:"path"`
			Lines struct {
				Text string `json:"text"`
			} `json:"lines"`
		} `json:"data"`
	}

	scanner := bufio.NewScanner(stdout)
	var scanErr error
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var ev rgEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			scanErr = fmt.Errorf("failed to decode ripgrep output: %w", err)
			break
		}

		if ev.Type != "match" {
			continue
		}

		noteName := strings.TrimSuffix(filepath.Base(ev.Data.Path.Text), ".md")
		matched := strings.TrimSpace(ev.Data.Lines.Text)

		if idx, exists := noteIndex[noteName]; exists {
			// Note already exists, append to its MatchedLines
			matches[idx].MatchedLines = append(matches[idx].MatchedLines, matched)
		} else {
			// New note, add to slice and record its index
			noteIndex[noteName] = len(matches)
			matches = append(matches, Match{
				NoteName:     noteName,
				MatchedLines: []string{matched},
			})
		}
	}

	if scanErr == nil {
		if err := scanner.Err(); err != nil {
			scanErr = fmt.Errorf("failed to read ripgrep output: %w", err)
		}
	}

	// Always wait for the command to finish to avoid zombie processes
	waitErr := cmd.Wait()

	// Check for scanning errors first
	if scanErr != nil {
		return nil, scanErr
	}

	// Handle command exit status
	if waitErr != nil {
		// Exit code 1 means no matches found, which is not an error
		if exitErr, ok := waitErr.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []Match{}, nil
		}
		return nil, fmt.Errorf("ripgrep failed: %w; stderr: %s", waitErr, stderr.String())
	}

	return matches, nil
}

// ReadRecentDailies reads the `n` most recent dailies.
// The dailies are sorted by descending filenames, with the most recent dates first.
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

// ReadRecentWeeklies reads the `n` most recent weeklies.
// The weeklies are sorted by descinding filename, with the most recent weeklies first.
// The contents of the returned slice are defined by ReadNote.
// If no weekly directory is available, returns a nil slice.
func (v *Vault) ReadRecentWeeklies(n int) ([]string, error) {
	if v.idx.weeklyDir == "" {
		return nil, nil
	}

	files, err := os.ReadDir(v.idx.weeklyDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read weekly directory: %w", err)
	}

	r := make([]string, 0, n)

	// We know that os.ReadDir returns all the directory entries /sorted/ by filename. Since weekly
	// notes contain only the date in their name, we can search for the most recent notes simply by
	// traversing the result of os.ReadDir backwards.
	for i := len(files) - 1; i >= 0; i-- {
		name := files[i].Name()
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
