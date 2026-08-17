// Package wallet provides backup and restore tests for wallet security boundaries.
package wallet

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// generateMnemonicFromEntropy creates a deterministic mnemonic-like phrase from entropy.
// This test helper is used to verify generation properties and boundary conditions.
func generateMnemonicFromEntropy(entropy []byte) (string, error) {
	if len(entropy) < 16 || len(entropy) > 32 || len(entropy)%4 != 0 {
		return "", ErrInvalidEntropyLength
	}

	words := make([]string, len(entropy))
	for i, b := range entropy {
		words[i] = "w" + hex.EncodeToString([]byte{b})
	}
	return strings.Join(words, " "), nil
}

// mnemonicToEd25519Seed derives a deterministic 32-byte seed from mnemonic text.
func mnemonicToEd25519Seed(mnemonic string) ([]byte, error) {
	if strings.TrimSpace(mnemonic) == "" {
		return nil, ErrEmptyMnemonic
	}
	hash := sha256.Sum256([]byte(mnemonic))
	seed := make([]byte, ed25519.SeedSize)
	copy(seed, hash[:])
	return seed, nil
}

// encryptPrivateKey encrypts private key bytes with AES-256-GCM.
func encryptPrivateKey(privKey, password, nonce []byte) ([]byte, error) {
	if len(privKey) != ed25519.PrivateKeySize {
		return nil, ErrInvalidPrivateKeySize
	}
	if len(password) == 0 {
		return nil, ErrEmptyPassword
	}
	if len(nonce) != 12 {
		return nil, ErrInvalidNonceSize
	}

	key := sha256.Sum256(password)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, privKey, nil)
	return ciphertext, nil
}

// decryptPrivateKey decrypts private key bytes with AES-256-GCM.
func decryptPrivateKey(ciphertext, password, nonce []byte) ([]byte, error) {
	if len(password) == 0 {
		return nil, ErrEmptyPassword
	}
	if len(nonce) != 12 {
		return nil, ErrInvalidNonceSize
	}

	key := sha256.Sum256(password)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	if len(plain) != ed25519.PrivateKeySize {
		return nil, ErrInvalidPrivateKeySize
	}

	return plain, nil
}

var (
	ErrInvalidEntropyLength  = &backupTestError{"invalid entropy length"}
	ErrEmptyMnemonic         = &backupTestError{"empty mnemonic"}
	ErrInvalidPrivateKeySize = &backupTestError{"invalid private key size"}
	ErrEmptyPassword         = &backupTestError{"empty password"}
	ErrInvalidNonceSize      = &backupTestError{"invalid nonce size"}
)

type backupTestError struct {
	msg string
}

func (e *backupTestError) Error() string {
	return e.msg
}

// TestBackup_GenerateMnemonic tests mnemonic generation and boundary validation.
func TestBackup_GenerateMnemonic(t *testing.T) {
	entropy := make([]byte, 16)
	if _, err := rand.Read(entropy); err != nil {
		t.Fatalf("failed to generate entropy: %v", err)
	}

	mnemonic, err := generateMnemonicFromEntropy(entropy)
	if err != nil {
		t.Fatalf("failed to generate mnemonic: %v", err)
	}

	if mnemonic == "" {
		t.Fatal("mnemonic should not be empty")
	}

	words := strings.Split(mnemonic, " ")
	if len(words) != 16 {
		t.Fatalf("expected 16 words from 16-byte entropy, got %d", len(words))
	}

	shortEntropy := make([]byte, 8)
	if _, err := generateMnemonicFromEntropy(shortEntropy); err == nil {
		t.Fatal("should reject entropy shorter than 16 bytes")
	}

	invalidEntropy := make([]byte, 18)
	if _, err := generateMnemonicFromEntropy(invalidEntropy); err == nil {
		t.Fatal("should reject entropy length not divisible by 4")
	}

	seed, err := mnemonicToEd25519Seed(mnemonic)
	if err != nil {
		t.Fatalf("failed to derive seed from mnemonic: %v", err)
	}

	if len(seed) != ed25519.SeedSize {
		t.Fatalf("expected seed size %d, got %d", ed25519.SeedSize, len(seed))
	}

	pub, priv, err := ed25519.GenerateKey(bytes.NewReader(seed))
	if err != nil {
		t.Fatalf("failed to generate deterministic key from seed reader: %v", err)
	}

	if len(pub) != ed25519.PublicKeySize || len(priv) != ed25519.PrivateKeySize {
		t.Fatalf("invalid key sizes: pub=%d priv=%d", len(pub), len(priv))
	}
}

