// Package api provides REST API handlers for AIB 2.0 blockchain queries.
// This file implements handlers for UTXO, validators, mempool, staking, and proposals.
package api

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/aib-protocol/aib/pkg/utxo"
)

// ============================================================================
// UTXO queryhandler
// ============================================================================

// UTXOByAddressResponse returnsaddress UTXO list
type UTXOByAddressResponse struct {
	Address  string     `json:"address"`
	UTXOs    []UTXOItem `json:"utxos"`
	Total    uint64     `json:"total"`
	CanSpend bool       `json:"can_spend"`
}

// UTXOItem represents a single UTXO
type UTXOItem struct {
	TxID    string `json:"tx_id"`
	Index   uint32 `json:"index"`
	Value   uint64 `json:"value"`
	Script  string `json:"script,omitempty"`
	Address string `json:"address"`
}

// handleUTXOByAddress handle GET /v1/utxo/{address}
func (s *Server) handleUTXOByAddress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// Extract the address from the URL path
	// Handle the /v1/utxo/{address} format
	urlPath := r.URL.Path
	// Strip the /v1/utxo/ prefix
	address := ""
	if len(urlPath) > len("/v1/utxo/") {
		address = urlPath[len("/v1/utxo/"):]
	}

	if address == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Address is required", "")
		return
	}

	// Validate the address format
	var addrBytes [32]byte
	decoded, err := hex.DecodeString(address)
	if err != nil || len(decoded) != 32 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid address format", "Address must be a 32-byte hex string")
		return
	}
	copy(addrBytes[:], decoded)

	// Get the address's UTXOs from UTXO storage
	utxos, balance := s.getUTXOsByAddress(r.Context(), addrBytes)

	response := UTXOByAddressResponse{
		Address:  address,
		UTXOs:    utxos,
		Total:    balance,
		CanSpend: len(utxos) > 0,
	}

	writeSuccess(w, response)
}

// getUTXOsByAddress retrieves UTXOs for an address from storage
func (s *Server) getUTXOsByAddress(ctx context.Context, address [32]byte) ([]UTXOItem, uint64) {
	var utxos []UTXOItem
	var total uint64

	// If UTXO storage is available, query it
	if s.utxoStore != nil {
		// Use the UTXO storage query method
		allUTXOs := s.utxoStore.GetAllUTXOs(address)
		for _, u := range allUTXOs {
			utxos = append(utxos, UTXOItem{
				TxID:    hex.EncodeToString(u.TxHash[:]),
				Index:   u.Index,
				Value:   u.Value,
				Script:  hex.EncodeToString(u.Script),
				Address: hex.EncodeToString(u.Address[:]),
			})
			total += u.Value
		}
	}

	// Sort by TxID
	sort.Slice(utxos, func(i, j int) bool {
		return utxos[i].TxID < utxos[j].TxID
	})

	return utxos, total
}

// ============================================================================
// Validator list handlers
// ============================================================================

// ValidatorsResponse returnsvalidatorlist
type ValidatorsResponse struct {
	Validators []ValidatorInfo `json:"validators"`
	Total      int             `json:"total"`
	Active     int             `json:"active"`
	TotalStake uint64          `json:"total_stake"`
}

// ValidatorInfo represents a single validator's information
type ValidatorInfo struct {
	Address      string `json:"address"`
	Stake        uint64 `json:"stake"`
	PublicKey    string `json:"public_key,omitempty"`
	JoinedAt     uint64 `json:"joined_at"`
	LastProposed uint64 `json:"last_proposed"`
	TotalRewards uint64 `json:"total_rewards"`
	IsActive     bool   `json:"is_active"`
	Commission   uint8  `json:"commission,omitempty"`
}

// handleValidators handle GET /v1/validators
func (s *Server) handleValidators(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// Get the validator list from consensus state
	validators, totalStake := s.getValidators(r.Context())

	// Count active validators
	activeCount := 0
	for _, v := range validators {
		if v.IsActive {
			activeCount++
		}
	}

	response := ValidatorsResponse{
		Validators: validators,
		Total:      len(validators),
		Active:     activeCount,
		TotalStake: totalStake,
	}

	writeSuccess(w, response)
}

