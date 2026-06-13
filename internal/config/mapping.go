package config

import "path/filepath"

// Mapping is the Clockify target a local project token resolves to: a project
// id and an optional task id.
type Mapping struct {
	Project string
	Task    string
}

// MergeMappings combines the committed (project) and personal (user) token maps
// with personal precedence: a token present in user wins over the same token in
// project. It is pure — no file access — so the precedence rule is testable
// directly. The inputs are not mutated.
func MergeMappings(project, user map[string]Mapping) map[string]Mapping {
	merged := make(map[string]Mapping, len(project)+len(user))
	for token, m := range project {
		merged[token] = m
	}
	for token, m := range user {
		merged[token] = m
	}
	return merged
}

// fromEntries converts the on-disk mapping table into the public Mapping map.
func fromEntries(entries map[string]mappingEntry) map[string]Mapping {
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string]Mapping, len(entries))
	for token, e := range entries {
		out[token] = Mapping(e)
	}
	return out
}

// ProjectMappings returns the team-shared, token-keyed mappings committed in the
// .clk.toml at projectPath. A missing file yields an empty map.
func ProjectMappings(projectPath string) (map[string]Mapping, error) {
	f, err := loadProjectFile(filepath.Join(projectPath, projectFileName))
	if err != nil {
		return nil, err
	}
	return fromEntries(f.Mappings), nil
}

// UserMappings returns the personal, token-keyed override mappings from
// ~/.clk/config.toml. A missing file yields an empty map.
func UserMappings() (map[string]Mapping, error) {
	f, err := loadUserFile()
	if err != nil {
		return nil, err
	}
	return fromEntries(f.Mappings), nil
}

// ResolveMapping returns the effective Clockify mapping for a project token,
// merging the committed .clk.toml with personal ~/.clk overrides (personal
// wins). ok is false when neither layer maps the token, which is the signal for
// the prompt-once fuzzy pick.
func ResolveMapping(projectPath, token string) (m Mapping, ok bool, err error) {
	project, err := ProjectMappings(projectPath)
	if err != nil {
		return Mapping{}, false, err
	}
	user, err := UserMappings()
	if err != nil {
		return Mapping{}, false, err
	}
	m, ok = MergeMappings(project, user)[token]
	return m, ok, nil
}

// SetProjectMapping persists a token mapping into the committed .clk.toml at
// projectPath, preserving the template, billable default, and any other token
// mappings. This is the team-shared write used by the prompt-once pick and the
// default `clk link`, so teammates inherit the mapping on pull.
func SetProjectMapping(projectPath, token string, m Mapping) error {
	path := filepath.Join(projectPath, projectFileName)
	f, err := loadProjectFile(path)
	if err != nil {
		return err
	}
	if f.Mappings == nil {
		f.Mappings = make(map[string]mappingEntry)
	}
	f.Mappings[token] = mappingEntry(m)
	return saveProjectFile(path, f)
}

// SetUserMapping persists a personal token override into ~/.clk/config.toml,
// preserving credentials and any other personal settings. A personal override
// wins over the committed mapping locally without touching shared config.
func SetUserMapping(token string, m Mapping) error {
	f, err := loadUserFile()
	if err != nil {
		return err
	}
	if f.Mappings == nil {
		f.Mappings = make(map[string]mappingEntry)
	}
	f.Mappings[token] = mappingEntry(m)
	return writeUserFile(f)
}
