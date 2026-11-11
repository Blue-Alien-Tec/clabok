package config

import (
	"encoding/json"
	"reflect"

	"github.com/Blue-Alien-Tec/clabok/internal/badge"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

// PutDetector stores one detector in the project configuration. It is the programmatic form of
// `config set badges.detectors.<id> <json>` and returns whether the document changed.
func PutDetector(env *cli.Env, id string, detector badge.Detector, dryRun bool) (bool, *cli.Error) {
	document, location, err := readDocument(env, true)
	if err != nil {
		return false, err
	}
	key := detectorPrefix + id
	before, _, beforeErr := lookupValue(document, key)
	if beforeErr != nil {
		return false, beforeErr
	}
	raw, marshalErr := json.Marshal(detector)
	if marshalErr != nil {
		return false, cli.Fail("config.write-failed", "%v", marshalErr)
	}
	if setErr := setValue(document, key, string(raw)); setErr != nil {
		return false, setErr
	}
	if issues := validate(document); len(issues) > 0 {
		return false, invalidError(issues)
	}
	after, _, afterErr := lookupValue(document, key)
	if afterErr != nil {
		return false, afterErr
	}
	changed := !reflect.DeepEqual(before, after) || !location.Exists
	if dryRun || !changed {
		return changed, nil
	}
	if writeErr := writeFile(targetFile(location), document); writeErr != nil {
		return false, writeErr
	}
	return true, nil
}
