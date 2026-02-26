package crypto

import "errors"

// ZK proof system identifiers.
const (
	ProofTypeGroth16 = "groth16"
	ProofTypePlonk   = "plonk"
)

// PlaceholderZKVerifier is a stub implementation of ZKVerifier.
// The real implementation will be provided in MODULE_CONSENSUS_03.
type PlaceholderZKVerifier struct {
	proofType string
}

// NewPlaceholderZKVerifier creates a placeholder verifier for the given proof type.
func NewPlaceholderZKVerifier(proofType string) *PlaceholderZKVerifier {
	return &PlaceholderZKVerifier{proofType: proofType}
}

// Verify always returns an error indicating the verifier is not yet implemented.
func (v *PlaceholderZKVerifier) Verify(proof *ZKProof) (bool, error) {
	if proof == nil {
		return false, errors.New("zk: nil proof")
	}
	if proof.ProofType != v.proofType {
		return false, errors.New("zk: proof type mismatch")
	}
	return false, errors.New("zk: verifier not yet implemented (see MODULE_CONSENSUS_03)")
}

// ProofType returns the proof system name.
func (v *PlaceholderZKVerifier) ProofType() string {
	return v.proofType
}

// ValidateZKProofStructure checks that a ZKProof has all required fields populated.
func ValidateZKProofStructure(proof *ZKProof) error {
	if proof == nil {
		return errors.New("zk: nil proof")
	}
	if len(proof.ProofData) == 0 {
		return errors.New("zk: empty proof data")
	}
	if len(proof.VerificationKey) == 0 {
		return errors.New("zk: empty verification key")
	}
	if proof.ProofType == "" {
		return errors.New("zk: empty proof type")
	}
	return nil
}
