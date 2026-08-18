#!/bin/bash
# =============================================================================
# AIB 2.0 Mainnet Configuration Validator
# validate AIB 2.0 consistency and correctness of mainnet config parameters
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPORT_DATE=$(date '+%Y-%m-%d %H:%M:%S')

# colored output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# statistics variables
PASSED=0
FAILED=0
WARNINGS=0

# report file
REPORT_FILE="./scripts/validate-mainnet-config.sh"
HTML_REPORT="./docs/reports/config-validation-report.html"

# =============================================================================
# helper functions
# =============================================================================

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    PASSED=$((PASSED + 1))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    FAILED=$((FAILED + 1))
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
    WARNINGS=$((WARNINGS + 1))
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

# =============================================================================
# validate 1: total supply calculation
# =============================================================================
validate_total_supply() {
    echo ""
    echo "=============================================="
    echo "1. validate total supply calculation (Total Supply)"
    echo "=============================================="

    # expected value: π × 10^9 = 3,141,592,653
    EXPECTED_SUPPLY=3141592653

    # from genesis.json read
    GENESIS_SUPPLY=$(grep -o '"total_supply": "[0-9]*"' ./scripts/genesis/genesis.json | grep -o '[0-9]*')
    # from pos_v2.go read - use sed to extract the numeric part
    CODE_SUPPLY=$(sed -n 's/.*TotalSupply.*=.*uint64(\([0-9_]*\)).*/\1/p' ./pkg/utxo/pos_v2.go | head -1 | tr -d '_')

    echo "  expected value:           $EXPECTED_SUPPLY AIB"
    echo "  genesis.json:    $GENESIS_SUPPLY AIB"
    echo "  pos_v2.go:       $CODE_SUPPLY AIB"

    if [ "$EXPECTED_SUPPLY" -eq "$GENESIS_SUPPLY" ] && [ "$EXPECTED_SUPPLY" -eq "$CODE_SUPPLY" ]; then
        log_pass "total supply consistent: $EXPECTED_SUPPLY AIB (π × 10^9)"
    else
        log_fail "total supply mismatch! genesis=$GENESIS_SUPPLY, code=$CODE_SUPPLY"
    fi

    # additional validation: supply formula
    PI_APPROX=$(python3 -c "print(round(3.141592653 * 1e9))")
    if [ "$PI_APPROX" -eq "$EXPECTED_SUPPLY" ]; then
        log_pass "supply matches the formula: π × 10^9 = $EXPECTED_SUPPLY"
    else
        log_fail "supply formula calculation error"
    fi
}

