package library

import (
	"fmt"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
)

// DefaultsResult reports the effective library defaults.
type DefaultsResult struct {
	DryRun   bool           `json:"dryRun"`
	Changed  bool           `json:"changed"`
	Source   string         `json:"source"`
	Settings model.Settings `json:"settings"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (result DefaultsResult) DryRunResult() bool { return result.DryRun }

func (result DefaultsResult) Text() string {
	return fmt.Sprintf("defaults (%s): numbering=%t toc=%t index-depth=%d output=%s",
		result.Source, result.Settings.Numbering.Enabled, result.Settings.TOC.Enabled,
		result.Settings.IndexDepth, result.Settings.Output)
}

// Defaults returns the effective library defaults and where they come from.
func (service *Service) Defaults() (DefaultsResult, *cli.Error) {
	registry, registryErr := service.loadRegistry()
	if registryErr != nil {
		return DefaultsResult{}, registryErr
	}
	return defaultsOf(registry), nil
}

// SetDefault changes one library default. The value is normalized before it is stored.
func (service *Service) SetDefault(key, value string, dryRun bool) (DefaultsResult, *cli.Error) {
	release, lockErr := service.Life.LockStore()
	if lockErr != nil {
		return DefaultsResult{}, cli.AsError(lockErr)
	}
	defer func() { _ = release() }()
	registry, registryErr := service.loadRegistry()
	if registryErr != nil {
		return DefaultsResult{}, registryErr
	}
	settings := registry.EffectiveDefaults()
	if err := structure.ApplySettingValue(&settings, key, value); err != nil {
		return DefaultsResult{}, cli.Usage("usage.invalid-value", "%v", err)
	}
	settings = settings.Normalized()
	result := DefaultsResult{DryRun: dryRun, Changed: true, Source: "library", Settings: settings}
	if dryRun {
		return result, nil
	}
	registry.Defaults = &settings
	if saveErr := registry.Save(service.Layout.Registry()); saveErr != nil {
		return DefaultsResult{}, cli.AsError(saveErr)
	}
	return result, nil
}

// UnsetDefaults removes the library defaults and returns to the built-in defaults.
func (service *Service) UnsetDefaults(dryRun bool) (DefaultsResult, *cli.Error) {
	release, lockErr := service.Life.LockStore()
	if lockErr != nil {
		return DefaultsResult{}, cli.AsError(lockErr)
	}
	defer func() { _ = release() }()
	registry, registryErr := service.loadRegistry()
	if registryErr != nil {
		return DefaultsResult{}, registryErr
	}
	result := DefaultsResult{
		DryRun: dryRun, Changed: registry.Defaults != nil, Source: "builtin",
		Settings: model.DefaultSettings(),
	}
	if dryRun || !result.Changed {
		return result, nil
	}
	registry.Defaults = nil
	if saveErr := registry.Save(service.Layout.Registry()); saveErr != nil {
		return DefaultsResult{}, cli.AsError(saveErr)
	}
	return result, nil
}

func defaultsOf(registry *store.Registry) DefaultsResult {
	source := "builtin"
	if registry.Defaults != nil {
		source = "library"
	}
	return DefaultsResult{Source: source, Settings: registry.EffectiveDefaults()}
}
