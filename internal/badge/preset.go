package badge

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// Detection is one project kind found by `badge detect`.
type Detection struct {
	Kind     string `json:"kind"`
	Preset   string `json:"preset"`
	Evidence string `json:"evidence"`
}

// Preset is a named detector block plus the badges it declares.
type PresetSpec struct {
	Name      string
	Detectors map[string]Detector
	Badges    []model.Badge
}

var presetNames = []string{"unity", "go", "node", "rust", "dotnet", "generic"}

// PresetNames lists the known presets.
func PresetNames() []string { return append([]string(nil), presetNames...) }

// Preset returns the detectors and declared badges of a preset.
func Preset(name string) (PresetSpec, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "unity":
		return PresetSpec{
			Name: "unity",
			Detectors: map[string]Detector{
				"unity-editor": {
					Kind: KindFile, Path: "ProjectSettings/ProjectVersion.txt",
					Format: FormatRegex, Pattern: `m_EditorVersion:\s*(\S+)`,
					Label: "Unity", Color: "000000", Logo: "unity",
				},
				"unity-backend": {
					Kind: KindFile, Path: "ProjectSettings/ProjectSettings.asset",
					Format:  FormatRegex,
					Pattern: `scriptingBackend:(?:\s*\n\s*[A-Za-z]+:\s*\d+)*\s*\n\s*Standalone:\s*(\d)`,
					Map:     map[string]string{"0": "Mono", "1": "IL2CPP"},
					Label:   "backend", Color: "lightgrey",
				},
				"csharp-lang": {
					Kind: KindFile, Format: FormatRegex,
					// Directory.Build.targets is hand-authored; a Unity *.csproj is generated with the
					// editor's language version, so it is only a fallback and never the answer.
					Paths:   []string{"Directory.Build.targets", "*.csproj"},
					Exclude: []string{"**/Assembly-CSharp*.csproj"},
					Pattern: `<LangVersion>\s*([^<]+)\s*</LangVersion>`,
					Label:   "C#", Color: "512BD4", Logo: "csharp",
				},
				"project-version": {
					Kind: KindFile, Path: "ProjectSettings/ProjectSettings.asset",
					Format: FormatRegex, Pattern: `bundleVersion:\s*(\S+)`,
					Label: "version", Color: "orange",
				},
			},
			Badges: []model.Badge{
				{ID: "unity-editor"}, {ID: "unity-backend"}, {ID: "csharp-lang"},
				{ID: "project-version"}, {ID: "license"}, {ID: "remote"},
			},
		}, true
	case "go":
		return PresetSpec{
			Name: "go",
			Badges: []model.Badge{
				{ID: "go"}, {ID: "module"}, {ID: "license"}, {ID: "remote"},
				{ID: "version"},
			},
		}, true
	case "node":
		return PresetSpec{
			Name: "node",
			Detectors: map[string]Detector{
				"node": {
					Kind: KindPackage, Ecosystem: "node", Field: "engines.node",
					Label: "node", Color: "339933", Logo: "nodedotjs",
				},
			},
			Badges: []model.Badge{{ID: "node"}, {ID: "license"}, {ID: "remote"}},
		}, true
	case "rust":
		return PresetSpec{
			Name: "rust",
			Detectors: map[string]Detector{
				"rust": {
					Kind: KindFile, Path: "rust-toolchain.toml", Format: FormatTOML,
					Query: "toolchain.channel", Label: "rust", Color: "000000", Logo: "rust",
				},
			},
			Badges: []model.Badge{{ID: "rust"}, {ID: "license"}, {ID: "remote"}},
		}, true
	case "dotnet":
		return PresetSpec{
			Name: "dotnet",
			Detectors: map[string]Detector{
				"target-framework": {
					Kind: KindPackage, Ecosystem: "dotnet", Field: "TargetFramework",
					Label: ".NET", Color: "512BD4", Logo: "dotnet",
				},
				"project-version": {
					Kind: KindFile,
					// Directory.Build.props is the shared, hand-authored place for the version; a
					// project file is the fallback.
					Paths:  []string{"Directory.Build.props", "*.csproj"},
					Format: FormatXML, Query: "Project/PropertyGroup/Version",
					Label: "version", Color: "orange",
				},
			},
			Badges: []model.Badge{{ID: "target-framework"}, {ID: "project-version"},
				{ID: "license"}, {ID: "remote"}},
		}, true
	case "generic":
		return PresetSpec{
			Name:   "generic",
			Badges: []model.Badge{{ID: "license"}, {ID: "remote"}, {ID: "version"}},
		}, true
	}
	return PresetSpec{}, false
}

// Detect reports the applicable project kinds with their evidence. It writes nothing and picks no
// preset implicitly. One project yields one ecosystem: a Unity project also carries generated
// *.csproj and *.sln files, so unity wins and .NET is not reported beside it.
func Detect(projectDir string) []Detection {
	detections := make([]Detection, 0, 5)
	unity := fileExists(projectDir, "ProjectSettings/ProjectVersion.txt")
	if unity {
		detections = append(detections, Detection{Kind: "unity", Preset: "unity",
			Evidence: "ProjectSettings/ProjectVersion.txt"})
	}
	if fileExists(projectDir, "go.mod") {
		detections = append(detections, Detection{Kind: "go", Preset: "go", Evidence: "go.mod"})
	}
	if fileExists(projectDir, "package.json") {
		detections = append(detections, Detection{Kind: "node", Preset: "node",
			Evidence: "package.json"})
	}
	if fileExists(projectDir, "Cargo.toml") {
		detections = append(detections, Detection{Kind: "rust", Preset: "rust",
			Evidence: "Cargo.toml"})
	}
	if !unity {
		if evidence, ok := dotnetEvidence(projectDir); ok {
			detections = append(detections, Detection{Kind: "dotnet", Preset: "dotnet",
				Evidence: evidence})
		}
	}
	return detections
}

// dotnetEvidence returns the project or solution file that identifies a .NET project. Unity's
// generated Assembly-CSharp files never count, even when Unity itself is not detected.
func dotnetEvidence(projectDir string) (string, bool) {
	for _, pattern := range []string{"*.csproj", "*.sln"} {
		matches, err := filepath.Glob(filepath.Join(projectDir, pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			name := filepath.Base(match)
			if strings.HasPrefix(name, "Assembly-CSharp") {
				continue
			}
			return name, true
		}
	}
	return "", false
}

// DetectorConflicts reports configured detector ids of a preset that differ from the preset.
func DetectorConflicts(preset PresetSpec, configured map[string]Detector) []string {
	conflicts := make([]string, 0)
	for id, detector := range preset.Detectors {
		existing, exists := configured[id]
		if exists && !sameDetector(existing, detector) {
			conflicts = append(conflicts, id)
		}
	}
	return conflicts
}

func sameDetector(left, right Detector) bool {
	if left.Kind != right.Kind || left.Path != right.Path || left.Format != right.Format ||
		left.Pattern != right.Pattern || left.Query != right.Query || left.Value != right.Value ||
		left.URL != right.URL || left.Ecosystem != right.Ecosystem || left.Field != right.Field ||
		left.Label != right.Label || left.Color != right.Color || left.Logo != right.Logo {
		return false
	}
	if len(left.Command) != len(right.Command) {
		return false
	}
	for index := range left.Command {
		if left.Command[index] != right.Command[index] {
			return false
		}
	}
	return true
}

func fileExists(projectDir, name string) bool {
	info, err := os.Stat(filepath.Join(projectDir, filepath.FromSlash(name)))
	return err == nil && !info.IsDir()
}
