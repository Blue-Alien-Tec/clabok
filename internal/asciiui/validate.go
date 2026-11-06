package asciiui

// Validate checks that a spec document parses. The authoring command calls it before it stores a
// spec, so a malformed drawing is rejected while it is still in the command, not at render time.
func Validate(text string) error {
	_, err := Parse(text)
	return err
}
