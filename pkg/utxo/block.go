// Package utxo implements UTXO-based transaction system for AIB blockchain.
// Team Alpha - Block Module
package utxo

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"
)

// BlockHeader represents the header of a block.
type BlockHeader struct {
	Version       uint32
	PrevBlockHash [32]byte
	MerkleRoot    [32]byte
	Timestamp     uint64
	Height        uint64
	Proposer      [32]byte // Wallet address of the block proposer
	ProposerKey   [32]byte // Public key of the proposer (for signature verification)
	Signature     []byte   // Signature of the block proposer

	// VRF (Verifiable Random Function) for proposer selection
	VRFProof []byte   // VRF proof that this proposer was legitimately selected
	VRFSeed  [32]byte // VRF output used as random seed for next block

	// Validator state (for verification)
	ValidatorStateRoot [32]byte // Merkle root of validator states at this block

	// PoAIW (Proof of AI Work) fields - only used when Version >= 2
	// These fields are optional and can be nil/empty for Version 1 blocks
	InferencePoW []byte // PoW proof for inference work (optional, for Version >= 2)
	ModelID      string // ID of the AI model used for inference (optional, for Version >= 2)
	EnergyClaim  uint64 // Energy/token claim for inference work (optional, for Version >= 2)

	// PoW fields (Version 3, consensus v3 era) — Bitcoin-style
	Nonce uint64 // PoW nonce
	Bits  uint32 // compact difficulty target
}

// Block represents a block in the blockchain.
type Block struct {
	Header       BlockHeader
	Transactions []*Transaction
	Hash         [32]byte // Cached block hash (with signature)
	SignedHash   [32]byte // Hash that was signed (without signature)
}

// NewBlock creates a new block with the given transactions and previous block hash.
func NewBlock(transactions []*Transaction, prevBlockHash [32]byte, height uint64, proposer [32]byte) *Block {
	block := &Block{
		Header: BlockHeader{
			Version:       1,
			PrevBlockHash: prevBlockHash,
			Timestamp:     uint64(time.Now().Unix()),
			Height:        height,
			Proposer:      proposer,
		},
		Transactions: transactions,
	}

	// Calculate Merkle root
	block.Header.MerkleRoot = block.CalculateMerkleRoot()

	// Calculate block hash
	block.Hash = block.CalculateHash()

	return block
}

// CalculateMerkleRoot computes the Merkle root of all transactions in the block.
func (b *Block) CalculateMerkleRoot() [32]byte {
	if len(b.Transactions) == 0 {
		return [32]byte{}
	}

	// Create a list of transaction hashes
	hashes := make([][32]byte, len(b.Transactions))
	for i, tx := range b.Transactions {
		hashes[i] = tx.Hash()
	}

	// Build Merkle tree
	for len(hashes) > 1 {
		// If odd number of hashes, duplicate the last one
		if len(hashes)%2 != 0 {
			hashes = append(hashes, hashes[len(hashes)-1])
		}

		// Create next level
		nextLevel := make([][32]byte, len(hashes)/2)
		for i := 0; i < len(hashes); i += 2 {
			// Concatenate and hash
			concat := append(hashes[i][:], hashes[i+1][:]...)
			hash := sha256.Sum256(concat)
			nextLevel[i/2] = sha256.Sum256(hash[:])
		}

		hashes = nextLevel
	}

	return hashes[0]
}

// CalculateHash computes the block hash (double SHA256 of the header).
func (b *Block) CalculateHash() [32]byte {
	headerData := b.Header.Serialize()
	hash1 := sha256.Sum256(headerData)
	hash2 := sha256.Sum256(hash1[:])
	return hash2
}

// Serialize converts the block header to bytes.
func (h *BlockHeader) Serialize() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.LittleEndian, h.Version)
	buf.Write(h.PrevBlockHash[:])
	buf.Write(h.MerkleRoot[:])
	binary.Write(&buf, binary.LittleEndian, h.Timestamp)
	binary.Write(&buf, binary.LittleEndian, h.Height)
	buf.Write(h.Proposer[:])
	buf.Write(h.ProposerKey[:])
	binary.Write(&buf, binary.LittleEndian, uint32(len(h.Signature)))
	buf.Write(h.Signature)

	// Serialize VRF fields
	binary.Write(&buf, binary.LittleEndian, uint32(len(h.VRFProof)))
	buf.Write(h.VRFProof)
	buf.Write(h.VRFSeed[:])

	// Serialize ValidatorStateRoot
	buf.Write(h.ValidatorStateRoot[:])

	// Serialize PoAIW fields (Version >= 2)
	if h.Version >= 2 {
		binary.Write(&buf, binary.LittleEndian, uint32(len(h.InferencePoW)))
		buf.Write(h.InferencePoW)

		modelIDLen := uint32(len(h.ModelID))
		binary.Write(&buf, binary.LittleEndian, modelIDLen)
		buf.Write([]byte(h.ModelID))

		binary.Write(&buf, binary.LittleEndian, h.EnergyClaim)
	}

	// PoW fields (Version 3) — appended at END, old formats unaffected
	if h.Version >= 3 {
		binary.Write(&buf, binary.LittleEndian, h.Nonce)
		binary.Write(&buf, binary.LittleEndian, h.Bits)
	}

	return buf.Bytes()
}

