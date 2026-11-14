package system

// PackageUpdate is one pending package update on the host.
type PackageUpdate struct {
	Name      string `json:"name" cbor:"0,keyasint"`
	Current   string `json:"current,omitempty" cbor:"1,keyasint,omitempty"` // installed version, empty if unknown
	Available string `json:"available" cbor:"2,keyasint"`
	Security  bool   `json:"security,omitzero" cbor:"3,keyasint,omitempty"`
}

// PackageUpdates is the detail payload returned by the agent for the
// GetPackageUpdates action. The counts in Info.PackageUpdates come from the same check.
type PackageUpdates struct {
	Manager string `json:"manager,omitempty" cbor:"0,keyasint,omitempty"`
	// CheckedAt is the Unix time in seconds of the last check, 0 if none has finished.
	CheckedAt int64 `json:"checkedAt,omitzero" cbor:"1,keyasint,omitempty"`
	// SecurityKnown is true if the package manager flags security updates per package.
	SecurityKnown bool            `json:"securityKnown,omitzero" cbor:"2,keyasint,omitempty"`
	Packages      []PackageUpdate `json:"packages" cbor:"3,keyasint"`
	// ApplyEnabled is true when the agent accepts ApplyPackageUpdates requests.
	ApplyEnabled bool `json:"applyEnabled,omitzero" cbor:"4,keyasint,omitempty"`
	// AutoApply is "all", "security", or "off" when apply is enabled; empty if apply is disabled.
	AutoApply string `json:"autoApply,omitempty" cbor:"5,keyasint,omitempty"`
	// LastAutoApplyAt is unix seconds of the last successful apply (manual or auto).
	LastAutoApplyAt int64 `json:"lastAutoApplyAt,omitzero" cbor:"6,keyasint,omitempty"`
}

// PackageUpdatesAutoApplyResult is returned after changing automatic apply mode.
type PackageUpdatesAutoApplyResult struct {
	Ok    bool   `json:"ok" cbor:"0,keyasint"`
	Mode  string `json:"mode,omitempty" cbor:"1,keyasint,omitempty"` // all|security|off
	Error string `json:"error,omitempty" cbor:"2,keyasint,omitempty"`
}

// PackageUpdatesApplyResult is returned after an apply attempt.
// Status is "started" | "running" | "done" | "error" for async applies.
type PackageUpdatesApplyResult struct {
	Ok        bool   `json:"ok" cbor:"0,keyasint"`
	Manager   string `json:"manager,omitempty" cbor:"1,keyasint,omitempty"`
	Mode      string `json:"mode,omitempty" cbor:"2,keyasint,omitempty"`
	Applied   int    `json:"applied,omitzero" cbor:"3,keyasint,omitempty"`
	Output    string `json:"output,omitempty" cbor:"4,keyasint,omitempty"`
	Error     string `json:"error,omitempty" cbor:"5,keyasint,omitempty"`
	CheckedAt int64  `json:"checkedAt,omitzero" cbor:"6,keyasint,omitempty"`
	Status    string `json:"status,omitempty" cbor:"7,keyasint,omitempty"`
}
