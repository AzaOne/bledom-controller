package lua

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

const patternDir = "patterns"

// sanitizeFilename checks for directory traversal and valid extension.
func sanitizeFilename(name string) (string, error) {
	if !strings.HasSuffix(name, ".lua") {
		return "", errors.New("filename must end with .lua")
	}
	cleanName := filepath.Base(name)
	if cleanName == "" || cleanName == ".lua" || strings.Contains(cleanName, "..") {
		return "", errors.New("invalid filename")
	}
	return cleanName, nil
}

// GetPatternPath returns the safe, absolute path to a pattern file.
func GetPatternPath(name string) (string, error) {
	cleanName, err := sanitizeFilename(name)
	if err != nil {
		return "", err
	}
	// Ensure the base directory exists
	if _, err := os.Stat(patternDir); os.IsNotExist(err) {
		os.Mkdir(patternDir, 0755)
	}
	return filepath.Join(patternDir, cleanName), nil
}

// GetPatternCode reads the content of a pattern file.
func GetPatternCode(name string) (string, error) {
	path, err := GetPatternPath(name)
	if err != nil {
		return "", err
	}
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// SavePatternCode writes content to a pattern file.
func SavePatternCode(name, code string) error {
	path, err := GetPatternPath(name)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, []byte(code), 0644)
}

// DeletePattern removes a pattern file.
func DeletePattern(name string) error {
	path, err := GetPatternPath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// GetPatternList returns a slice of all available pattern filenames.
func GetPatternList() ([]string, error) {
	var patterns []string
	files, err := ioutil.ReadDir(patternDir)
	if err != nil {
		// If the directory doesn't exist, that's not an error, just no patterns.
		if os.IsNotExist(err) {
			return patterns, nil
		}
		return nil, err
	}
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".lua" {
			patterns = append(patterns, file.Name())
		}
	}
	return patterns, nil
}