// getValidators retrieves validator information from consensus state
func (s *Server) getValidators(ctx context.Context) ([]ValidatorInfo, uint64) {
	var validators []ValidatorInfo
	var totalStake uint64

	if s.consensusState != nil {
		// Get all validators
		allValidators := s.consensusState.GetActiveValidators()
		for _, v := range allValidators {
			validatorInfo := ValidatorInfo{
				Address:      hex.EncodeToString(v.Address[:]),
				Stake:        v.Stake,
				PublicKey:    hex.EncodeToString(v.PublicKey),
				JoinedAt:     v.JoinedAt,
				LastProposed: v.LastProposed,
				TotalRewards: v.TotalRewards,
				IsActive:     v.Stake >= s.consensusState.GetConfig().MinStake,
				Commission:   v.Commission,
			}
			validators = append(validators, validatorInfo)
			totalStake += v.Stake
		}
	}

	// Sort by stake amount
	sort.Slice(validators, func(i, j int) bool {
		return validators[i].Stake > validators[j].Stake
	})

	return validators, totalStake
}

// ============================================================================
// Mempool query handlers
// ============================================================================

// MempoolResponse returns the mempool state
type MempoolResponse struct {
	Transactions []MempoolTxInfo `json:"transactions"`
	Count        int             `json:"count"`
	TotalFees    uint64          `json:"total_fees"`
	TotalSize    int             `json:"total_size"`
	MinFeeRate   float64         `json:"min_fee_rate"`
	MaxFeeRate   float64         `json:"max_fee_rate"`
	AvgFeeRate   float64         `json:"avg_fee_rate"`
}

// MempoolTxInfo represents a transaction in the mempool
type MempoolTxInfo struct {
	TxID       string    `json:"tx_id"`
	Fee        uint64    `json:"fee"`
	FeeRate    float64   `json:"fee_rate"`
	Size       int       `json:"size"`
	AddedAt    time.Time `json:"added_at"`
	Inputs     int       `json:"inputs"`
	Outputs    int       `json:"outputs"`
	IsCoinbase bool      `json:"is_coinbase"`
}

// handleMempool handle GET /v1/mempool
func (s *Server) handleMempool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// queryparameter
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
		if limit > 1000 {
			limit = 1000 // maximum limit
		}
	}

	// Get transactions from the mempool
	txs := s.getMempoolTransactions(r.Context(), limit)

	// Compute statistics
	var totalFees uint64
	var totalSize int
	var totalFeeRate float64
	minFeeRate := float64(-1)
	maxFeeRate := float64(0)

	for _, tx := range txs {
		totalFees += tx.Fee
		totalSize += tx.Size
		totalFeeRate += tx.FeeRate
		if minFeeRate < 0 || tx.FeeRate < minFeeRate {
			minFeeRate = tx.FeeRate
		}
		if tx.FeeRate > maxFeeRate {
			maxFeeRate = tx.FeeRate
		}
	}

	avgFeeRate := float64(0)
	if len(txs) > 0 {
		avgFeeRate = totalFeeRate / float64(len(txs))
	}

	response := MempoolResponse{
		Transactions: txs,
		Count:        len(txs),
		TotalFees:    totalFees,
		TotalSize:    totalSize,
		MinFeeRate:   minFeeRate,
		MaxFeeRate:   maxFeeRate,
		AvgFeeRate:   avgFeeRate,
	}

	writeSuccess(w, response)
}