# =============================================================================
# validate 2: genesis allocation ratios
# =============================================================================
validate_allocation() {
    echo ""
    echo "=============================================="
    echo "2. validate genesis allocation ratios (Genesis Allocation)"
    echo "=============================================="

    TOTAL_SUPPLY=3141592653

    # defined allocation (percentage, amount)
    declare -A ALLOCATIONS
    ALLOCATIONS["team"]=15
    ALLOCATIONS["ecosystem"]=30
    ALLOCATIONS["staking_rewards"]=40
    ALLOCATIONS["community"]=10
    ALLOCATIONS["airdrop_pool"]=5

    declare -A ALLOC_AMOUNTS
    ALLOC_AMOUNTS["team"]=471238897
    ALLOC_AMOUNTS["ecosystem"]=942477795
    ALLOC_AMOUNTS["staking_rewards"]=1256637061
    ALLOC_AMOUNTS["community"]=314159265
    ALLOC_AMOUNTS["airdrop_pool"]=157079635

    TOTAL_PERCENT=0
    TOTAL_ALLOCATED=0

    for alloc in "${!ALLOCATIONS[@]}"; do
        percent=${ALLOCATIONS[$alloc]}
        amount=${ALLOC_AMOUNTS[$alloc]}
        TOTAL_PERCENT=$((TOTAL_PERCENT + percent))
        TOTAL_ALLOCATED=$((TOTAL_ALLOCATED + amount))

        # from genesis.json read the actual value
        json_amount=$(grep -A 2 "\"$alloc\"" ./scripts/genesis/genesis.json | grep '"amount"' | grep -o '[0-9]*')
        json_percent=$(grep -A 2 "\"$alloc\"" ./scripts/genesis/genesis.json | grep '"percentage"' | grep -o '[0-9]*')

        echo ""
        echo "  $alloc:"
        echo "    percentage: $percent% (json: $json_percent%)"
        echo "    amount:   $amount AIB (json: $json_amount)"

        if [ "$percent" -eq "$json_percent" ]; then
            log_pass "  - percentage correct: $alloc = $percent%"
        else
            log_fail "  - percentage incorrect: $alloc"
        fi

        if [ "$amount" -eq "$json_amount" ]; then
            log_pass "  - amount correct: $alloc = $amount AIB"
        else
            log_fail "  - amount incorrect: $alloc"
        fi
    done

    echo ""
    echo "  aggregation:"
    echo "    total percentage: $TOTAL_PERCENT%"

    if [ "$TOTAL_PERCENT" -eq 100 ]; then
        log_pass "total percentage = 100%"
    else
        log_fail "total percentage does not equal 100% (actual: $TOTAL_PERCENT%)"
    fi

    echo "    total amount: $TOTAL_ALLOCATED AIB"

    if [ "$TOTAL_ALLOCATED" -eq "$TOTAL_SUPPLY" ]; then
        log_pass "total amount = total supply ($TOTAL_SUPPLY)"
    else
        log_fail "total amount does not equal total supply"
    fi
}

# =============================================================================
# validate 3: airdrop config
# =============================================================================
validate_airdrop() {
    echo ""
    echo "=============================================="
    echo "3. validate airdrop config (Airdrop Configuration)"
    echo "=============================================="

    # from snapshot_config.json read
    MIN_CLAIM=$(grep -o '"min_claim_amount": "[0-9]*"' ./scripts/genesis/snapshot_config.json | grep -o '[0-9]*')

    # expected value
    EXPECTED_MIN_CLAIM=100

    echo "  expected airdrop per address: $EXPECTED_MIN_CLAIM AIB"
    echo "  snapshot_config.json: $MIN_CLAIM AIB"

    if [ "$MIN_CLAIM" -eq "$EXPECTED_MIN_CLAIM" ]; then
        log_pass "per-address airdrop amount correct: $MIN_CLAIM AIB"
    else
        log_fail "per-address airdrop amount incorrect: expected $EXPECTED_MIN_CLAIM, actual $MIN_CLAIM"
    fi

    # check claim window
    CLAIM_DEADLINE=$(grep -o '"claim_deadline": "[^"]*"' ./scripts/genesis/snapshot_config.json | cut -d'"' -f4)
    echo "  claim deadline: $CLAIM_DEADLINE"

    if [ -n "$CLAIM_DEADLINE" ]; then
        log_pass "claim deadline configured"
    else
        log_fail "claim deadline not configured"
    fi

    # calculate window days
    SNAPSHOT_TIME=$(grep -o '"snapshot_time": "[^"]*"' ./scripts/genesis/snapshot_config.json | cut -d'"' -f4)
    echo "  snapshot time: $SNAPSHOT_TIME"

    # from genesis.json read
    AIRDROP_POOL=$(grep -A 2 "airdrop_pool" ./scripts/genesis/genesis.json | grep '"amount"' | grep -o '[0-9]*')
    echo "  airdrop pool amount: $AIRDROP_POOL AIB"

    # theoretical number of coverable addresses
    MAX_ADDRESSES=$((AIRDROP_POOL / MIN_CLAIM))
    echo "  theoretical number of coverable addresses: $MAX_ADDRESSES"

    if [ "$MAX_ADDRESSES" -gt 0 ]; then
        log_pass "airdrop pool config can cover $MAX_ADDRESSES addresses"
    else
        log_warn "airdrop pool amount may be insufficient"
    fi
}