// DeserializeBlockHeader parses a block header from bytes.
func DeserializeBlockHeader(data []byte) (*BlockHeader, error) {
	buf := bytes.NewReader(data)
	header := &BlockHeader{}

	if err := binary.Read(buf, binary.LittleEndian, &header.Version); err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}

	if _, err := buf.Read(header.PrevBlockHash[:]); err != nil {
		return nil, fmt.Errorf("failed to read prev block hash: %w", err)
	}

	if _, err := buf.Read(header.MerkleRoot[:]); err != nil {
		return nil, fmt.Errorf("failed to read merkle root: %w", err)
	}

	if err := binary.Read(buf, binary.LittleEndian, &header.Timestamp); err != nil {
		return nil, fmt.Errorf("failed to read timestamp: %w", err)
	}

	if err := binary.Read(buf, binary.LittleEndian, &header.Height); err != nil {
		return nil, fmt.Errorf("failed to read height: %w", err)
	}

	if _, err := buf.Read(header.Proposer[:]); err != nil {
		return nil, err
	}
	if _, err := buf.Read(header.ProposerKey[:]); err != nil {
		return nil, fmt.Errorf("failed to read proposer: %w", err)
	}

	var sigLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &sigLen); err != nil {
		return nil, fmt.Errorf("failed to read signature length: %w", err)
	}

	header.Signature = make([]byte, sigLen)
	if _, err := buf.Read(header.Signature); err != nil {
		return nil, fmt.Errorf("failed to read signature: %w", err)
	}

	// Read VRF fields
	var vrfProofLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &vrfProofLen); err != nil {
		return nil, fmt.Errorf("failed to read VRF proof length: %w", err)
	}

	header.VRFProof = make([]byte, vrfProofLen)
	if _, err := buf.Read(header.VRFProof); err != nil {
		return nil, fmt.Errorf("failed to read VRF proof: %w", err)
	}

	if _, err := buf.Read(header.VRFSeed[:]); err != nil {
		return nil, fmt.Errorf("failed to read VRF seed: %w", err)
	}

	// Read ValidatorStateRoot
	if _, err := buf.Read(header.ValidatorStateRoot[:]); err != nil {
		return nil, fmt.Errorf("failed to read validator state root: %w", err)
	}

	// Read PoAIW fields (Version >= 2)
	if header.Version >= 2 {
		var inferencePoWLen uint32
		if err := binary.Read(buf, binary.LittleEndian, &inferencePoWLen); err != nil {
			return nil, fmt.Errorf("failed to read inference PoW length: %w", err)
		}

		header.InferencePoW = make([]byte, inferencePoWLen)
		if _, err := buf.Read(header.InferencePoW); err != nil {
			return nil, fmt.Errorf("failed to read inference PoW: %w", err)
		}

		var modelIDLen uint32
		if err := binary.Read(buf, binary.LittleEndian, &modelIDLen); err != nil {
			return nil, fmt.Errorf("failed to read model ID length: %w", err)
		}

		modelIDBytes := make([]byte, modelIDLen)
		if _, err := buf.Read(modelIDBytes); err != nil {
			return nil, fmt.Errorf("failed to read model ID: %w", err)
		}
		header.ModelID = string(modelIDBytes)

		if err := binary.Read(buf, binary.LittleEndian, &header.EnergyClaim); err != nil {
			return nil, fmt.Errorf("failed to read energy claim: %w", err)
		}
	}

	// PoW fields (Version 3)
	if header.Version >= 3 {
		if err := binary.Read(buf, binary.LittleEndian, &header.Nonce); err != nil {
			return nil, fmt.Errorf("failed to read nonce: %w", err)
		}
		if err := binary.Read(buf, binary.LittleEndian, &header.Bits); err != nil {
			return nil, fmt.Errorf("failed to read bits: %w", err)
		}
	}

	return header, nil
}

