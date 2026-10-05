package ide

import (
	"fmt"
	"regexp"
	"strings"
)

type SearchQuery struct {
	Text          string `json:"text"`
	Regex         bool   `json:"regex"`
	CaseSensitive bool   `json:"caseSensitive"`
	WholeWord     bool   `json:"wholeWord"`
	Include       string `json:"include"`
}

type SearchMatch struct {
	Version string `json:"version"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Text    string `json:"text"`
	Length  int    `json:"length"`
}

type SearchResult struct {
	Matches   []SearchMatch `json:"matches"`
	Truncated bool          `json:"truncated"`
}

func searchPattern(query SearchQuery) (*regexp.Regexp, error) {
	if query.Text == "" {
		return nil, fmt.Errorf("Enter a search term")
	}
	pattern := query.Text
	if !query.Regex {
		pattern = regexp.QuoteMeta(pattern)
	}
	if query.WholeWord {
		pattern = `\b(?:` + pattern + `)\b`
	}
	if !query.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

func (w *Workspace) Search(query SearchQuery) (*SearchResult, error) {
	pattern, err := searchPattern(query)
	if err != nil {
		return nil, err
	}
	files, err := w.FileIndex()
	if err != nil {
		return nil, err
	}
	result := &SearchResult{Matches: []SearchMatch{}}
	for _, file := range files {
		if query.Include != "" && !strings.Contains(strings.ToLower(file.Path), strings.ToLower(query.Include)) {
			continue
		}
		document, err := w.Read(file.Path)
		if err != nil {
			continue
		}
		for index, line := range strings.Split(document.Content, "\n") {
			for _, match := range pattern.FindAllStringIndex(line, -1) {
				result.Matches = append(result.Matches, SearchMatch{Version: document.Version, Path: file.Path, Line: index + 1, Column: len([]rune(line[:match[0]])) + 1, Text: line, Length: len([]rune(line[match[0]:match[1]]))})
				if len(result.Matches) >= 2000 {
					result.Truncated = true
					return result, nil
				}
			}
		}
	}
	return result, nil
}

func (w *Workspace) ReplaceSearchFile(query SearchQuery, path, replacement, version string) (*Document, error) {
	pattern, err := searchPattern(query)
	if err != nil {
		return nil, err
	}
	document, err := w.Read(path)
	if err != nil {
		return nil, err
	}
	if document.Version != version {
		return nil, fmt.Errorf("File changed since search. Search again before replacing")
	}
	content := pattern.ReplaceAllStringFunc(document.Content, func(string) string { return replacement })
	if query.Regex {
		content = pattern.ReplaceAllString(document.Content, replacement)
	}
	return w.SaveChecked(path, content, version)
}
