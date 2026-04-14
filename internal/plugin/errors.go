package plugin

// PluginError is the base error type for all plugin errors.
// Specific plugin error types embed this struct and may add extra fields.
type PluginError struct {
	Code    string
	Message string
	Status  int
}

func (e *PluginError) Error() string { return e.Message }

// As allows errors.As to match embedded PluginError fields.
func (e *PluginError) As(target any) bool {
	if t, ok := target.(**PluginError); ok {
		*t = e
		return true
	}
	return false
}