// SignBlock signs the block with the proposer's private key.
func (b *Block) SignBlock(privKey ed25519.PrivateKey) error {
	// Embed the public key so verification doesn't depend on out-of-band data
	pub, ok := privKey.Public().(ed25519.PublicKey)
	if ok && len(pub) == 32 {
		copy(b.Header.ProposerKey[:], pub)
	}
	// Store the unsigned hash (the hash that was signed)
	b.SignedHash = b.CalculateHash()
	// Sign the unsigned hash
	signature := ed25519.Sign(privKey, b.SignedHash[:])
	b.Header.Signature = signature
	// Recalculate hash (now includes signature in header)
	b.Hash = b.CalculateHash()
	return nil
}

// VerifyBlockSignature verifies the block signature.
func (b *Block) VerifyBlockSignature() bool {
	if len(b.Header.Signature) == 0 {
		return false
	}

	// To verify, we need to reconstruct the hash that was signed
	// (the hash without the signature in the header).
	// We do this by temporarily removing the signature, hashing, then restoring.
	savedSig := b.Header.Signature
	b.Header.Signature = nil
	unsignedHash := b.CalculateHash()
	b.Header.Signature = savedSig

	return ed25519.Verify(ed25519.PublicKey(b.Header.ProposerKey[:]), unsignedHash[:], b.Header.Signature)
}

// ValidateBlock performs basic block validation.
func (b *Block) ValidateBlock() error {
	// Check block hash
	expectedHash := b.CalculateHash()
	if b.Hash != expectedHash {
		return fmt.Errorf("block hash mismatch")
	}

	// Verify Merkle root
	expectedMerkleRoot := b.CalculateMerkleRoot()
	if b.Header.MerkleRoot != expectedMerkleRoot {
		return fmt.Errorf("merkle root mismatch")
	}

	// Verify block signature
	if !b.VerifyBlockSignature() {
		return fmt.Errorf("invalid block signature")
	}

	// Validate transactions
	for _, tx := range b.Transactions {
		if !tx.VerifyAllInputs() {
			return fmt.Errorf("transaction validation failed")
		}
	}

	return nil
}

// IsGenesis checks if this is the genesis block.
func (b *Block) IsGenesis() bool {
	return b.Header.Height == 0 && b.Header.PrevBlockHash == [32]byte{}
}

// GetTransactionCount returns the number of transactions in the block.
func (b *Block) GetTransactionCount() int {
	return len(b.Transactions)
}

// GetTransactionByHash finds a transaction by its hash.
func (b *Block) GetTransactionByHash(hash [32]byte) *Transaction {
	for _, tx := range b.Transactions {
		if tx.Hash() == hash {
			return tx
		}
	}
	return nil
}

// CreateGenesisBlock creates the genesis block.
func CreateGenesisBlock(coinbaseTx *Transaction, proposer [32]byte) *Block {
	genesis := NewBlock([]*Transaction{coinbaseTx}, [32]byte{}, 0, proposer)
	genesis.Header.Timestamp = 1704067200 // Fixed timestamp for reproducibility
	genesis.Hash = genesis.CalculateHash()
	return genesis
}

// SerializeBlock converts the block to bytes.
func (b *Block) SerializeBlock() []byte {
	var buf bytes.Buffer

	// Serialize header
	headerData := b.Header.Serialize()
	binary.Write(&buf, binary.LittleEndian, uint32(len(headerData)))
	buf.Write(headerData)

	// Transaction count
	binary.Write(&buf, binary.LittleEndian, uint32(len(b.Transactions)))

	// Transactions
	for _, tx := range b.Transactions {
		txData := tx.Serialize()
		binary.Write(&buf, binary.LittleEndian, uint32(len(txData)))
		buf.Write(txData)
	}

	return buf.Bytes()
}

// DeserializeBlock parses a block from bytes.
func DeserializeBlock(data []byte) (*Block, error) {
	buf := bytes.NewReader(data)
	block := &Block{}

	// Header length
	var headerLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &headerLen); err != nil {
		return nil, fmt.Errorf("failed to read header length: %w", err)
	}

	// Header
	headerData := make([]byte, headerLen)
	if _, err := buf.Read(headerData); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	var err error
	header, err := DeserializeBlockHeader(headerData)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize header: %w", err)
	}
	block.Header = *header

	// Transaction count
	var txCount uint32
	if err := binary.Read(buf, binary.LittleEndian, &txCount); err != nil {
		return nil, fmt.Errorf("failed to read tx count: %w", err)
	}

	// Transactions
	block.Transactions = make([]*Transaction, txCount)
	for i := uint32(0); i < txCount; i++ {
		var txLen uint32
		if err := binary.Read(buf, binary.LittleEndian, &txLen); err != nil {
			return nil, fmt.Errorf("failed to read tx length: %w", err)
		}

		txData := make([]byte, txLen)
		if _, err := buf.Read(txData); err != nil {
			return nil, fmt.Errorf("failed to read tx data: %w", err)
		}

		tx, err := DeserializeTransaction(txData)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize tx: %w", err)
		}
		block.Transactions[i] = tx
	}

	// Calculate hash
	block.Hash = block.CalculateHash()

	return block, nil
}

