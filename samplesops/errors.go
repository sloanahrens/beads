package samplesops

import "github.com/steveyegge/beads/beadserrors"

// ErrValidation classifies this role's deterministic request-validation
// failures. It is an ALIAS of beadserrors.ErrValidation, not a second sentinel,
// so every errors.Is site that already classifies a validation refusal keeps
// matching the same value whichever leaf's spelling it holds.
//
// It is re-exported here rather than left for callers to reach through
// beadserrors so that code holding only the Samples interface can classify a
// refusal without discovering a second package.
var ErrValidation = beadserrors.ErrValidation
