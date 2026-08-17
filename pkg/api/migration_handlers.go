package api

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aib-protocol/aib/internal/interfaces"
	"github.com/aib-protocol/aib/pkg/migration"
)

// ============================================================================
// Migration API Handlers
// ============================================================================

// handleMigrationSnapshot 处理 GET /api/migration/snapshot
// 查询 AIB1 快照信息
func (s *Server) handleMigrationSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	hub := s.GetMigrationHub()
	if hub == nil {
		// Return default response when no hub is configured
		writeSuccess(w, AIB1SnapshotResponse{
			SnapshotRoot:  "0000000000000000000000000000000000000000000000000000000000000000",
			SnapshotTime:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			ClaimDeadline: time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
			ClaimOpen:     true,
			TotalMigrated: 0,
		})
		return
	}

	status := hub.GetMigrationStatus()

	writeSuccess(w, AIB1SnapshotResponse{
		SnapshotRoot:  "0000000000000000000000000000000000000000000000000000000000000000", // Snapshot root not exposed via hub
		SnapshotTime:  status.MigrationWindowStart,                                        // Use migration window start as snapshot time
		ClaimDeadline: status.AIB1ClaimDeadline,
		ClaimOpen:     status.AIB1ClaimOpen,
		TotalMigrated: status.AIB1TotalMigrated,
	})
}

// handleMigrationRates 处理 GET /api/migration/rates
// 查询当前迁移汇率
func (s *Server) handleMigrationRates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	hub := s.GetMigrationHub()
	if hub == nil {
		// Return default response when no hub is configured
		writeSuccess(w, MigrationRatesResponse{
			Timestamp: time.Now().UTC(),
			AIB1Rate:  100, // 1:1 = 100%
			ChainRates: map[string]ChainRateInfo{
				"BTC": {Chain: "BTC", CurrentRate: 5, WindowOpen: false, WindowStart: time.Time{}, WindowEnd: time.Time{}},
				"ETH": {Chain: "ETH", CurrentRate: 4, WindowOpen: false, WindowStart: time.Time{}, WindowEnd: time.Time{}},
				"SOL": {Chain: "SOL", CurrentRate: 3, WindowOpen: false, WindowStart: time.Time{}, WindowEnd: time.Time{}},
			},
		})
		return
	}

	status := hub.GetMigrationStatus()

	chainRates := map[string]ChainRateInfo{
		"BTC": {
			Chain:       "BTC",
			CurrentRate: status.BTCCurrentRate,
			WindowOpen:  status.BTCWindowOpen,
			WindowStart: status.MigrationWindowStart,
			WindowEnd:   status.MigrationWindowEnd,
		},
		"ETH": {
			Chain:       "ETH",
			CurrentRate: status.ETHCurrentRate,
			WindowOpen:  status.ETHWindowOpen,
			WindowStart: status.MigrationWindowStart,
			WindowEnd:   status.MigrationWindowEnd,
		},
		"SOL": {
			Chain:       "SOL",
			CurrentRate: status.SOLCurrentRate,
			WindowOpen:  status.SOLWindowOpen,
			WindowStart: status.MigrationWindowStart,
			WindowEnd:   status.MigrationWindowEnd,
		},
	}

	writeSuccess(w, MigrationRatesResponse{
		Timestamp:  time.Now().UTC(),
		AIB1Rate:   100, // 1:1 固定汇率
		ChainRates: chainRates,
	})
}

// handleMigrationStatus 处理 GET /api/migration/status/{addr}
// 查询用户迁移状态
func (s *Server) handleMigrationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// Parse address from path
	addrStr := parsePathVar(r, "/api/migration/status/")
	if addrStr == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing address", "")
		return
	}

	// Convert address string to Address type
	addr, err := parseAddress(addrStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid address format", err.Error())
		return
	}

	hub := s.GetMigrationHub()
	if hub == nil {
		// Return empty response when no hub is configured
		writeSuccess(w, UserMigrationInfoAPI{
			AIB1SnapshotBalance: 0,
			AIB1Claimed:         false,
			LockedRewards:       LockedRewardsInfo{},
			TotalClaimable:      0,
			TotalLocked:         0,
		})
		return
	}

	info := hub.GetUserMigrationInfo(addr)

	// Convert to API response format
	response := convertUserMigrationInfo(info)

	writeSuccess(w, response)
}