// GetCoinbaseTransaction returns the coinbase transaction (first transaction).
func (b *Block) GetCoinbaseTransaction() *Transaction {
	if len(b.Transactions) > 0 {
		return b.Transactions[0]
	}
	return nil
}

// GetBlockReward returns the total block reward (coinbase output value).
func (b *Block) GetBlockReward() uint64 {
	coinbase := b.GetCoinbaseTransaction()
	if coinbase == nil {
		return 0
	}
	return coinbase.TotalOutputValue()
}

// GetTotalTransactionFees returns the sum of all transaction fees in the block.
// Requires UTXO provider to look up input values.
func (b *Block) GetTotalTransactionFees(utxoProvider UTXOProvider) (uint64, error) {
	var totalFees uint64

	for i, tx := range b.Transactions {
		// Skip coinbase transaction (first transaction)
		if i == 0 {
			continue
		}

		fee, err := tx.GetFee(utxoProvider)
		if err != nil {
			return 0, fmt.Errorf("failed to calculate fee for tx %d: %w", i, err)
		}
		totalFees += fee
	}

	return totalFees, nil
}

// CalculateTotalReward calculates the total reward for the block proposer.
// Total Reward = Block Subsidy + Transaction Fees
func (b *Block) CalculateTotalReward(utxoProvider UTXOProvider, blockSubsidy uint64) (uint64, error) {
	fees, err := b.GetTotalTransactionFees(utxoProvider)
	if err != nil {
		return 0, err
	}

	return blockSubsidy + fees, nil
}

// GetNonCoinbaseTransactions returns all transactions except the coinbase.
func (b *Block) GetNonCoinbaseTransactions() []*Transaction {
	if len(b.Transactions) <= 1 {
		return nil
	}
	return b.Transactions[1:]
}

// ValidateCoinbaseReward validates that the coinbase output includes block subsidy + all tx fees.
func (b *Block) ValidateCoinbaseReward(utxoProvider UTXOProvider, expectedSubsidy uint64) error {
	coinbase := b.GetCoinbaseTransaction()
	if coinbase == nil {
		return fmt.Errorf("no coinbase transaction")
	}

	if !coinbase.IsCoinbase() {
		return fmt.Errorf("first transaction is not coinbase")
	}

	// Calculate total fees from transactions
	totalFees, err := b.GetTotalTransactionFees(utxoProvider)
	if err != nil {
		return fmt.Errorf("failed to calculate fees: %w", err)
	}

	// Expected reward = subsidy + fees
	expectedReward := expectedSubsidy + totalFees
	actualReward := coinbase.TotalOutputValue()

	if actualReward < expectedReward {
		return fmt.Errorf("coinbase reward %d < expected %d (subsidy %d + fees %d)",
			actualReward, expectedReward, expectedSubsidy, totalFees)
	}

	return nil
}

// ValidateBlockChain validates this block against its parent.
func (b *Block) ValidateBlockChain(parentBlock *Block) error {
	if parentBlock == nil {
		if !b.IsGenesis() {
			return fmt.Errorf("non-genesis block has no parent")
		}
		return nil
	}

	// Check height continuity
	if b.Header.Height != parentBlock.Header.Height+1 {
		return fmt.Errorf("block height mismatch: expected %d, got %d",
			parentBlock.Header.Height+1, b.Header.Height)
	}

	// Check previous block hash
	if b.Header.PrevBlockHash != parentBlock.Hash {
		return fmt.Errorf("previous block hash mismatch")
	}

	// Check timestamp
	if b.Header.Timestamp <= parentBlock.Header.Timestamp {
		return fmt.Errorf("block timestamp must be greater than parent")
	}

	// validate block timestamp
	blockTime := time.Unix(int64(b.Header.Timestamp), 0)
	parentTime := time.Unix(int64(parentBlock.Header.Timestamp), 0)
	timeDiff := blockTime.Sub(parentTime)

	if timeDiff < MinBlockTime {
		return fmt.Errorf("block time %v below minimum %v", timeDiff, MinBlockTime)
	}

	// Drift vs wall clock applies only near the chain tip (parent recent):
	// while catching up, historical block timestamps are legitimately old and
	// must be accepted. Mirrors ChainState.validateBlockTimestamp. Comparing
	// the parent-to-child interval, or applying an unconditional wall-clock
	// bound, would deadlock catch-up after any outage longer than the drift
	// bound (the gap is already history we cannot fix).
	now := time.Now()
	parentIsRecent := now.Sub(parentTime) < 2*MaxBlockTimeDrift
	if parentIsRecent {
		if d := blockTime.Sub(now); d > MaxBlockTimeDrift || d < -MaxBlockTimeDrift {
			return fmt.Errorf("block time %v exceeds maximum drift %v from now", d.Round(time.Second), MaxBlockTimeDrift)
		}
	}

	return nil
}

