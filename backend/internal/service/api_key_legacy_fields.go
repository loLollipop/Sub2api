package service

// ModelScopeAll is retained only for the historical database column.
// API keys are authorized by their native groups and model allowlists.
const ModelScopeAll = "all"

func NormalizeModelScope(scope string) string {
	if scope == "" {
		return ModelScopeAll
	}
	return scope
}
