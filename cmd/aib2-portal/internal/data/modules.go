package data

import (
	"encoding/json"
	"io/fs"
)

type Module struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	Status       string   `json:"status"`
	Package      string   `json:"package"`
	Dependencies []string `json:"dependencies"`
	Deliverables []string `json:"deliverables"`
	Description  string   `json:"description"`
}

type ModuleStats struct {
	Total      int `json:"total"`
	Completed  int `json:"completed"`
	InProgress int `json:"in_progress"`
	Planned    int `json:"planned"`
}

func LoadModules(fsys fs.FS) ([]Module, error) {
	data, err := fs.ReadFile(fsys, "data/modules.json")
	if err != nil {
		return nil, err
	}
	var modules []Module
	if err := json.Unmarshal(data, &modules); err != nil {
		return nil, err
	}
	return modules, nil
}

func Stats(modules []Module) ModuleStats {
	s := ModuleStats{Total: len(modules)}
	for _, m := range modules {
		switch m.Status {
		case "completed":
			s.Completed++
		case "in_progress":
			s.InProgress++
		default:
			s.Planned++
		}
	}
	return s
}

func ByCategory(modules []Module) map[string][]Module {
	result := make(map[string][]Module)
	for _, m := range modules {
		result[m.Category] = append(result[m.Category], m)
	}
	return result
}

// CategoryOrder returns the display order for module categories.
func CategoryOrder() []string {
	return []string{
		"Core Infrastructure",
		"Consensus",
		"Storage",
		"Smart Contracts",
		"AI Integration",
		"Applications",
		"Evolution",
	}
}