// getMempoolTransactions retrieves transactions from the mempool
func (s *Server) getMempoolTransactions(ctx context.Context, limit int) []MempoolTxInfo {
	var txs []MempoolTxInfo

	if s.mempool == nil {
		return txs
	}

	// Get all transactions in the mempool
	entries := s.mempool.GetAllEntries()

	for _, entry := range entries {
		if len(txs) >= limit {
			break
		}

		txHash := entry.Tx.Hash()
		txInfo := MempoolTxInfo{
			TxID:       hex.EncodeToString(txHash[:]),
			Fee:        entry.Fee,
			FeeRate:    entry.FeeRate,
			Size:       entry.Tx.SerializeSize(),
			AddedAt:    entry.AddedAt,
			Inputs:     len(entry.Tx.Inputs),
			Outputs:    len(entry.Tx.Outputs),
			IsCoinbase: entry.Tx.IsCoinbase(),
		}
		txs = append(txs, txInfo)
	}

	// Sort by fee rate (highest first)
	sort.Slice(txs, func(i, j int) bool {
		return txs[i].FeeRate > txs[j].FeeRate
	})

	return txs
}

// ============================================================================
// Staking status handlers
// ============================================================================

// StakingResponse returnsglobalstake / stakingstatus
type StakingResponse struct {
	TotalStaked      uint64       `json:"total_staked"`
	TotalValidators  int          `json:"total_validators"`
	ActiveValidators int          `json:"active_validators"`
	CurrentEpoch     uint64       `json:"current_epoch"`
	MinStake         uint64       `json:"min_stake"`
	StakeLockPeriod  uint64       `json:"stake_lock_period"`
	APY              float64      `json:"apy"`
	Stakers          []StakerInfo `json:"stakers,omitempty"`
}

// StakerInfo represents a single staker's information
type StakerInfo struct {
	Address string  `json:"address"`
	Stake   uint64  `json:"stake"`
	Rewards uint64  `json:"rewards"`
	Share   float64 `json:"share"` // share of total stake
}

// handleStaking handle GET /v1/staking
func (s *Server) handleStaking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// queryparameter
	includeStakers := r.URL.Query().Get("include_stakers") == "true"

	stakingInfo := s.getStakingInfo(r.Context(), includeStakers)

	writeSuccess(w, stakingInfo)
}

// getStakingInfo getstake / stakinginfo
func (s *Server) getStakingInfo(ctx context.Context, includeStakers bool) *StakingResponse {
	if s.consensusState == nil {
		return &StakingResponse{}
	}

	config := s.consensusState.GetConfig()
	totalStake := s.consensusState.GetTotalStake()
	validators := s.consensusState.GetActiveValidators()
	currentEpoch := s.consensusState.GetCurrentEpoch()

	response := &StakingResponse{
		TotalStaked:      totalStake,
		TotalValidators:  s.consensusState.GetValidatorCount(),
		ActiveValidators: len(validators),
		CurrentEpoch:     currentEpoch,
		MinStake:         config.MinStake,
		StakeLockPeriod:  config.StakeLockPeriod,
		APY:              calculateAPY(totalStake, config.BlockReward),
	}

	// If the request includes staker details
	if includeStakers {
		var stakers []StakerInfo
		for _, v := range validators {
			share := float64(0)
			if totalStake > 0 {
				share = float64(v.Stake) / float64(totalStake) * 100
			}
			stakers = append(stakers, StakerInfo{
				Address: hex.EncodeToString(v.Address[:]),
				Stake:   v.Stake,
				Rewards: v.TotalRewards,
				Share:   share,
			})
		}
		// Sort by stake amount
		sort.Slice(stakers, func(i, j int) bool {
			return stakers[i].Stake > stakers[j].Stake
		})
		response.Stakers = stakers
	}

	return response
}

// calculateAPY computes the annual staking yield
func calculateAPY(totalStake, blockReward uint64) float64 {
	if totalStake == 0 {
		return 0
	}
	// Simplified APY calculation
	// Assume 365*24*3600/30 = 1,051,200 blocks are produced per year
	blocksPerYear := float64(365*24*60*60) / 30
	annualReward := float64(blockReward) * blocksPerYear
	apy := (annualReward / float64(totalStake)) * 100
	return apy
}

// ============================================================================
// Governance proposal handlers
// ============================================================================