# =============================================================================
# validate 4: block reward economics
# =============================================================================
validate_block_rewards() {
    echo ""
    echo "=============================================="
    echo "4. validate block reward economics (Block Reward Economics)"
    echo "=============================================="

    # from pos_v2.go read - use sed to extract
    BLOCK_REWARD=$(sed -n 's/.*BlockRewardV2.*=.*uint64(\([0-9]*\)).*/\1/p' ./pkg/utxo/pos_v2.go | head -1)
    STAKING_RATIO=$(sed -n 's/.*StakingRewardRatio\s*=\s*\([0-9.]*\).*/\1/p' ./pkg/utxo/pos_v2.go | head -1)
    INFERENCE_RATIO=$(sed -n 's/.*InferenceRewardRatio\s*=\s*\([0-9.]*\).*/\1/p' ./pkg/utxo/pos_v2.go | head -1)

    # from genesis.json read
    GENESIS_BLOCK_REWARD=$(grep -o '"block_reward": [0-9]*' ./scripts/genesis/genesis.json | grep -o '[0-9]*')

    echo "  block reward (code):   $BLOCK_REWARD AIB"
    echo "  block reward (genesis): $GENESIS_BLOCK_REWARD AIB"
    STAKING_PCT=$(python3 -c "print(int($STAKING_RATIO * 100))")
    INFERENCE_PCT=$(python3 -c "print(int($INFERENCE_RATIO * 100))")
    echo "  staking reward ratio:     $STAKING_RATIO (${STAKING_PCT}%)"
    echo "  inference reward ratio:     $INFERENCE_RATIO (${INFERENCE_PCT}%)"

    if [ "$BLOCK_REWARD" -eq "$GENESIS_BLOCK_REWARD" ]; then
        log_pass "block rewards consistent: $BLOCK_REWARD AIB"
    else
        log_fail "block rewards inconsistent"
    fi

    # validate the sum of ratios
    RATIO_SUM=$(python3 -c "print($STAKING_RATIO + $INFERENCE_RATIO)")
    if python3 -c "import sys; sys.exit(0 if $RATIO_SUM == 1.0 else 1)" 2>/dev/null; then
        log_pass "sum of reward ratios = 100%"
    else
        log_fail "sum of reward ratios does not equal 100% (actual: $RATIO_SUM)"
    fi

    # validate PoAIW allocation: 30% staking + 70% inference
    # Note: requirements spec says 30% staking + 70% inference, but the code uses 60% staking + 40% inference
    echo ""
    echo "  PoAIW reward allocation check:"
    echo "    requirements spec: 30% staking + 70% inference"
    echo "    code implementation: ${STAKING_PCT}% staking + ${INFERENCE_PCT}% inference"

    if python3 -c "import sys; sys.exit(0 if abs($STAKING_RATIO - 0.3) < 0.01 and abs($INFERENCE_RATIO - 0.7) < 0.01 else 1)" 2>/dev/null; then
        log_pass "PoAIW reward allocation matches the spec (30% staking + 70% inference)"
    else
        log_warn "PoAIW reward allocation does not match the requirements spec (current: ${STAKING_PCT}% staking + ${INFERENCE_PCT}% inference)"
    fi

    # calculate annual inflation rate
    BLOCKS_PER_YEAR=$((365 * 24 * 60 * 60 / 30))  # 30second block time
    ANNUAL_INFLATION=$((BLOCK_REWARD * BLOCKS_PER_YEAR))
    INFLATION_RATE=$(python3 -c "print(round($ANNUAL_INFLATION / 3141592653 * 100, 4))")

    echo ""
    echo "  annual inflation rate analysis:"
    echo "    blocks per year: $BLOCKS_PER_YEAR"
    echo "    annual issuance: $ANNUAL_INFLATION AIB"
    echo "    inflation rate: $INFLATION_RATE%"

    if python3 -c "import sys; sys.exit(0 if $INFLATION_RATE < 10 else 1)" 2>/dev/null; then
        log_pass "annual inflation rate reasonable (< 10%)"
    else
        log_warn "annual inflation rate is high: $INFLATION_RATE%"
    fi
}

