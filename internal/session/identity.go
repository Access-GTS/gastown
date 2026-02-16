// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Role represents the type of Gas Town agent.
type Role string

const (
	RoleMayor    Role = "mayor"
	RoleDeacon   Role = "deacon"
	RoleWitness  Role = "witness"
	RoleRefinery Role = "refinery"
	RoleCrew     Role = "crew"
	RolePolecat  Role = "polecat"
)

// AgentIdentity represents a parsed Gas Town agent identity.
type AgentIdentity struct {
	Role   Role   // mayor, deacon, witness, refinery, crew, polecat
	Rig    string // rig name (empty for mayor/deacon)
	Name   string // crew/polecat name (empty for mayor/deacon/witness/refinery)
	Prefix string // beads prefix for the rig (e.g., "gt" for gastown)
}

// Prefix registry maps between rig names and their beads prefixes.
// Used by ParseSessionName to determine rig from session prefix,
// and by SessionName() to look up prefix from rig name.
var (
	prefixToRig = map[string]string{} // prefix → rig name
	rigToPrefix = map[string]string{} // rig name → prefix
	registryMu  sync.RWMutex
)

// RegisterRigPrefix registers a mapping between a rig name and its beads prefix.
// This must be called at startup for each rig so that ParseSessionName can
// correctly determine the rig from a session name.
func RegisterRigPrefix(rigName, prefix string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	prefixToRig[prefix] = rigName
	rigToPrefix[rigName] = prefix
}

// PrefixForRig returns the beads prefix for a rig name.
// Returns "gt" as fallback if the rig is not registered.
func PrefixForRig(rigName string) string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if p, ok := rigToPrefix[rigName]; ok {
		return p
	}
	return "gt" // fallback for gastown default
}

// RigForPrefix returns the rig name for a beads prefix.
// Returns the prefix itself as fallback if not registered (prefix often equals rig name).
func RigForPrefix(prefix string) string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if r, ok := prefixToRig[prefix]; ok {
		return r
	}
	return prefix // fallback: prefix is often the rig name itself
}

// RegisteredPrefixes returns all registered prefixes sorted longest-first
// for use in session name parsing (longest match wins).
func RegisteredPrefixes() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	prefixes := make([]string, 0, len(prefixToRig))
	for p := range prefixToRig {
		prefixes = append(prefixes, p)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i]) > len(prefixes[j])
	})
	return prefixes
}

// ClearPrefixRegistry clears the prefix registry. Used in tests.
func ClearPrefixRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	prefixToRig = map[string]string{}
	rigToPrefix = map[string]string{}
}

// ParseAddress parses a mail-style address into an AgentIdentity.
func ParseAddress(address string) (*AgentIdentity, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("empty address")
	}

	if address == "mayor" || address == "mayor/" {
		return &AgentIdentity{Role: RoleMayor}, nil
	}
	if address == "deacon" || address == "deacon/" {
		return &AgentIdentity{Role: RoleDeacon}, nil
	}
	if address == "overseer" {
		return nil, fmt.Errorf("overseer has no session")
	}

	address = strings.TrimSuffix(address, "/")
	parts := strings.Split(address, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid address %q", address)
	}

	rig := parts[0]
	switch len(parts) {
	case 2:
		name := parts[1]
		switch name {
		case "witness":
			return &AgentIdentity{Role: RoleWitness, Rig: rig}, nil
		case "refinery":
			return &AgentIdentity{Role: RoleRefinery, Rig: rig}, nil
		case "crew", "polecats":
			return nil, fmt.Errorf("invalid address %q", address)
		default:
			return &AgentIdentity{Role: RolePolecat, Rig: rig, Name: name}, nil
		}
	case 3:
		role := parts[1]
		name := parts[2]
		switch role {
		case "crew":
			return &AgentIdentity{Role: RoleCrew, Rig: rig, Name: name}, nil
		case "polecats":
			return &AgentIdentity{Role: RolePolecat, Rig: rig, Name: name}, nil
		default:
			return nil, fmt.Errorf("invalid address %q", address)
		}
	default:
		return nil, fmt.Errorf("invalid address %q", address)
	}
}

