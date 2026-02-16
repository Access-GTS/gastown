package session

import (
	"testing"
)

func TestParseSessionName(t *testing.T) {
	// Register prefixes for test
	ClearPrefixRegistry()
	RegisterRigPrefix("gastown", "gt")
	RegisterRigPrefix("beads", "bd")
	RegisterRigPrefix("hop", "hop")
	RegisterRigPrefix("sky", "sky")
	RegisterRigPrefix("foo-bar", "fb")
	defer ClearPrefixRegistry()

	tests := []struct {
		name     string
		session  string
		wantRole Role
		wantRig  string
		wantName string
		wantErr  bool
	}{
		// Town-level roles (hq-mayor, hq-deacon)
		{
			name:     "mayor",
			session:  "hq-mayor",
			wantRole: RoleMayor,
		},
		{
			name:     "deacon",
			session:  "hq-deacon",
			wantRole: RoleDeacon,
		},

		// Boot watchdog (town-level)
		{
			name:     "boot",
			session:  "hq-boot",
			wantRole: RoleDeacon,
			wantName: "boot",
		},

		// Witness (using rig prefix)
		{
			name:     "witness gastown",
			session:  "gt-witness",
			wantRole: RoleWitness,
			wantRig:  "gastown",
		},
		{
			name:     "witness beads",
			session:  "bd-witness",
			wantRole: RoleWitness,
			wantRig:  "beads",
		},
		{
			name:     "witness hop",
			session:  "hop-witness",
			wantRole: RoleWitness,
			wantRig:  "hop",
		},

		// Refinery (using rig prefix)
		{
			name:     "refinery gastown",
			session:  "gt-refinery",
			wantRole: RoleRefinery,
			wantRig:  "gastown",
		},
		{
			name:     "refinery beads",
			session:  "bd-refinery",
			wantRole: RoleRefinery,
			wantRig:  "beads",
		},

		// Crew (with marker)
		{
			name:     "crew gastown",
			session:  "gt-crew-max",
			wantRole: RoleCrew,
			wantRig:  "gastown",
			wantName: "max",
		},
		{
			name:     "crew beads",
			session:  "bd-crew-alice",
			wantRole: RoleCrew,
			wantRig:  "beads",
			wantName: "alice",
		},
		{
			name:     "crew hyphenated name",
			session:  "gt-crew-my-worker",
			wantRole: RoleCrew,
			wantRig:  "gastown",
			wantName: "my-worker",
		},

		// Polecat (fallback)
		{
			name:     "polecat gastown",
			session:  "gt-morsov",
			wantRole: RolePolecat,
			wantRig:  "gastown",
			wantName: "morsov",
		},
		{
			name:     "polecat beads",
			session:  "bd-worker1",
			wantRole: RolePolecat,
			wantRig:  "beads",
			wantName: "worker1",
		},
		{
			name:     "polecat hop",
			session:  "hop-ostrom",
			wantRole: RolePolecat,
			wantRig:  "hop",
			wantName: "ostrom",
		},

		// Error cases
		{
			name:    "unregistered prefix",
			session: "xyz-witness",
			wantErr: true,
		},
		{
			name:    "empty",
			session: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSessionName(tt.session)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSessionName(%q) error = %v, wantErr %v", tt.session, err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.Role != tt.wantRole {
				t.Errorf("ParseSessionName(%q).Role = %v, want %v", tt.session, got.Role, tt.wantRole)
			}
			if got.Rig != tt.wantRig {
				t.Errorf("ParseSessionName(%q).Rig = %v, want %v", tt.session, got.Rig, tt.wantRig)
			}
			if got.Name != tt.wantName {
				t.Errorf("ParseSessionName(%q).Name = %v, want %v", tt.session, got.Name, tt.wantName)
			}
		})
	}
}

