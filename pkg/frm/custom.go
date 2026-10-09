package frm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vb3dec/pkg/vbx"
)

// LoadCustomModels reads the control models of a project's custom control
// libraries, searching dirs for each file name case-insensitively. The models
// are keyed by lower-case class name, which is how form streams reference
// custom control types. Libraries that cannot be found or read are reported as
// warnings; their controls are still extracted, without properties.
func LoadCustomModels(libraries []string, dirs []string) (map[string]*vbx.Model, []string) {
	models := make(map[string]*vbx.Model)
	var warnings []string
	for _, lib := range libraries {
		base := lib[strings.LastIndexAny(lib, `\/`)+1:]
		path, ok := findFileFold(dirs, base)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("custom control library %s not found; its control properties are not decoded", base))
			continue
		}
		libModels, err := vbx.ParseFile(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("reading custom control library %s: %v", path, err))
			continue
		}
		if len(libModels) == 0 {
			warnings = append(warnings, fmt.Sprintf("no control models found in %s", path))
			continue
		}
		for _, m := range libModels {
			models[strings.ToLower(m.ClassName)] = m
		}
	}
	return models, warnings
}

// findFileFold returns the first file in dirs whose name matches name case-insensitively.
func findFileFold(dirs []string, name string) (string, bool) {
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(e.Name(), name) {
				return filepath.Join(dir, e.Name()), true
			}
		}
	}
	return "", false
}