// ============================================================================
// Security Validation
// ============================================================================

// CoinbaseMaturityBlocks is the number of blocks before coinbase output can be spent.
const CoinbaseMaturityBlocks = 100

// MaxBlockSize is the maximum block size in bytes.
const MaxBlockSize = 1_000_000 // 1 MB

// MinTransactionFee is the minimum fee per transaction in satoshi.
const MinTransactionFee = uint64(100) // 100 satoshi

// ValidateBlockSecurity performs comprehensive security validation.
func (b *Block) ValidateBlockSecurity(utxoProvider UTXOProvider, currentHeight uint64) []error {
	var errs []error

	// 1. Validate only one coinbase transaction
	coinbaseCount := 0
	for _, tx := range b.Transactions {
		if tx.IsCoinbase() {
			coinbaseCount++
		}
	}
	if coinbaseCount > 1 {
		errs = append(errs, fmt.Errorf("block contains %d coinbase transactions, expected at most 1", coinbaseCount))
	}
	if len(b.Transactions) > 0 && !b.Transactions[0].IsCoinbase() {
		errs = append(errs, fmt.Errorf("first transaction must be coinbase"))
	}

	// 2. Validate minimum transaction fees
	for i := 1; i < len(b.Transactions); i++ {
		tx := b.Transactions[i]
		fee, err := tx.GetFee(utxoProvider)
		if err != nil {
			errs = append(errs, fmt.Errorf("tx %d: cannot calculate fee: %v", i, err))
			continue
		}
		if fee < MinTransactionFee {
			errs = append(errs, fmt.Errorf("tx %d: fee %d below minimum %d", i, fee, MinTransactionFee))
		}
	}

	// 3. Validate Merkle root
	computedRoot := b.CalculateMerkleRoot()
	if computedRoot != b.Header.MerkleRoot {
		errs = append(errs, fmt.Errorf("merkle root mismatch: computed %x, header %x", computedRoot, b.Header.MerkleRoot))
	}

	// 4. Validate all transaction signatures
	for i := 1; i < len(b.Transactions); i++ {
		tx := b.Transactions[i]
		if !tx.VerifyAllInputs() {
			errs = append(errs, fmt.Errorf("tx %d: invalid signature", i))
		}
	}

	// 5. Validate no duplicate inputs (double spend within block)
	inputSet := make(map[[36]byte]bool)
	for i := 1; i < len(b.Transactions); i++ {
		for _, input := range b.Transactions[i].Inputs {
			var key [36]byte
			copy(key[:32], input.TxHash[:])
			key[32] = byte(input.Index)
			key[33] = byte(input.Index >> 8)
			key[34] = byte(input.Index >> 16)
			key[35] = byte(input.Index >> 24)

			if inputSet[key] {
				errs = append(errs, fmt.Errorf("tx %d: double spend of UTXO %x:%d within block", i, input.TxHash, input.Index))
			}
			inputSet[key] = true
		}
	}

	return errs
}

// ValidateTransactionMinFee validates that a transaction meets the minimum fee.
func ValidateTransactionMinFee(tx *Transaction, utxoProvider UTXOProvider) error {
	if tx.IsCoinbase() {
		return nil
	}

	fee, err := tx.GetFee(utxoProvider)
	if err != nil {
		return fmt.Errorf("cannot calculate fee: %w", err)
	}

	if fee < MinTransactionFee {
		return fmt.Errorf("fee %d below minimum %d", fee, MinTransactionFee)
	}

	return nil
}

// IsCoinbaseSpendable checks if a coinbase UTXO is old enough to be spent.
func IsCoinbaseSpendable(coinbaseHeight uint64, currentHeight uint64) bool {
	if coinbaseHeight == 0 {
		return true // Genesis coinbase
	}
	return currentHeight-coinbaseHeight >= CoinbaseMaturityBlocks
}