func TestAgentIdentity_SessionName(t *testing.T) {
	// Register prefixes for test
	ClearPrefixRegistry()
	RegisterRigPrefix("gastown", "gt")
	RegisterRigPrefix("beads", "bd")
	RegisterRigPrefix("my-project", "mp")
	defer ClearPrefixRegistry()

	tests := []struct {
		name     string
		identity AgentIdentity
		want     string
	}{
		{
			name:     "mayor",
			identity: AgentIdentity{Role: RoleMayor},
			want:     "hq-mayor",
		},
		{
			name:     "deacon",
			identity: AgentIdentity{Role: RoleDeacon},
			want:     "hq-deacon",
		},
		{
			name:     "boot",
			identity: AgentIdentity{Role: RoleDeacon, Name: "boot"},
			want:     "hq-boot",
		},
		{
			name:     "witness with prefix",
			identity: AgentIdentity{Role: RoleWitness, Rig: "gastown", Prefix: "gt"},
			want:     "gt-witness",
		},
		{
			name:     "witness via registry",
			identity: AgentIdentity{Role: RoleWitness, Rig: "gastown"},
			want:     "gt-witness",
		},
		{
			name:     "refinery",
			identity: AgentIdentity{Role: RoleRefinery, Rig: "my-project", Prefix: "mp"},
			want:     "mp-refinery",
		},
		{
			name:     "crew",
			identity: AgentIdentity{Role: RoleCrew, Rig: "gastown", Name: "max", Prefix: "gt"},
			want:     "gt-crew-max",
		},
		{
			name:     "polecat",
			identity: AgentIdentity{Role: RolePolecat, Rig: "gastown", Name: "morsov", Prefix: "gt"},
			want:     "gt-morsov",
		},
		{
			name:     "polecat beads",
			identity: AgentIdentity{Role: RolePolecat, Rig: "beads", Name: "worker1", Prefix: "bd"},
			want:     "bd-worker1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.identity.SessionName(); got != tt.want {
				t.Errorf("AgentIdentity.SessionName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentIdentity_Address(t *testing.T) {
	tests := []struct {
		name     string
		identity AgentIdentity
		want     string
	}{
		{
			name:     "mayor",
			identity: AgentIdentity{Role: RoleMayor},
			want:     "mayor",
		},
		{
			name:     "deacon",
			identity: AgentIdentity{Role: RoleDeacon},
			want:     "deacon",
		},
		{
			name:     "witness",
			identity: AgentIdentity{Role: RoleWitness, Rig: "gastown"},
			want:     "gastown/witness",
		},
		{
			name:     "refinery",
			identity: AgentIdentity{Role: RoleRefinery, Rig: "my-project"},
			want:     "my-project/refinery",
		},
		{
			name:     "crew",
			identity: AgentIdentity{Role: RoleCrew, Rig: "gastown", Name: "max"},
			want:     "gastown/crew/max",
		},
		{
			name:     "polecat",
			identity: AgentIdentity{Role: RolePolecat, Rig: "gastown", Name: "Toast"},
			want:     "gastown/polecats/Toast",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.identity.Address(); got != tt.want {
				t.Errorf("AgentIdentity.Address() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSessionName_RoundTrip(t *testing.T) {
	// Register prefixes for test
	ClearPrefixRegistry()
	RegisterRigPrefix("gastown", "gt")
	RegisterRigPrefix("foo-bar", "fb")
	defer ClearPrefixRegistry()

	sessions := []string{
		"hq-mayor",
		"hq-deacon",
		"gt-witness",
		"fb-refinery",
		"gt-crew-max",
		"gt-morsov",
	}

	for _, sess := range sessions {
		t.Run(sess, func(t *testing.T) {
			identity, err := ParseSessionName(sess)
			if err != nil {
				t.Fatalf("ParseSessionName(%q) error = %v", sess, err)
			}
			if got := identity.SessionName(); got != sess {
				t.Errorf("Round-trip failed: ParseSessionName(%q).SessionName() = %q", sess, got)
			}
		})
	}
}

func TestParseAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    AgentIdentity
		wantErr bool
	}{
		{
			name:    "mayor",
			address: "mayor/",
			want:    AgentIdentity{Role: RoleMayor},
		},
		{
			name:    "deacon",
			address: "deacon",
			want:    AgentIdentity{Role: RoleDeacon},
		},
		{
			name:    "witness",
			address: "gastown/witness",
			want:    AgentIdentity{Role: RoleWitness, Rig: "gastown"},
		},
		{
			name:    "refinery",
			address: "rig-a/refinery",
			want:    AgentIdentity{Role: RoleRefinery, Rig: "rig-a"},
		},
		{
			name:    "crew",
			address: "gastown/crew/max",
			want:    AgentIdentity{Role: RoleCrew, Rig: "gastown", Name: "max"},
		},
		{
			name:    "polecat explicit",
			address: "gastown/polecats/nux",
			want:    AgentIdentity{Role: RolePolecat, Rig: "gastown", Name: "nux"},
		},
		{
			name:    "polecat canonical",
			address: "gastown/nux",
			want:    AgentIdentity{Role: RolePolecat, Rig: "gastown", Name: "nux"},
		},
		{
			name:    "invalid",
			address: "gastown/crew",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAddress(tt.address)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAddress(%q) error = %v", tt.address, err)
			}
			if got.Role != tt.want.Role || got.Rig != tt.want.Rig || got.Name != tt.want.Name {
				t.Fatalf("ParseAddress(%q) = %#v, want %#v", tt.address, *got, tt.want)
			}
		})
	}
}

func TestPrefixRegistry(t *testing.T) {
	ClearPrefixRegistry()
	defer ClearPrefixRegistry()

	// Test registration
	RegisterRigPrefix("gastown", "gt")
	RegisterRigPrefix("beads", "bd")

	// Test PrefixForRig
	if got := PrefixForRig("gastown"); got != "gt" {
		t.Errorf("PrefixForRig(gastown) = %q, want %q", got, "gt")
	}
	if got := PrefixForRig("beads"); got != "bd" {
		t.Errorf("PrefixForRig(beads) = %q, want %q", got, "bd")
	}
	if got := PrefixForRig("unknown"); got != "gt" {
		t.Errorf("PrefixForRig(unknown) = %q, want %q (fallback)", got, "gt")
	}

	// Test RigForPrefix
	if got := RigForPrefix("gt"); got != "gastown" {
		t.Errorf("RigForPrefix(gt) = %q, want %q", got, "gastown")
	}
	if got := RigForPrefix("bd"); got != "beads" {
		t.Errorf("RigForPrefix(bd) = %q, want %q", got, "beads")
	}
	if got := RigForPrefix("xyz"); got != "xyz" {
		t.Errorf("RigForPrefix(xyz) = %q, want %q (fallback)", got, "xyz")
	}

	// Test RegisteredPrefixes returns longest first
	RegisterRigPrefix("my-rig", "my-rig")
	prefixes := RegisteredPrefixes()
	if len(prefixes) < 3 {
		t.Fatalf("RegisteredPrefixes() returned %d, want >= 3", len(prefixes))
	}
	// my-rig (6 chars) should come before gt (2 chars)
	if prefixes[0] != "my-rig" {
		t.Errorf("RegisteredPrefixes()[0] = %q, want %q (longest first)", prefixes[0], "my-rig")
	}
}
