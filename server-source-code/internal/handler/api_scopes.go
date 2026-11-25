package handler

import "encoding/json"

// Scope grants are always explicit: new resources and actions require an entry here.
var apiScopeActions = map[string]map[string]bool{
	// Legacy host write selections remain valid for editing older credentials.
	// No scoped route grants these actions any capabilities.
	"host":             {"get": true, "put": true, "patch": true, "update": true, "delete": true},
	"host_group":       {"get": true, "create": true, "update": true, "delete": true},
	"enrollment_token": {"get": true, "create": true, "update": true, "delete": true},
	"patching":         {"trigger": true},
}

func validateScopes(raw json.RawMessage) error {
	var scopes map[string]json.RawMessage
	if err := json.Unmarshal(raw, &scopes); err != nil || scopes == nil {
		return &scopeError{"Scopes must be an object"}
	}
	for resource, rawActions := range scopes {
		allowedActions, ok := apiScopeActions[resource]
		if !ok {
			return &scopeError{"Unknown scope resource \"" + resource + "\""}
		}
		var actions []string
		if err := json.Unmarshal(rawActions, &actions); err != nil || actions == nil {
			return &scopeError{"Scopes for resource \"" + resource + "\" must be an array of actions"}
		}
		for _, action := range actions {
			if !allowedActions[action] {
				return &scopeError{"Unknown action \"" + action + "\" for scope resource \"" + resource + "\""}
			}
		}
	}
	return nil
}

type scopeError struct{ msg string }

func (e *scopeError) Error() string { return e.msg }
