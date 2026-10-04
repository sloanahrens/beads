package utils

import "os"

// townRootEnvNames lists the environment variables that name the orchestrator
// (town) root, in precedence order. GT_TOWN_ROOT replaced the older GT_ROOT;
// GT_ROOT stays here as a deprecated alias read only when GT_TOWN_ROOT is
// empty, so a release that still exports GT_ROOT keeps working.
var townRootEnvNames = []string{"GT_TOWN_ROOT", "GT_ROOT"}

// TownRoot returns the orchestrator (town) root named by the environment, or
// "" when neither GT_TOWN_ROOT nor the deprecated GT_ROOT alias is set.
// GT_TOWN_ROOT wins; GT_ROOT is consulted only when GT_TOWN_ROOT is empty.
func TownRoot() string {
	for _, name := range townRootEnvNames {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return ""
}

// TownRootEnvNames returns the town-root environment variables in precedence
// order. Callers that must try each candidate separately — validating a town
// marker under each root, for instance — use this instead of TownRoot, which
// takes only the first non-empty value.
func TownRootEnvNames() []string {
	return append([]string(nil), townRootEnvNames...)
}