// handleMigrationClaimable 处理 GET /api/migration/claimable/{addr}
// 查询可领取金额
func (s *Server) handleMigrationClaimable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// Parse address from path
	addrStr := parsePathVar(r, "/api/migration/claimable/")
	if addrStr == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing address", "")
		return
	}

	// Convert address string to Address type
	addr, err := parseAddress(addrStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid address format", err.Error())
		return
	}

	hub := s.GetMigrationHub()
	if hub == nil {
		// Return empty response when no hub is configured
		writeSuccess(w, ClaimableResponse{
			Address:        addrStr,
			TotalClaimable: 0,
			AIB1Claimable:  0,
			CrossChainClaimable: CrossChainClaimable{
				BTC: 0,
				ETH: 0,
				SOL: 0,
			},
		})
		return
	}

	info := hub.GetUserMigrationInfo(addr)

	// Calculate AIB1 claimable (if not claimed)
	var aib1Claimable uint64
	if !info.AIB1Claimed && info.AIB1SnapshotBalance > 0 {
		aib1Claimable = info.AIB1SnapshotBalance
	}

	writeSuccess(w, ClaimableResponse{
		Address:        addrStr,
		TotalClaimable: info.TotalClaimable + aib1Claimable,
		AIB1Claimable:  aib1Claimable,
		CrossChainClaimable: CrossChainClaimable{
			BTC: info.TotalClaimable,
			ETH: 0,
			SOL: 0,
		},
	})
}

// handleClaimAIB1 处理 POST /api/migration/claim-aib1
// 领取 AIB1 快照代币
func (s *Server) handleClaimAIB1(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req ClaimAIB1Request
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	// Validate required fields
	if req.TargetAddress == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing target_address", "")
		return
	}
	if req.Amount == 0 {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Amount must be greater than 0", "")
		return
	}
	if req.PublicKey == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing public_key", "")
		return
	}
	if req.Signature == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing signature", "")
		return
	}

	hub := s.GetMigrationHub()
	if hub == nil {
		writeError(w, http.StatusServiceUnavailable, ErrCodeInternalError, "Migration service not available", "")
		return
	}

	// Decode public key and signature from base64
	pubKey, err := base64.StdEncoding.DecodeString(req.PublicKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidSignature, "Invalid public_key format", "must be base64 encoded")
		return
	}

	sig, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidSignature, "Invalid signature format", "must be base64 encoded")
		return
	}

	// Convert address string to Address type
	targetAddr, err := parseAddress(req.TargetAddress)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid address format", err.Error())
		return
	}

	// Perform the claim
	err = hub.ClaimAIB1(targetAddr, req.Amount, pubKey, sig, req.Nonce)
	if err != nil {
		// Map error to appropriate response
		errCode := ErrCodeInternalError
		errMsg := err.Error()

		if err == migration.ErrAlreadyClaimed {
			errCode = ErrCodeAlreadyClaimed
			errMsg = "Address has already claimed AIB1 tokens"
		} else if err == migration.ErrClaimExpired {
			errCode = ErrCodeMigrationWindowClosed
			errMsg = "AIB1 claim deadline has expired"
		} else if err == migration.ErrSnapshotNotFound {
			errCode = ErrCodeMigrationNotFound
			errMsg = "Address not found in AIB1 snapshot"
		} else if err == migration.ErrInvalidSignature {
			errCode = ErrCodeInvalidSignature
			errMsg = "Signature verification failed"
		}

		writeError(w, http.StatusBadRequest, errCode, errMsg, "")
		return
	}

	// Generate transaction hash
	txHash := fmt.Sprintf("%x", time.Now().UnixNano())

	writeSuccess(w, MigrationClaimResponse{
		TxHash:    txHash,
		Address:   req.TargetAddress,
		Amount:    req.Amount,
		Type:      "aib1",
		Timestamp: time.Now().UTC(),
	})
}