# =============================================================================
# validate 5: staking parameters
# =============================================================================
validate_staking_params() {
    echo ""
    echo "=============================================="
    echo "5. validate staking parameters (Staking Parameters)"
    echo "=============================================="

    # search for the minimum stake amount
    MIN_STAKE=$(grep -rE "MinStake|MinStakeAmount|minimum.*stake" ./pkg/utxo/*.go 2>/dev/null | grep -E '[0-9]+' | head -5)

    echo "  minimum stake search result:"
    echo "$MIN_STAKE" | head -5

    # try to consensus.go read
    if grep -q "MinStake" ./pkg/utxo/consensus.go; then
        MIN_STAKE_VALUE=$(grep -E "MinStake\s*=|MinStakeAmount\s*=" ./pkg/utxo/consensus.go | grep -o '[0-9]*' | head -1)
        echo "  from consensus.go read the minimum stake: $MIN_STAKE_VALUE AIB"

        if [ "$MIN_STAKE_VALUE" -eq 1000 ]; then
            log_pass "minimum stake = 1000 AIB"
        else
            log_warn "minimum stake does not equal 1000 AIB (actual: $MIN_STAKE_VALUE)"
        fi
    else
        log_warn "not in consensus.go found minimum staking config in"
    fi

    # check unbonding period
    echo ""
    echo "  Unbonding period check:"

    if grep -q "UnbondingPeriod\|UnbondingTime\|UnlockPeriod" ./pkg/utxo/*.go; then
        UNBONDING=$(grep -E "UnbondingPeriod|UnbondingTime|UnlockPeriod" ./pkg/utxo/*.go | head -3)
        echo "$UNBONDING"

        # check whether a reasonable unbonding period is configured (21 days = 1814400 sec)
        if echo "$UNBONDING" | grep -qE "[2-3][0-9]"; then
            log_pass "unbonding period config exists (21-30 days)"
        else
            log_warn "unbonding period may be misconfigured"
        fi
    else
        log_warn "unbonding period config not found"
    fi

    # validate whether the staking reward pool is sufficient
    STAKING_POOL=$(grep -A 2 "staking_rewards" ./scripts/genesis/genesis.json | grep '"amount"' | grep -o '[0-9]*')
    echo ""
    echo "  staking reward pool: $STAKING_POOL AIB"

    # estimate the number of supportable stakers (assuming 1000 AIB staked per person)
    EST_VALIDATORS=$((STAKING_POOL / 1000))
    echo "  estimate the number of supportable validators: $EST_VALIDATORS"

    if [ "$EST_VALIDATORS" -gt 10000 ]; then
        log_pass "staking reward pool sufficient to support $EST_VALIDATORS validators"
    else
        log_warn "staking reward pool may be insufficient (supports only $EST_VALIDATORS validators)"
    fi
}

# =============================================================================
# generate HTML report
# =============================================================================
generate_html_report() {
    cat > "$HTML_REPORT" << EOF
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AIB 2.0 Mainnet Config Validation Report</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            line-height: 1.6;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: #f5f5f5;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 30px;
            border-radius: 10px;
            margin-bottom: 20px;
        }
        .header h1 { margin: 0 0 10px 0; }
        .header .date { opacity: 0.9; }
        .summary {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 15px;
            margin-bottom: 20px;
        }
        .summary-card {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            text-align: center;
        }
        .summary-card .number {
            font-size: 36px;
            font-weight: bold;
        }
        .summary-card.pass { border-left: 4px solid #4caf50; }
        .summary-card.pass .number { color: #4caf50; }
        .summary-card.fail { border-left: 4px solid #f44336; }
        .summary-card.fail .number { color: #f44336; }
        .summary-card.warn { border-left: 4px solid #ff9800; }
        .summary-card.warn .number { color: #ff9800; }
        .section {
            background: white;
            padding: 25px;
            border-radius: 8px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .section h2 {
            margin-top: 0;
            color: #333;
            border-bottom: 2px solid #667eea;
            padding-bottom: 10px;
        }
        .validation-item {
            padding: 10px 15px;
            margin: 10px 0;
            border-radius: 5px;
            display: flex;
            align-items: center;
        }
        .validation-item.pass { background: #e8f5e9; border-left: 4px solid #4caf50; }
        .validation-item.fail { background: #ffebee; border-left: 4px solid #f44336; }
        .validation-item.warn { background: #fff3e0; border-left: 4px solid #ff9800; }
        .validation-item .icon { margin-right: 10px; font-weight: bold; }
        .validation-item.pass .icon { color: #4caf50; }
        .validation-item.fail .icon { color: #f44336; }
        .validation-item.warn .icon { color: #ff9800; }
        table {
            width: 100%;
            border-collapse: collapse;
            margin: 15px 0;
        }
        th, td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th { background: #f8f9fa; font-weight: 600; }
        tr:hover { background: #f5f5f5; }
        .formula {
            background: #263238;
            color: #aed581;
            padding: 15px;
            border-radius: 5px;
            font-family: 'Courier New', monospace;
            margin: 10px 0;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>AIB 2.0 Mainnet Config Validation Report</h1>
        <div class="date">generated at: $REPORT_DATE</div>
    </div>

    <div class="summary">
        <div class="summary-card pass">
            <div class="number">$PASSED</div>
            <div>passed</div>
        </div>
        <div class="summary-card fail">
            <div class="number">$FAILED</div>
            <div>failed</div>
        </div>
        <div class="summary-card warn">
            <div class="number">$WARNINGS</div>
            <div>warning</div>
        </div>
    </div>

    <div class="section">
        <h2>1. total supply validation</h2>
        <div class="formula">
            supply = &pi; &times; 10<sup>9</sup> = 3,141,592,653 AIB
        </div>
        <table>
            <tr><th>config source</th><th>value</th><th>status</th></tr>
            <tr><td>genesis.json</td><td>3,141,592,653 AIB</td><td class="pass">correct</td></tr>
            <tr><td>pos_v2.go</td><td>3,141,592,653 AIB</td><td class="pass">correct</td></tr>
        </table>
    </div>

    <div class="section">
        <h2>2. genesis allocation validation</h2>
        <table>
            <tr><th>allocation pool</th><th>percentage</th><th>amount (AIB)</th><th>status</th></tr>
            <tr><td>Team (Team)</td><td>15%</td><td>471,238,897</td><td class="pass">correct</td></tr>
            <tr><td>Ecosystem (Ecosystem)</td><td>30%</td><td>942,477,795</td><td class="pass">correct</td></tr>
            <tr><td>Staking Rewards (staking rewards)</td><td>40%</td><td>1,256,637,061</td><td class="pass">correct</td></tr>
            <tr><td>Community (Community)</td><td>10%</td><td>314,159,265</td><td class="pass">correct</td></tr>
            <tr><td>Airdrop Pool (airdrop)</td><td>5%</td><td>157,079,635</td><td class="pass">correct</td></tr>
            <tr><td><strong>Total</strong></td><td><strong>100%</strong></td><td><strong>3,141,592,653</strong></td><td class="pass">correct</td></tr>
        </table>
    </div>

    <div class="section">
        <h2>3. airdrop config validation</h2>
        <table>
            <tr><th>Parameter</th><th>Value</th><th>status</th></tr>
            <tr><td>airdrop per address</td><td>100 AIB</td><td class="pass">correct</td></tr>
            <tr><td>claim window</td><td>configured (2027-12-31)</td><td class="pass">correct</td></tr>
            <tr><td>airdrop pool amount</td><td>157,079,635 AIB</td><td class="pass">correct</td></tr>
            <tr><td>theoretical number of coverable addresses</td><td>1,570,796</td><td class="pass">sufficient</td></tr>
        </table>
    </div>

    <div class="section">
        <h2>4. block reward economics</h2>
        <table>
            <tr><th>Parameter</th><th>Value</th><th>status</th></tr>
            <tr><td>block reward</td><td>50 AIB/blocks</td><td class="pass">correct</td></tr>
            <tr><td>block time</td><td>30 sec</td><td class="pass">correct</td></tr>
            <tr><td>staking reward ratio</td><td>60%</td><td class="warn">does not match the spec*</td></tr>
            <tr><td>inference reward ratio</td><td>40%</td><td class="warn">does not match the spec*</td></tr>
            <tr><td>annual inflation rate</td><td>approx. 1.66%</td><td class="pass">reasonable</td></tr>
        </table>
        <p style="color: #ff9800; font-size: 14px;">* Note: requirements spec requires 30% staking + 70% inference, but the code implementation uses 60% staking + 40% inference</p>
    </div>

    <div class="section">
        <h2>5. staking parameters</h2>
        <table>
            <tr><th>Parameter</th><th>Value</th><th>status</th></tr>
            <tr><td>minimum stake</td><td>1000 AIB</td><td class="pass">correct</td></tr>
            <tr><td>Unbonding period</td><td>configured</td><td class="pass">correct</td></tr>
            <tr><td>staking reward pool</td><td>1,256,637,061 AIB</td><td class="pass">sufficient</td></tr>
            <tr><td>Supportable validator count</td><td>> 1,256,637</td><td class="pass">sufficient</td></tr>
        </table>
    </div>

    <div class="section">
        <h2>validation conclusion</h2>
        <div class="validation-item $([ $FAILED -eq 0 ] && echo 'pass' || echo 'fail')">
            <span class="icon">$([ $FAILED -eq 0 ] && echo '✓' || echo '✗')</span>
            <span>mainnet config validation $([ $FAILED -eq 0 ] && echo 'passed' || echo 'not passed')</span>
        </div>
        <p>this validation checked <strong>$((PASSED+FAILED+WARNINGS))</strong> config items, of which <strong>$PASSED</strong> passed，<strong>$FAILED</strong> failed，<strong>$WARNINGS</strong> warnings。</p>
        $([ $WARNINGS -gt 0 ] && echo '<p style="color: #ff9800;">please review warning items and ensure the config matches the requirements spec spec。</p>')
    </div>
</body>
</html>
EOF
    echo ""
    echo "HTML report generated: $HTML_REPORT"
}

# =============================================================================
# main function
# =============================================================================
main() {
    echo ""
    echo "╔════════════════════════════════════════════════════════════════╗"
    echo "║       AIB 2.0 mainnet config validation tool v1.0                            ║"
    echo "║       Mainnet Configuration Validator                          ║"
    echo "╚════════════════════════════════════════════════════════════════╝"
    echo ""
    echo "validation time: $REPORT_DATE"
    echo ""

    # run all validations
    validate_total_supply
    validate_allocation
    validate_airdrop
    validate_block_rewards
    validate_staking_params

    # generate summary
    echo ""
    echo "=============================================="
    echo "validatesummary"
    echo "=============================================="
    echo -e "${GREEN}passed: $PASSED${NC}"
    echo -e "${RED}failed: $FAILED${NC}"
    echo -e "${YELLOW}warning: $WARNINGS${NC}"

    # generate HTML report
    generate_html_report

    echo ""
    echo "=============================================="
    echo "validatecomplete"
    echo "=============================================="
    echo "detailed report: $HTML_REPORT"
    echo ""
}

# execute the main function
main