// ParseSessionName parses a tmux session name into an AgentIdentity.
//
// Session name formats:
//   - hq-mayor → Role: mayor (town-level, one per machine)
//   - hq-deacon → Role: deacon (town-level, one per machine)
//   - <prefix>-witness → Role: witness, Rig: (looked up from prefix)
//   - <prefix>-refinery → Role: refinery, Rig: (looked up from prefix)
//   - <prefix>-crew-<name> → Role: crew, Rig: (looked up), Name: <name>
//   - <prefix>-<name> → Role: polecat, Rig: (looked up), Name: <name>
//
// The prefix is the rig's beads prefix (e.g., "gt" for gastown, "bd" for beads).
// RegisterRigPrefix must be called at startup to populate the prefix→rig mapping.
func ParseSessionName(session string) (*AgentIdentity, error) {
	// Check for town-level roles (hq- prefix)
	if strings.HasPrefix(session, HQPrefix) {
		suffix := strings.TrimPrefix(session, HQPrefix)
		if suffix == "mayor" {
			return &AgentIdentity{Role: RoleMayor, Prefix: "hq"}, nil
		}
		if suffix == "deacon" {
			return &AgentIdentity{Role: RoleDeacon, Prefix: "hq"}, nil
		}
		if suffix == "boot" {
			return &AgentIdentity{Role: RoleDeacon, Name: "boot", Prefix: "hq"}, nil
		}
		if suffix == "overseer" {
			return &AgentIdentity{Prefix: "hq"}, fmt.Errorf("invalid session name %q: overseer is not an agent", session)
		}
		return nil, fmt.Errorf("invalid session name %q: unknown hq- role", session)
	}

	// Try registered rig prefixes (longest match first to avoid ambiguity)
	prefixes := RegisteredPrefixes()
	for _, prefix := range prefixes {
		marker := prefix + "-"
		if !strings.HasPrefix(session, marker) {
			continue
		}
		suffix := strings.TrimPrefix(session, marker)
		if suffix == "" {
			return nil, fmt.Errorf("invalid session name %q: empty after prefix", session)
		}
		rigName := RigForPrefix(prefix)
		return parseRigSuffix(suffix, rigName, prefix)
	}

	return nil, fmt.Errorf("invalid session name %q: no registered prefix matches", session)
}

// parseRigSuffix parses the suffix after the rig prefix to determine role and name.
// suffix is everything after "<prefix>-", e.g., "witness", "crew-max", "furiosa".
func parseRigSuffix(suffix, rigName, prefix string) (*AgentIdentity, error) {
	// Check for witness/refinery
	if suffix == "witness" {
		return &AgentIdentity{Role: RoleWitness, Rig: rigName, Prefix: prefix}, nil
	}
	if suffix == "refinery" {
		return &AgentIdentity{Role: RoleRefinery, Rig: rigName, Prefix: prefix}, nil
	}

	// Check for crew (prefix: "crew-<name>")
	if strings.HasPrefix(suffix, "crew-") {
		name := strings.TrimPrefix(suffix, "crew-")
		if name == "" {
			return nil, fmt.Errorf("invalid session suffix %q: empty crew name", suffix)
		}
		return &AgentIdentity{Role: RoleCrew, Rig: rigName, Name: name, Prefix: prefix}, nil
	}

	// Default to polecat
	return &AgentIdentity{Role: RolePolecat, Rig: rigName, Name: suffix, Prefix: prefix}, nil
}

// SessionName returns the tmux session name for this identity.
// Uses a.Prefix if set, otherwise looks up the prefix from the registry.
func (a *AgentIdentity) SessionName() string {
	prefix := a.Prefix
	if prefix == "" {
		prefix = PrefixForRig(a.Rig)
	}
	switch a.Role {
	case RoleMayor:
		return MayorSessionName()
	case RoleDeacon:
		if a.Name == "boot" {
			return BootSessionName()
		}
		return DeaconSessionName()
	case RoleWitness:
		return WitnessSessionName(prefix)
	case RoleRefinery:
		return RefinerySessionName(prefix)
	case RoleCrew:
		return CrewSessionName(prefix, a.Name)
	case RolePolecat:
		return PolecatSessionName(prefix, a.Name)
	default:
		return ""
	}
}

// Address returns the mail-style address for this identity.
// Examples:
//   - mayor → "mayor"
//   - deacon → "deacon"
//   - witness → "gastown/witness"
//   - refinery → "gastown/refinery"
//   - crew → "gastown/crew/max"
//   - polecat → "gastown/polecats/Toast"
func (a *AgentIdentity) Address() string {
	switch a.Role {
	case RoleMayor:
		return "mayor"
	case RoleDeacon:
		return "deacon"
	case RoleWitness:
		return fmt.Sprintf("%s/witness", a.Rig)
	case RoleRefinery:
		return fmt.Sprintf("%s/refinery", a.Rig)
	case RoleCrew:
		return fmt.Sprintf("%s/crew/%s", a.Rig, a.Name)
	case RolePolecat:
		return fmt.Sprintf("%s/polecats/%s", a.Rig, a.Name)
	default:
		return ""
	}
}

// GTRole returns the GT_ROLE environment variable format.
// This is the same as Address() for most roles.
func (a *AgentIdentity) GTRole() string {
	return a.Address()
}