// ProposalsResponse returnsgovernanceproposallist
type ProposalsResponse struct {
	Proposals []ProposalInfo `json:"proposals"`
	Total     int            `json:"total"`
	Active    int            `json:"active"`
	Passed    int            `json:"passed"`
	Rejected  int            `json:"rejected"`
}

// ProposalInfo represents a governance proposal
type ProposalInfo struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	Proposer     string    `json:"proposer"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	VotesFor     uint64    `json:"votes_for"`
	VotesAgainst uint64    `json:"votes_against"`
	TotalVoted   uint64    `json:"total_voted"`
	Quorum       uint64    `json:"quorum"`
}

// handleProposals handle GET /v1/proposals
func (s *Server) handleProposals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// queryparameter
	status := r.URL.Query().Get("status") // active, passed, rejected, all

	proposals := s.getProposals(r.Context(), status)

	// Count by status
	active, passed, rejected := 0, 0, 0
	for _, p := range proposals {
		switch p.Status {
		case "active":
			active++
		case "passed":
			passed++
		case "rejected":
			rejected++
		}
	}

	response := ProposalsResponse{
		Proposals: proposals,
		Total:     len(proposals),
		Active:    active,
		Passed:    passed,
		Rejected:  rejected,
	}

	writeSuccess(w, response)
}

// getProposals getgovernanceproposal
func (s *Server) getProposals(ctx context.Context, statusFilter string) []ProposalInfo {
	var proposals []ProposalInfo

	// Note: the governance module may not be implemented yet
	// A sample implementation is provided here; if the project has a governance module, proposals should be fetched from it

	if s.governance != nil {
		// Get proposals from the governance module
		allProposals := s.governance.GetAllProposals()
		for _, p := range allProposals {
			if statusFilter != "" && statusFilter != "all" && p.Status != statusFilter {
				continue
			}

			proposalInfo := ProposalInfo{
				ID:           p.ID,
				Title:        p.Title,
				Description:  p.Description,
				Type:         p.Type,
				Status:       p.Status,
				Proposer:     hex.EncodeToString(p.Proposer[:]),
				CreatedAt:    p.CreatedAt,
				ExpiresAt:    p.ExpiresAt,
				VotesFor:     p.VotesFor,
				VotesAgainst: p.VotesAgainst,
				TotalVoted:   p.VotesFor + p.VotesAgainst,
				Quorum:       p.Quorum,
			}
			proposals = append(proposals, proposalInfo)
		}
	}

	// Sort by creation time
	sort.Slice(proposals, func(i, j int) bool {
		return proposals[i].CreatedAt.After(proposals[j].CreatedAt)
	})

	return proposals
}

// ============================================================================
// Helper interface definitions
// ============================================================================

// utxoStoreInterface is the UTXO storage interface
type utxoStoreInterface interface {
	GetAllUTXOs(addr [32]byte) []*utxo.UTXO
	GetTransactionIndex(txHash [32]byte) (uint64, error)
}

// mempoolInterface is the mempool interface
// Note: utxo.Mempool may not have a GetAllEntries method
// We need to adjust based on the actual implementation
type mempoolInterface interface {
	GetAllEntries() []*utxo.MempoolEntry
	GetTransaction(txHash [32]byte) *utxo.Transaction
}

// consensusConfigInterface consensusstatusinterface
type consensusConfigInterface interface {
	GetConfig() *utxo.PoSConfig
	GetActiveValidators() []*utxo.Validator
	GetTotalStake() uint64
	GetValidatorCount() int
	GetCurrentEpoch() uint64
}

// governanceInterface is the governance interface (if the project has one)
type governanceInterface interface {
	GetAllProposals() []Proposal
}

// Proposal represents a governance proposal
type Proposal struct {
	ID           string
	Title        string
	Description  string
	Type         string
	Status       string
	Proposer     [32]byte
	CreatedAt    time.Time
	ExpiresAt    time.Time
	VotesFor     uint64
	VotesAgainst uint64
	Quorum       uint64
}
