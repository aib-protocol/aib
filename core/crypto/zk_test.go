package crypto

import (
	"testing"
)

func TestPlaceholderZKVerifier(t *testing.T) {
	verifier := NewPlaceholderZKVerifier(ProofTypeGroth16)

	if verifier.ProofType() != ProofTypeGroth16 {
		t.Fatalf("expected %s, got %s", ProofTypeGroth16, verifier.ProofType())
	}

	proof := &ZKProof{
		ProofData:       []byte("fake proof"),
		PublicInputs:    [][]byte{[]byte("input1")},
		VerificationKey: []byte("fake key"),
		ProofType:       ProofTypeGroth16,
	}

	// Should return false with "not implemented" error
	valid, err := verifier.Verify(proof)
	if valid {
		t.Fatal("placeholder should not validate proofs")
	}
	if err == nil {
		t.Fatal("placeholder should return error")
	}
}

func TestPlaceholderZKVerifierNilProof(t *testing.T) {
	verifier := NewPlaceholderZKVerifier(ProofTypeGroth16)
	_, err := verifier.Verify(nil)
	if err == nil {
		t.Fatal("should reject nil proof")
	}
}

func TestPlaceholderZKVerifierTypeMismatch(t *testing.T) {
	verifier := NewPlaceholderZKVerifier(ProofTypeGroth16)

	proof := &ZKProof{
		ProofData:       []byte("fake"),
		PublicInputs:    [][]byte{},
		VerificationKey: []byte("key"),
		ProofType:       ProofTypePlonk,
	}

	_, err := verifier.Verify(proof)
	if err == nil {
		t.Fatal("should reject mismatched proof type")
	}
}

func TestValidateZKProofStructure(t *testing.T) {
	tests := []struct {
		name    string
		proof   *ZKProof
		wantErr bool
	}{
		{
			name:    "nil proof",
			proof:   nil,
			wantErr: true,
		},
		{
			name: "empty proof data",
			proof: &ZKProof{
				ProofData:       []byte{},
				VerificationKey: []byte("key"),
				ProofType:       ProofTypeGroth16,
			},
			wantErr: true,
		},
		{
			name: "empty verification key",
			proof: &ZKProof{
				ProofData:       []byte("data"),
				VerificationKey: []byte{},
				ProofType:       ProofTypeGroth16,
			},
			wantErr: true,
		},
		{
			name: "empty proof type",
			proof: &ZKProof{
				ProofData:       []byte("data"),
				VerificationKey: []byte("key"),
				ProofType:       "",
			},
			wantErr: true,
		},
		{
			name: "valid structure",
			proof: &ZKProof{
				ProofData:       []byte("data"),
				PublicInputs:    [][]byte{[]byte("input")},
				VerificationKey: []byte("key"),
				ProofType:       ProofTypeGroth16,
			},
			wantErr: false,
		},
		{
			name: "valid without public inputs",
			proof: &ZKProof{
				ProofData:       []byte("data"),
				VerificationKey: []byte("key"),
				ProofType:       ProofTypePlonk,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateZKProofStructure(tt.proof)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateZKProofStructure() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestZKProofConstants(t *testing.T) {
	if ProofTypeGroth16 != "groth16" {
		t.Fatalf("ProofTypeGroth16 should be 'groth16'")
	}
	if ProofTypePlonk != "plonk" {
		t.Fatalf("ProofTypePlonk should be 'plonk'")
	}
}