// handleClaimUnlocked 处理 POST /api/migration/claim-unlocked
// 领取已解锁代币
func (s *Server) handleClaimUnlocked(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	var req ClaimUnlockedRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid request body", err.Error())
		return
	}

	// Validate required fields
	if req.Address == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing address", "")
		return
	}

	hub := s.GetMigrationHub()
	if hub == nil {
		writeError(w, http.StatusServiceUnavailable, ErrCodeInternalError, "Migration service not available", "")
		return
	}

	// Convert address string to Address type
	addr, err := parseAddress(req.Address)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid address format", err.Error())
		return
	}

	// Perform the claim
	claimed, err := hub.ClaimUnlocked(addr)
	if err != nil {
		errCode := ErrCodeInternalError
		errMsg := err.Error()

		if err == migration.ErrNothingToClaim {
			errCode = ErrCodeNotFound
			errMsg = "No unlocked tokens available to claim"
		}

		writeError(w, http.StatusBadRequest, errCode, errMsg, "")
		return
	}

	// Generate transaction hash
	txHash := fmt.Sprintf("%x", time.Now().UnixNano())

	writeSuccess(w, MigrationClaimResponse{
		TxHash:    txHash,
		Address:   req.Address,
		Amount:    claimed,
		Type:      "cross_chain",
		Timestamp: time.Now().UTC(),
	})
}

// handleMigrationEstimate 处理 GET /api/migration/estimate
// 估算迁移收益
func (s *Server) handleMigrationEstimate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, ErrCodeInvalidRequest, "Method not allowed", "")
		return
	}

	// Parse query parameters
	chain := r.URL.Query().Get("chain")
	amountStr := r.URL.Query().Get("amount")

	if chain == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing chain parameter", "chain is required (BTC, ETH, or SOL)")
		return
	}

	if amountStr == "" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Missing amount parameter", "amount is required")
		return
	}

	amount, err := strconv.ParseUint(amountStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid amount", "must be a positive integer")
		return
	}

	// Validate chain
	chain = normalizeChain(chain)
	if chain != "BTC" && chain != "ETH" && chain != "SOL" {
		writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid chain", "must be BTC, ETH, or SOL")
		return
	}

	hub := s.GetMigrationHub()
	var currentRate uint64 = 0

	if hub != nil {
		rate, err := hub.GetCrossChainRate(chain)
		if err == nil {
			currentRate = rate
		}
	}

	// Default rates if no hub
	if currentRate == 0 {
		switch chain {
		case "BTC":
			currentRate = 5
		case "ETH":
			currentRate = 4
		case "SOL":
			currentRate = 3
		}
	}

	// Calculate rewards
	totalReward := amount * currentRate
	tgePercent := uint64(20)
	tgeAmount := totalReward * tgePercent / 100
	vestingMonths := uint64(3)

	// Build vesting schedule
	vesting := buildVestingSchedule(totalReward, tgePercent, vestingMonths)

	writeSuccess(w, EstimateResponse{
		SourceChain:  chain,
		SourceAmount: amount,
		Reward: EstimateRewardInfo{
			TotalReward:   totalReward,
			CurrentRate:   currentRate,
			TGEPercent:    tgePercent,
			TGEAmount:     tgeAmount,
			VestingMonths: vestingMonths,
		},
		Vesting: vesting,
	})
}

// ============================================================================
// Helper Functions
// ============================================================================

// parseAddress parses a hex string to Address type
func parseAddress(addrStr string) (interfaces.Address, error) {
	var addr interfaces.Address
	addrBytes, err := hex.DecodeString(addrStr)
	if err != nil {
		// Try as raw string (32 bytes)
		if len(addrStr) == 32 {
			copy(addr[:], addrStr)
			return addr, nil
		}
		return addr, fmt.Errorf("must be 32 bytes hex or 32 char string")
	}
	if len(addrBytes) != 32 {
		return addr, fmt.Errorf("must be 32 bytes")
	}
	copy(addr[:], addrBytes)
	return addr, nil
}

