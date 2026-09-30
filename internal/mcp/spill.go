package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/home"
)

// Tool results land in the agent's context, and hosts cap them: Claude Code
// warns past about 10k tokens and cuts at 25k, and a cut-off JSON tree is
// worse than none. So a large result is kept whole on disk and the agent is
// given its path and a smaller way to get what it needs. Files are written
// only inside DeviceDeck's own folder, under names built from the tool and a
// timestamp — never a caller-supplied path.

// maxToolText is the largest result sent inline, in characters (~10k tokens).
const maxToolText = 40_000

// outDir is where oversized results are written: ~/.devicedeck/mcp-out, or a
// temporary folder when the home cannot be resolved.
func outDir() string {
	dir, err := home.Dir()
	if err != nil {
		return filepath.Join(os.TempDir(), "devicedeck-mcp-out")
	}
	return filepath.Join(dir, "mcp-out")
}

// spillJSON returns a JSON result as is when small, else writes it to a file
// and returns where, with what to use instead.
func (c *Client) spillJSON(tool, text string) (string, error) {
	if len(text) <= maxToolText {
		return text, nil
	}
	path, err := c.writeOut(tool, "json", text)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s result is %d KB, too large to return; the full JSON is in %s. "+
		"Use snapshot (compact, with refs) or find_element to look for what you need.", tool, len(text)/1024, path), nil
}

// capLines returns a line-oriented result cut at a line boundary under the
// limit, saying how many lines were left out and where the rest is.
func (c *Client) capLines(tool, text string) (string, error) {
	if len(text) <= maxToolText {
		return text, nil
	}
	path, err := c.writeOut(tool, "txt", text)
	if err != nil {
		return "", err
	}
	cut := strings.LastIndex(text[:maxToolText], "\n")
	if cut < 0 {
		cut = maxToolText
	}
	rest := strings.Count(text[cut:], "\n")
	return fmt.Sprintf("%s\n… %d more lines not shown (full list in %s). Narrow it: mode \"interactive\", "+
		"find_element, or scroll.", text[:cut], rest, path), nil
}

// writeOut stores a result under the output folder and returns its path.
func (c *Client) writeOut(tool, ext, text string) (string, error) {
	dir := c.OutDir
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("save %s result: %w", tool, err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.%s", tool, time.Now().Format("20060102-150405.000"), ext))
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return "", fmt.Errorf("save %s result: %w", tool, err)
	}
	return path, nil
}