// TestBackup_PrivateKeyExportImport tests raw private key export/import with real ed25519 keys.
func TestBackup_PrivateKeyExportImport(t *testing.T) {
	original, err := NewWallet()
	if err != nil {
		t.Fatalf("failed to create original wallet: %v", err)
	}

	msg := []byte("backup-export-import-validation")
	origSig := original.Sign(msg)
	if !original.Verify(msg, origSig) {
		t.Fatal("original wallet signature verification failed")
	}

	exported := original.ExportPrivateKey()
	if len(exported) != ed25519.PrivateKeySize {
		t.Fatalf("expected private key size %d, got %d", ed25519.PrivateKeySize, len(exported))
	}

	restored, err := FromPrivateKeyBytes(exported)
	if err != nil {
		t.Fatalf("failed to restore wallet from private key: %v", err)
	}

	if original.GetAddress() != restored.GetAddress() {
		t.Fatalf("restored address mismatch: original=%s restored=%s", original.GetAddressHex(), restored.GetAddressHex())
	}

	if !bytes.Equal(original.GetPublicKey(), restored.GetPublicKey()) {
		t.Fatal("restored public key mismatch")
	}

	if !restored.Verify(msg, origSig) {
		t.Fatal("restored wallet should verify signature from original key")
	}

	invalid := exported[:len(exported)-1]
	if _, err := FromPrivateKeyBytes(invalid); err == nil {
		t.Fatal("should reject truncated private key")
	}

	// Test with a completely different key (not just mutation)
	_, priv2, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate second key: %v", err)
	}
	different, err := FromPrivateKeyBytes(priv2)
	if err != nil {
		t.Fatalf("failed to create wallet from second key: %v", err)
	}
	if different.GetAddress() == original.GetAddress() {
		t.Error("different key should not produce same address")
	}
}

// TestBackup_WalletFileEncryption tests encrypted wallet backup payload boundaries.
func TestBackup_WalletFileEncryption(t *testing.T) {
	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	priv := wallet.ExportPrivateKey()
	password := []byte("strong-password-for-wallet-backup")
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("failed to generate nonce: %v", err)
	}

	ciphertext, err := encryptPrivateKey(priv, password, nonce)
	if err != nil {
		t.Fatalf("failed to encrypt private key: %v", err)
	}

	if bytes.Equal(ciphertext, priv) {
		t.Fatal("ciphertext must differ from plaintext private key")
	}

	plaintext, err := decryptPrivateKey(ciphertext, password, nonce)
	if err != nil {
		t.Fatalf("failed to decrypt private key: %v", err)
	}

	if !bytes.Equal(plaintext, priv) {
		t.Fatal("decrypted private key does not match original")
	}

	if _, err := decryptPrivateKey(ciphertext, []byte("wrong-password"), nonce); err == nil {
		t.Fatal("decryption should fail with wrong password")
	}

	badNonce := make([]byte, len(nonce))
	copy(badNonce, nonce)
	badNonce[0] ^= 0xAA
	if _, err := decryptPrivateKey(ciphertext, password, badNonce); err == nil {
		t.Fatal("decryption should fail with wrong nonce")
	}

	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[len(tampered)-1] ^= 0x01
	if _, err := decryptPrivateKey(tampered, password, nonce); err == nil {
		t.Fatal("decryption should fail for tampered ciphertext")
	}

	if _, err := encryptPrivateKey(priv, []byte{}, nonce); err == nil {
		t.Fatal("encryption should reject empty password")
	}

	if _, err := encryptPrivateKey(priv, password, nonce[:8]); err == nil {
		t.Fatal("encryption should reject invalid nonce size")
	}
}

// TestBackup_RecoveryValidation tests full recovery verification end-to-end.
func TestBackup_RecoveryValidation(t *testing.T) {
	original, err := NewWallet()
	if err != nil {
		t.Fatalf("failed to create original wallet: %v", err)
	}

	entropy := make([]byte, 16)
	if _, err := rand.Read(entropy); err != nil {
		t.Fatalf("failed to generate entropy: %v", err)
	}
	mnemonic, err := generateMnemonicFromEntropy(entropy)
	if err != nil {
		t.Fatalf("failed to generate mnemonic: %v", err)
	}
	if _, err := mnemonicToEd25519Seed(mnemonic); err != nil {
		t.Fatalf("failed to derive seed from mnemonic: %v", err)
	}

	backupPriv := original.ExportPrivateKey()
	password := []byte("recovery-validation-password")
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("failed to generate nonce: %v", err)
	}

	enc, err := encryptPrivateKey(backupPriv, password, nonce)
	if err != nil {
		t.Fatalf("failed to encrypt backup private key: %v", err)
	}

	dec, err := decryptPrivateKey(enc, password, nonce)
	if err != nil {
		t.Fatalf("failed to decrypt backup private key: %v", err)
	}

	recovered, err := FromPrivateKeyBytes(dec)
	if err != nil {
		t.Fatalf("failed to recover wallet from decrypted backup: %v", err)
	}

	if original.GetAddressHex() != recovered.GetAddressHex() {
		t.Fatalf("recovery failed: address mismatch original=%s recovered=%s", original.GetAddressHex(), recovered.GetAddressHex())
	}

	testPayloads := [][]byte{
		[]byte("recovery-check-1"),
		[]byte("recovery-check-2-different-bytes"),
		[]byte{},
	}

	for i, payload := range testPayloads {
		sig := original.Sign(payload)
		if !recovered.Verify(payload, sig) {
			t.Fatalf("recovered wallet failed to verify original signature for payload index %d", i)
		}

		recoveredSig := recovered.Sign(payload)
		if !original.Verify(payload, recoveredSig) {
			t.Fatalf("original wallet failed to verify recovered signature for payload index %d", i)
		}
	}

	otherWallet, err := NewWallet()
	if err != nil {
		t.Fatalf("failed to create other wallet: %v", err)
	}
	probe := []byte("cross-wallet-boundary-check")
	probeSig := original.Sign(probe)
	if otherWallet.Verify(probe, probeSig) {
		t.Fatal("different wallet must not verify recovered/original signature")
	}
}