// normalizeChain normalizes chain string to uppercase
func normalizeChain(chain string) string {
	switch chain {
	case "btc", "BTC":
		return "BTC"
	case "eth", "ETH":
		return "ETH"
	case "sol", "SOL":
		return "SOL"
	default:
		return chain
	}
}

// convertUserMigrationInfo converts migration.UserMigrationInfo to API format
func convertUserMigrationInfo(info *migration.UserMigrationInfo) UserMigrationInfoAPI {
	now := time.Now()

	// Convert BTC rewards
	btcRewards := make([]VestingRewardInfo, 0, len(info.BTCLockedRewards))
	for _, r := range info.BTCLockedRewards {
		btcRewards = append(btcRewards, convertVestingReward(r, now))
	}

	// Convert ETH rewards
	ethRewards := make([]VestingRewardInfo, 0, len(info.ETHLockedRewards))
	for _, r := range info.ETHLockedRewards {
		ethRewards = append(ethRewards, convertVestingReward(r, now))
	}

	// Convert SOL rewards
	solRewards := make([]VestingRewardInfo, 0, len(info.SOLLockedRewards))
	for _, r := range info.SOLLockedRewards {
		solRewards = append(solRewards, convertVestingReward(r, now))
	}

	return UserMigrationInfoAPI{
		AIB1SnapshotBalance: info.AIB1SnapshotBalance,
		AIB1Claimed:         info.AIB1Claimed,
		LockedRewards: LockedRewardsInfo{
			BTC: btcRewards,
			ETH: ethRewards,
			SOL: solRewards,
		},
		TotalClaimable: info.TotalClaimable,
		TotalLocked:    info.TotalLocked,
	}
}

// convertVestingReward converts migration.LockedReward to API format
func convertVestingReward(r *migration.LockedReward, now time.Time) VestingRewardInfo {
	schedule := make([]VestingEntryInfo, 0, len(r.Vesting))
	for _, v := range r.Vesting {
		amount := r.TotalReward * v.Percent / 100
		status := "locked"
		if now.After(v.UnlockTime) || now.Equal(v.UnlockTime) {
			status = "unlocked"
		}
		schedule = append(schedule, VestingEntryInfo{
			UnlockTime: v.UnlockTime,
			Percent:    v.Percent,
			Amount:     amount,
			Status:     status,
		})
	}

	return VestingRewardInfo{
		SourceTxID:      hex.EncodeToString(r.SourceTxID[:]),
		SourceAmount:    r.SourceAmount,
		TotalReward:     r.TotalReward,
		Claimed:         r.Claimed,
		Claimable:       r.Claimable(now),
		Locked:          r.RemainingLocked(),
		VestingSchedule: schedule,
	}
}

// buildVestingSchedule builds a vesting schedule for estimation
func buildVestingSchedule(totalReward, tgePercent, vestingMonths uint64) []VestingEntryInfo {
	now := time.Now()
	entries := make([]VestingEntryInfo, 0, vestingMonths+1)

	// TGE entry
	tgeAmount := totalReward * tgePercent / 100
	entries = append(entries, VestingEntryInfo{
		UnlockTime: now,
		Percent:    tgePercent,
		Amount:     tgeAmount,
		Status:     "unlocked",
	})

	// Vesting entries
	remaining := uint64(100) - tgePercent
	perMonth := remaining / vestingMonths

	for i := uint64(1); i <= vestingMonths; i++ {
		unlockTime := now.AddDate(0, int(i), 0)
		percent := perMonth
		if i == vestingMonths {
			percent = remaining - (perMonth * (vestingMonths - 1))
		}
		amount := totalReward * percent / 100

		entries = append(entries, VestingEntryInfo{
			UnlockTime: unlockTime,
			Percent:    percent,
			Amount:     amount,
			Status:     "locked",
		})
	}

	return entries
}
